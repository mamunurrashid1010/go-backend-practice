# Day 18 — Practice Tasks

JWTs look magical until you've decoded one by hand. These tasks make the magic boring.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day18
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

## Warm-up

- [ ] Register + login → confirm the response is `{"access_token":"...","token_type":"Bearer","expires_in":900}`.
- [ ] Copy the `access_token` into <https://jwt.io>. Observe:
  - Header has `alg: HS256`, `typ: JWT`.
  - Payload has `uid`, `email`, `iss`, `sub`, `iat`, `nbf`, `exp`.
  - Signature panel says "Invalid Signature" until you paste your `JWT_SECRET` from `.env`.
- [ ] Decode the payload by hand in PowerShell (no library):
  ```powershell
  $token = "<paste your token>"
  $payload = $token.Split(".")[1]
  # base64url -> base64 padding
  $padded = $payload + ("=" * ((4 - $payload.Length % 4) % 4))
  [Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($padded.Replace('-', '+').Replace('_', '/')))
  ```
  You should see the JSON. The lesson: **the payload is publicly readable**.

---

## Task 1 — Misconfig fails at startup

- [ ] Comment out `JWT_SECRET` in `.env`. `go run .` → fatal "JWT_SECRET is not set".
- [ ] Set `JWT_SECRET=tooshort`. `go run .` → fatal "JWT_SECRET must be at least 32 chars".
- [ ] Restore. The point: **bad secret = no server, ever**.

---

## Task 2 — Try to tamper with a token

- [ ] Take a working token. Change one character in the **payload** segment (the middle of the three).
- [ ] Try to use it (Day 19 verification doesn't exist yet, but you can verify by hand):
  ```go
  import "github.com/golang-jwt/jwt/v5"
  parsed, err := jwt.Parse(modifiedToken, func(t *jwt.Token) (any, error) {
      return []byte("<your JWT_SECRET>"), nil
  })
  // err != nil — signature is now invalid
  ```
- [ ] Confirm: **the signature catches the tampering**. Without the secret, an attacker can't make a valid signature for their modified payload.

---

## Task 3 — Inspect the standard claims

- [ ] Decode a token's payload. Confirm:
  - `iat` (issued at) ≈ now.
  - `exp` (expires at) ≈ now + 15 minutes.
  - `nbf` (not before) ≈ now.
  - `iss` matches `JWT_ISSUER`.
  - `sub` is the user ID as a *string*.
- [ ] Why is `sub` a string when `uid` is a number? Standard convention — `sub` historically held usernames. Our `uid` is the canonical int. Some libraries refuse non-string `sub`.

---

## Task 4 — Change the expiry and watch tokens age

- [ ] Set `JWT_ACCESS_TTL=30s` in `.env` and restart.
- [ ] Login. Decode payload: `exp - iat = 30`.
- [ ] You can't verify expiry until Day 19, but understand: any token issued now is dead in 30 seconds. Short TTLs are why we need refresh tokens (Day 20).
- [ ] Restore `15m`.

---

## Task 5 — Add a `role` claim

- [ ] Add a `Role string` field to `User` (DB migration: `ALTER TABLE users ADD COLUMN role TEXT NOT NULL DEFAULT 'user';`).
- [ ] Update the repo's SELECT/INSERT to include `role`.
- [ ] Add `Role string \`json:"role"\`` to `AccessTokenClaims` so it gets baked into the token.
- [ ] Set `claims.Role = u.Role` in `TokenIssuer.Issue`.
- [ ] Login and decode payload — `"role":"user"` is there.

**Why:** roles in the token = the auth middleware (Day 19) can authorize without a DB lookup. Trade-off: a role change requires re-issuing the token (until `exp` it's stale).

---

## Task 6 — A "what's in this token?" helper endpoint (temporary, demo only)

Day 19 will validate tokens properly. For today, build a throwaway debug route.

- [ ] Add `GET /auth/debug-token?token=...` that:
  ```go
  parsed, err := jwt.ParseWithClaims(tokenStr, &auth.AccessTokenClaims{},
      func(t *jwt.Token) (any, error) { return []byte(cfg.Auth.JWTSecret), nil })
  ```
- [ ] Return the claims if valid; `401` if not.
- [ ] **Delete this route before Day 19** — it's a security hole (lets anyone test arbitrary tokens against your secret).

**Why:** building a verify path by hand makes Day 19's middleware completely unsurprising.

---

## Task 7 — Rotate the secret, watch everyone get logged out

- [ ] Login → save the token.
- [ ] Change `JWT_SECRET` in `.env` to a different (still 32+ char) value. Restart.
- [ ] Once Day 19's middleware exists, the old token's signature will fail to verify with the new secret.
- [ ] Today, prove it by hand: the `jwt.Parse` from Task 2, with the **new** secret, returns "signature is invalid" on the **old** token.

**Why:** rotating `JWT_SECRET` is the nuclear "log everyone out NOW" button — useful after a breach. The cost is every session ends.

---

## Stretch — only if you're flying

- [ ] Read the JWT spec (just §1–§3): <https://www.rfc-editor.org/rfc/rfc7519>. Short, lots of "MUST/SHOULD" — fluency comes from skimming the source once.
- [ ] Try **RS256** instead of HS256. Generate a key pair:
  ```powershell
  openssl genrsa -out priv.pem 2048
  openssl rsa -in priv.pem -pubout -out pub.pem
  ```
  Sign with `jwt.SigningMethodRS256` and the parsed private key. Verifiers need only the public key. This is the multi-service pattern.
- [ ] Add the `jti` (JWT ID) claim with a random UUID. Day 20's refresh token rotation uses it for "this specific token has been revoked".

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
