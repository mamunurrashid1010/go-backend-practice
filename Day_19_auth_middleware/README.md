# Day 19 — Auth Middleware (Verify + `GetUserID(ctx)`)

> **Goal:** complete the auth loop. Build a `TokenVerifier` (the inverse of yesterday's `TokenIssuer`), a `RequireAuth` middleware that gates protected routes, and a `GET /auth/me` route that proves the whole chain works end-to-end.

This is the day Day 6's middleware pattern pays off in a real, security-critical context.

---

## 1. The header — `Authorization: Bearer <token>`

The convention every HTTP client speaks:

```
Authorization: Bearer eyJhbGciOiJIUzI1NiIs...
```

Two parts separated by a space — the literal word `Bearer` and the token. "Bearer" means "whoever has this token is the user" — no further proof required. The middleware's job is to strip the prefix, hand the token to the verifier, and turn the result into either "yes, this is user N" or "401".

> **`Bearer` is case-insensitive** by the HTTP spec, but Postman, browsers, and every example you'll see use the capitalized form. We accept `Bearer ` exactly today and explore the relaxed parser as a task.

---

## 2. The verifier — inverse of Day 18

`TokenIssuer.Issue(u)` → signed string. `TokenVerifier.Verify(s)` → claims or error. Same secret, same algorithm, same issuer.

```go
func (v *TokenVerifier) Verify(tokenStr string) (*AccessTokenClaims, error) {
    var claims AccessTokenClaims
    parsed, err := jwt.ParseWithClaims(
        tokenStr, &claims,
        func(t *jwt.Token) (any, error) { return v.secret, nil },
        jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Name}),
        jwt.WithIssuer(v.issuer),
        jwt.WithExpirationRequired(),
    )
    if err != nil { return nil, err }
    if !parsed.Valid { return nil, jwt.ErrTokenInvalidClaims }
    return &claims, nil
}
```

Three guards beyond "the signature checks out", each closing a real attack:

| Option | What it stops |
| --- | --- |
| `WithValidMethods([]string{"HS256"})` | The classic `alg: none` attack — an attacker swaps the header to `{"alg":"none"}` and removes the signature. Pinning the allowed methods makes this impossible. Also stops algorithm confusion (signing with HMAC using the RSA public key as the HMAC secret). |
| `WithIssuer(v.issuer)` | A token minted by a *different* service with the same secret is rejected. Rare locally, common in shared-secret multi-service setups. |
| `WithExpirationRequired()` | Refuses a token without `exp`. Defense in depth — your issuer always sets it, but a misconfigured one might not. |

`exp` itself is validated automatically. A token whose `exp` has passed returns `jwt.ErrTokenExpired`.

---

## 3. The middleware

```go
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
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

The middleware does three things:
1. **Extract** the token from the header.
2. **Verify** the signature, expiry, issuer, and algorithm.
3. **Inject** the `userID` into the request context, then call the next handler.

Same shape as Day 6's `RequestID` middleware — the only difference is *what* we put on the context.

---

## 4. The context plumbing

```go
type userIDKey struct{}

func WithUserID(ctx context.Context, id int64) context.Context {
    return context.WithValue(ctx, userIDKey{}, id)
}

func GetUserID(ctx context.Context) (int64, bool) {
    id, ok := ctx.Value(userIDKey{}).(int64)
    return id, ok
}
```

Same unexported-struct-as-key trick from Day 6. Same `(value, ok)` return so the caller can detect "no user on context" — which means "this handler ran without `RequireAuth` in front of it, which is probably a routing bug."

In a protected handler:

```go
func (h *Handler) me(w http.ResponseWriter, r *http.Request) {
    userID, ok := GetUserID(r.Context())
    if !ok {
        // Should never happen if routing is correct.
        respond.Unauthorized(w, "not authenticated")
        return
    }
    u, _ := h.Svc.GetByID(r.Context(), userID)
    respond.JSON(w, http.StatusOK, u)
}
```

The handler **trusts** the userID. The middleware did the verification; if execution got here, the user is who they claim to be.

---

## 5. Mounting protected routes — `r.Group` + `r.Use`

```go
func (h *Handler) Router(verifier *TokenVerifier) chi.Router {
    r := chi.NewRouter()

    // Public
    r.Post("/register", h.register)
    r.Post("/login", h.login)

    // Protected — RequireAuth runs before every route in this group
    r.Group(func(r chi.Router) {
        r.Use(RequireAuth(verifier))
        r.Get("/me", h.me)
    })

    return r
}
```

This is the Day 5 Task 5 pattern, now with real auth. The `r.Group` block is a sub-router — its `r.Use(...)` only applies inside, so register/login stay public.

---

## 6. 401 vs 403

A subtle distinction the HTTP spec gets right and most code gets wrong:

- **`401 Unauthorized`** — "I don't know who you are." No token, bad token, expired token.
- **`403 Forbidden`** — "I know who you are, but you can't do this." Right user, wrong permissions.

Today everything is 401 because we don't have roles yet. Day 27-ish (real auth setups) would add a `RequireRole("admin")` middleware that returns 403 when a valid logged-in user isn't an admin.

---

## 7. What changed from Day 18

| File | Change |
| --- | --- |
| `internal/auth/token.go` | added `TokenVerifier` with `Verify` |
| `internal/auth/middleware.go` | **NEW** — `RequireAuth`, `WithUserID`, `GetUserID` |
| `internal/auth/service.go` | added `GetByID` |
| `internal/auth/handler.go` | added `me` handler + protected `r.Group` in `Router` |
| `main.go` | builds `TokenVerifier`, passes it to `Handler.Router(verifier)` |

Register, login, the password code, the repository, the migrations, the config — all carry over unchanged.

---

## 8. Run it

```powershell
cd Day_19_auth_middleware
docker compose up -d
go mod init day19
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/jackc/pgx/v5/pgconn
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go get golang.org/x/crypto/bcrypt
go get github.com/golang-jwt/jwt/v5
go run .
```

Walk the full flow:

```powershell
# 1. register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register

# 2. login — grab the token
$resp = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json
$token = $resp.access_token

# 3. me — protected, requires the bearer token
curl.exe -i -H "Authorization: Bearer $token" http://localhost:8080/auth/me
# 200 {"id":1,"email":"a@b.com","created_at":...}

# 4. me without a token → 401
curl.exe -i http://localhost:8080/auth/me

# 5. me with a wrong token → 401
curl.exe -i -H "Authorization: Bearer not.a.valid.token" http://localhost:8080/auth/me
```

For maximum learning value:
- Set `JWT_ACCESS_TTL=20s`, restart, login, wait 25 seconds, hit `/me` → see `401` from an expired token.
- Change `JWT_SECRET`, restart (don't re-login), hit `/me` with the *old* token → `401`. Signature no longer verifies under the new secret.

---

## 9. What's next

**Day 20** — refresh tokens. The 15-minute access token is short on purpose (small attack window if it leaks). A refresh token is long-lived (days/weeks), server-stored, and rotated on use. The client uses it to get a new access token when the old one expires — no re-login required.

Then **Day 21** ships the Notes API: a real, multi-user feature where `RequireAuth` gates every route and `GetUserID(r.Context())` decides which notes belong to whom.
