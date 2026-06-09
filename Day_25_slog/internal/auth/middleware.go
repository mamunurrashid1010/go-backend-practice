package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"day25/internal/logging"
	"day25/internal/respond"
)

type userIDKey struct{}

func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}

// RequireAuth validates the bearer token and:
//   1. puts the userID on the context (Day 19)
//   2. enriches the request-scoped logger with user_id (Day 25)
// so every log line downstream carries who made the request.
func RequireAuth(v *TokenVerifier) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenStr, ok := bearerToken(r)
			if !ok {
				respond.Unauthorized(w, "missing or malformed Authorization header")
				return
			}
			claims, err := v.Verify(tokenStr)
			if err != nil {
				respond.Unauthorized(w, "invalid or expired token")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			// Enrich the logger so downstream log lines have user_id.
			ctx = logging.With(ctx,
				logging.From(ctx).With(slog.Int64("user_id", claims.UserID)))
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func bearerToken(r *http.Request) (string, bool) {
	const prefix = "Bearer "
	h := r.Header.Get("Authorization")
	if h == "" || !strings.HasPrefix(h, prefix) {
		return "", false
	}
	tok := strings.TrimPrefix(h, prefix)
	if tok == "" {
		return "", false
	}
	return tok, true
}
