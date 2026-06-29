package middleware

import (
	"net"
	"net/http"
	"strconv"
	"time"

	"day33/internal/auth"
	"day33/internal/ratelimit"
	"day33/internal/respond"
)

// RateLimit takes any ratelimit.Limiter — memory or Redis. The
// emitted headers are the same regardless of backend.
func RateLimit(l ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientKey(r)
			limit := l.Limit()
			d := l.Allow(r.Context(), key)

			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(d.Remaining))
			if !d.ResetAt.IsZero() {
				w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(d.ResetAt.Unix(), 10))
			}

			if !d.Allowed {
				secs := int(d.RetryAfter.Round(time.Second).Seconds())
				if secs < 1 {
					secs = 1
				}
				w.Header().Set("Retry-After", strconv.Itoa(secs))
				respond.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func clientKey(r *http.Request) string {
	if id, ok := auth.GetUserID(r.Context()); ok {
		return "user:" + strconv.FormatInt(id, 10)
	}
	return "ip:" + clientIP(r)
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
