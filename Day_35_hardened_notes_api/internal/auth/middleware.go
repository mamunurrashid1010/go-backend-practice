package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"day35/internal/logging"
	"day35/internal/respond"
)

type userIDKey struct{}

func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}

// RequireUserID reads the authenticated user id from ctx and writes a
// 401 response if it's missing. Handlers get:
//
//	userID, ok := auth.RequireUserID(w, r)
//	if !ok { return }
//
// Extracted in Day 35 — the same helper was hand-rolled in notes and
// audit handlers; three uses is over the "extract it" line.
func RequireUserID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, ok := GetUserID(r.Context())
	if !ok {
		respond.Unauthorized(w, "not authenticated")
		return 0, false
	}
	return id, true
}

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
			ctx = logging.With(ctx, logging.From(ctx).With(slog.Int64("user_id", claims.UserID)))
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
