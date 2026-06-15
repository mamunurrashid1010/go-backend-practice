package middleware

import (
	"net"
	"net/http"
	"strconv"

	"day28/internal/auth"
	"day28/internal/ratelimit"
	"day28/internal/respond"
)

func RateLimit(l *ratelimit.Limiter) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := clientKey(r)
			limit := strconv.Itoa(l.Burst())
			w.Header().Set("X-RateLimit-Limit", limit)

			if !l.Allow(key) {
				w.Header().Set("X-RateLimit-Remaining", "0")
				w.Header().Set("Retry-After", "1")
				respond.Error(w, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests")
				return
			}
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(l.Tokens(key)))
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
