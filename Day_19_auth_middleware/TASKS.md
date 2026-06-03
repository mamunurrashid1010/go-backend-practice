# Day 19 — Practice Tasks

These drill the verify path. Auth bugs are quiet — if you skip the tasks, your code might *look* like it works while letting bad tokens through.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day19
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
> go run .
> ```

---

## Warm-up — the full happy path

```powershell
# register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register
# 201

# login → grab the token
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json
$token = $r.access_token

# me — works
curl.exe -i -H "Authorization: Bearer $token" http://localhost:8080/auth/me   # 200

# me — no header
curl.exe -i http://localhost:8080/auth/me                                       # 401

# me — garbage token
curl.exe -i -H "Authorization: Bearer not.a.real.token" http://localhost:8080/auth/me  # 401

# me — wrong scheme
curl.exe -i -H "Authorization: $token" http://localhost:8080/auth/me            # 401 (no Bearer prefix)
```

- [ ] All five behave as commented.

---

## Task 1 — Watch a token expire in real time

- [ ] Set `JWT_ACCESS_TTL=10s` in `.env` and restart.
- [ ] Login. Save the token.
- [ ] Hit `/me` immediately → 200.
- [ ] Wait 12 seconds. Hit `/me` again → 401.
- [ ] Restore `15m`. Lesson: short TTLs are why we need refresh tokens (Day 20).

---

## Task 2 — Algorithm-confusion attack (the `alg=none` test)

The `WithValidMethods` option exists *specifically* to defeat this attack. Prove it.

- [ ] Take a working token. Decode it (PowerShell snippet from Day 18 Task 0).
- [ ] Hand-craft a "fake" token in jwt.io: set `alg: none`, keep your payload, drop the signature segment (the third part becomes empty).
- [ ] Try to use it: `curl.exe -i -H "Authorization: Bearer eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.<payload>." http://localhost:8080/auth/me`.
- [ ] Expect: **401**. The verifier refuses `alg: none` because it's not in our allowed list.
- [ ] Now temporarily delete the `jwt.WithValidMethods(...)` option from `Verify`. Try again. **Depending on the library version this may now succeed** — old `dgrijalva/jwt-go` famously did. `golang-jwt/jwt/v5` defends by default, but pinning is belt-and-braces. Always pin.

---

## Task 3 — Wrong issuer rejected

- [ ] Change `JWT_ISSUER=different-app` in `.env`, restart. Login (which uses the NEW issuer) → token works on `/me`.
- [ ] Now keep an OLD token (issued under `day19-auth`). Restart with the OLD issuer back in `.env`. The new token (issued under "different-app") is now rejected → 401.
- [ ] Lesson: `iss` lets one secret protect many apps without them accepting each other's tokens.

---

## Task 4 — Distinguish "expired" from "invalid" without leaking

Right now `RequireAuth` returns the same `"invalid or expired token"` for both. Some teams want richer client UX (e.g., trigger refresh on expired but redirect on invalid).

- [ ] In `RequireAuth`, after `v.Verify`, inspect the error:
  ```go
  switch {
  case errors.Is(err, jwt.ErrTokenExpired):
      w.Header().Set("WWW-Authenticate", `Bearer error="invalid_token", error_description="token expired"`)
      respond.Unauthorized(w, "token expired")
  default:
      respond.Unauthorized(w, "invalid token")
  }
  ```
- [ ] Test by waiting out the TTL. Confirm the `WWW-Authenticate` header now describes "token expired".
- [ ] Decide: does this leak useful info to an attacker? Mostly no — they can already check `exp` themselves by base64-decoding the payload. **Helpful for honest clients, not useful for attackers.**

---

## Task 5 — Re-use bearerToken from a different middleware

You'll want auth in more than one place eventually.

- [ ] Add a fake admin route guarded by `RequireAuth` AND a `RequireAdmin` middleware that reads `GetUserID` from the context and rejects if it isn't user id 1.
- [ ] `r.Get("/admin/secret", auth.RequireAuth(verifier))(auth.RequireAdmin)(adminHandler)` — chain them.
- [ ] Confirm: any logged-in user gets 200 on `/me` but only user 1 gets 200 on `/admin/secret`.
- [ ] **This is the 401-vs-403 distinction.** `RequireAuth` returns 401; `RequireAdmin` returns 403.

---

## Task 6 — Deletion races a valid token

- [ ] Login as user A. Save the token.
- [ ] Delete user A directly in `psql`: `DELETE FROM users WHERE id = 1;`.
- [ ] Hit `/me` with A's token. What happens?
  - The token is still cryptographically valid until `exp`.
  - But `GET /me` calls `repo.GetByID`, which returns `ErrNotFound`, which the handler maps to 401 "user no longer exists".
- [ ] This is the "JWT revocation is hard" problem. Token says "user 1"; user 1 is gone. Days 20+ (refresh tokens + a revocation list) close the loop properly.

---

## Stretch — only if you're flying

- [ ] Read the `golang-jwt/jwt/v5` README on `ParseWithClaims` and the options. Notice `WithLeeway` for clock skew, `WithAudience`, `WithJSONNumber`. Useful when you need them.
- [ ] Implement **`requireAuth` as a chi middleware that fetches the user too** (one round-trip, available on context). Trade-off: every protected handler pays for a DB lookup even if they don't need the user. Decide which you'd ship.
- [ ] Add a `?debug=1` query param that, in development only, returns the decoded claims with the response (do NOT do this in prod). Gates the leak behind `cfg.IsDev()`.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
