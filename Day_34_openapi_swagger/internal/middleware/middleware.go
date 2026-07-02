package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"day34/internal/logging"
	"day34/internal/respond"
)

type requestIDKey struct{}

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

func GetRequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

func newRequestID() string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

type loggingRW struct {
	http.ResponseWriter
	status int
	size   int
}

func (l *loggingRW) WriteHeader(code int) { l.status = code; l.ResponseWriter.WriteHeader(code) }
func (l *loggingRW) Write(b []byte) (int, error) {
	if l.status == 0 {
		l.status = http.StatusOK
	}
	n, err := l.ResponseWriter.Write(b)
	l.size += n
	return n, err
}

func Logger(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			reqLog := base.With(
				slog.String("rid", GetRequestID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			ctx := logging.With(r.Context(), reqLog)
			lrw := &loggingRW{ResponseWriter: w}
			next.ServeHTTP(lrw, r.WithContext(ctx))
			reqLog.LogAttrs(ctx, slog.LevelInfo, "http_request",
				slog.Int("status", lrw.status),
				slog.Duration("duration", time.Since(start)),
				slog.Int("size", lrw.size),
			)
		})
	}
}

func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logging.From(r.Context()).ErrorContext(r.Context(), "panic",
					slog.Any("recovered", rec),
					slog.String("stack", string(debug.Stack())),
				)
				respond.Internal(r.Context(), w, fmt.Errorf("panic: %v", rec))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
