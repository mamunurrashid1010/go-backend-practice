package idempotency

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"day33/internal/auth"
	"day33/internal/logging"
	"day33/internal/respond"
)

// MaxBody — same cap as httpjson; bigger requests skip idempotency.
const MaxBody = 1 << 20

// Middleware applies idempotency semantics to POST/PUT/PATCH requests
// that carry an Idempotency-Key header. GET/DELETE/HEAD pass through
// unchanged — they're already either idempotent or harmless.
//
// Apply this AFTER auth middleware so the Idempotency-Key can be
// scoped by userID.
func Middleware(store *Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !mutating(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			key := r.Header.Get("Idempotency-Key")
			if key == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Bound the body. We need to read it fully to hash it.
			body, err := readAndRestoreBody(r)
			if err != nil {
				respond.Error(w, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "request body too large for idempotent retry")
				return
			}
			bodyHash := sha256Hex(body)
			scoped := scopeKey(r.Context(), key)

			rec, err := store.Reserve(r.Context(), scoped, bodyHash)
			switch {
			case errors.Is(err, ErrPending):
				respond.Error(w, http.StatusConflict, "IDEMPOTENCY_IN_FLIGHT",
					"a previous request with this Idempotency-Key is still in flight")
				return
			case errors.Is(err, ErrMismatch):
				respond.Error(w, http.StatusUnprocessableEntity, "IDEMPOTENCY_MISMATCH",
					"request body differs from a previous request with the same Idempotency-Key")
				return
			case err != nil:
				// Redis trouble. Idempotency is a bonus, not a contract — log
				// and fall through so the API keeps working.
				logging.From(r.Context()).WarnContext(r.Context(), "idempotency reserve failed",
					slog.String("key", scoped), slog.Any("err", err))
				next.ServeHTTP(w, r)
				return
			}

			if rec != nil {
				// Cached "done" response — replay it.
				replay(w, rec)
				return
			}

			// Slot acquired. Run the handler against a recorder so we
			// can both reply to this client AND save a copy.
			rr := &responseRecorder{
				headers: http.Header{},
				body:    &bytes.Buffer{},
			}
			next.ServeHTTP(rr, r)

			status := rr.status
			if status == 0 {
				status = http.StatusOK
			}

			// Flush to the real client.
			for k, vv := range rr.headers {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(status)
			_, _ = w.Write(rr.body.Bytes())

			// Save best-effort. A failure here means a retry would re-run
			// the handler — annoying but not wrong.
			if err := store.Save(r.Context(), scoped, bodyHash, status, rr.headers.Clone(), rr.body.Bytes()); err != nil {
				logging.From(r.Context()).WarnContext(r.Context(), "idempotency save failed",
					slog.String("key", scoped), slog.Any("err", err))
			}
		})
	}
}

func replay(w http.ResponseWriter, rec *Record) {
	for k, vv := range rec.Headers {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.Header().Set("Idempotent-Replayed", "true")
	w.WriteHeader(rec.Status)
	_, _ = w.Write(rec.Body)
}

func mutating(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch:
		return true
	}
	return false
}

func scopeKey(ctx context.Context, key string) string {
	if id, ok := auth.GetUserID(ctx); ok {
		return fmt.Sprintf("idem:user:%d:%s", id, key)
	}
	return "idem:anon:" + key
}

func readAndRestoreBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	// LimitReader caps reads at MaxBody+1 so we can detect overflow.
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("body exceeds %d bytes", MaxBody)
	}
	// Put the body back for the handler to decode.
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// responseRecorder captures the handler's output so we can save AND
// forward it. Buffering is fine for JSON responses; not for streaming.
type responseRecorder struct {
	headers http.Header
	body    *bytes.Buffer
	status  int
}

func (r *responseRecorder) Header() http.Header  { return r.headers }
func (r *responseRecorder) WriteHeader(s int)    { r.status = s }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}
