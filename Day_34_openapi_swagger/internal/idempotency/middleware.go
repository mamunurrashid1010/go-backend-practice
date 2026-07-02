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

	"day34/internal/auth"
	"day34/internal/logging"
	"day34/internal/respond"
)

const MaxBody = 1 << 20

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
				logging.From(r.Context()).WarnContext(r.Context(), "idempotency reserve failed",
					slog.String("key", scoped), slog.Any("err", err))
				next.ServeHTTP(w, r)
				return
			}

			if rec != nil {
				replay(w, rec)
				return
			}

			rr := &responseRecorder{headers: http.Header{}, body: &bytes.Buffer{}}
			next.ServeHTTP(rr, r)

			status := rr.status
			if status == 0 {
				status = http.StatusOK
			}
			for k, vv := range rr.headers {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(status)
			_, _ = w.Write(rr.body.Bytes())

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
	body, err := io.ReadAll(io.LimitReader(r.Body, MaxBody+1))
	if err != nil {
		return nil, err
	}
	if len(body) > MaxBody {
		return nil, fmt.Errorf("body exceeds %d bytes", MaxBody)
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

type responseRecorder struct {
	headers http.Header
	body    *bytes.Buffer
	status  int
}

func (r *responseRecorder) Header() http.Header { return r.headers }
func (r *responseRecorder) WriteHeader(s int)   { r.status = s }
func (r *responseRecorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.body.Write(b)
}
