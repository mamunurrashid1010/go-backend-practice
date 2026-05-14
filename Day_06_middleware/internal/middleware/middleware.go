// Package middleware contains the hand-written middlewares for Day 6:
//
//   - RequestID — generates or forwards X-Request-ID
//   - Logger    — logs method/path/status/duration with the request ID
//   - Recover   — catches panics, logs the stack, returns a clean 500
//
// All three implement the standard signature:
//
//	func(http.Handler) http.Handler
//
// So they can be chained freely with chi.Router.Use(...) or wrapped manually.
package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"day06/internal/respond"
)

// ---- Request ID --------------------------------------------------------

// requestIDKey is an unexported key type. Using a struct (not a string)
// guarantees no other package can collide with our context key.
type requestIDKey struct{}

// RequestID stores a per-request ID on the request context and echoes it
// back in the X-Request-ID response header. If the client sent X-Request-ID,
// reuse it so traces span multiple services.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// GetRequestID reads the request ID set by the RequestID middleware.
// Returns "" if nothing's there — handlers should tolerate that.
func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// newRequestID returns 8 random bytes as 16 hex chars. Good enough for
// correlation; not cryptographically meaningful. crypto/rand is overkill
// here but it's already in the stdlib and never fails on healthy systems.
func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// ---- Logger ------------------------------------------------------------

// loggingRW wraps http.ResponseWriter so we can record the status code.
// http.ResponseWriter has no public way to read what status was written;
// this is the canonical workaround.
type loggingRW struct {
	http.ResponseWriter
	status int
	size   int
}

func (l *loggingRW) WriteHeader(code int) {
	l.status = code
	l.ResponseWriter.WriteHeader(code)
}

func (l *loggingRW) Write(b []byte) (int, error) {
	if l.status == 0 {
		// Write was called before WriteHeader; Go would default to 200.
		l.status = http.StatusOK
	}
	n, err := l.ResponseWriter.Write(b)
	l.size += n
	return n, err
}

// Logger logs one line per request:
//
//	GET /users 200 1.2ms rid=8f3a91c2 size=126
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lrw := &loggingRW{ResponseWriter: w}
		next.ServeHTTP(lrw, r)
		dur := time.Since(start)

		log.Printf("%-6s %s %d %s rid=%s size=%d",
			r.Method, r.URL.Path, lrw.status, dur, GetRequestID(r.Context()), lrw.size)
	})
}

// ---- Recover -----------------------------------------------------------

// Recover catches any panic from downstream handlers. It logs the cause
// plus a stack trace, then writes a clean JSON 500 to the client.
// Without this, a nil-deref in one handler still completes (Go's HTTP
// server catches per-goroutine) but the client gets a torn connection
// with no body.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				rid := GetRequestID(r.Context())
				log.Printf("panic rid=%s: %v\n%s", rid, rec, debug.Stack())
				respond.Internal(w, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
