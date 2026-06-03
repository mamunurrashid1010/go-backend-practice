package auth

import (
	"context"
	"net/http"
	"strings"

	"day19/internal/respond"
)

// userIDKey is an unexported struct so no other package can collide with
// our context key. Same trick as Day 6's RequestID.
type userIDKey struct{}

// WithUserID attaches the authenticated user id to a request context.
// Used by the middleware after a successful Verify.
func WithUserID(ctx context.Context, id int64) context.Context {
	return context.WithValue(ctx, userIDKey{}, id)
}

// GetUserID reads the user id put on the context by the auth middleware.
// Returns (0, false) when called from a handler that wasn't wrapped — which
// usually means a routing bug.
func GetUserID(ctx context.Context) (int64, bool) {
	id, ok := ctx.Value(userIDKey{}).(int64)
	return id, ok
}

// RequireAuth returns a middleware that:
//  1. extracts "Authorization: Bearer <token>"
//  2. verifies signature/alg/issuer/exp
//  3. injects the user id from claims onto the request context
//  4. calls next
//
// Any failure short-circuits with 401. Day 27-ish would add a separate
// RequireRole middleware for 403-style authorisation.
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
				// Could distinguish expired vs invalid for nicer messages —
				// we keep it generic on purpose (don't leak which check failed).
				respond.Unauthorized(w, "invalid or expired token")
				return
			}
			ctx := WithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// bearerToken pulls the token out of an "Authorization: Bearer <token>"
// header. Returns ("", false) if missing or malformed.
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
