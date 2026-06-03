# Day 20 — Refresh Tokens with Rotation

> **Goal:** keep the access token short (15 min) for security, but give the user a way to stay logged in for weeks. Build a refresh-token flow: opaque tokens stored hashed in the DB, **rotated on every use**, with **reuse detection** that revokes the whole token family on theft.

This is the standard OAuth2 refresh pattern. Once you understand it, every "stay logged in" implementation you see makes sense.

---

## 1. Why two tokens?

Day 19 issued a 15-minute access token. That's deliberately short:

- If it leaks (browser console, a log line, XSS), the damage window is bounded.
- JWT can't really be "revoked" — it's valid until `exp`. Short expiry is the soft revocation.

But asking the user to re-enter their password every 15 minutes is hostile UX. So:

| | Access token | Refresh token |
| --- | --- | --- |
| Format | JWT (stateless, signed) | opaque random bytes (stateful, looked up) |
| Lifetime | 15 min | days–weeks (we use 30 days) |
| Sent with | every API request | only the refresh endpoint |
| Stored on server | nothing | hashed in `refresh_tokens` table |
| Can be revoked instantly | no (must wait for `exp`) | yes (DB row update) |
| Use | authorize requests | get a fresh access token |

The flow: when the access token is about to expire, the client sends its refresh token to `POST /auth/refresh` and gets back a **new** access token (and a **new** refresh token — see §3).

---

## 2. Why opaque, not JWT?

Some tutorials make the refresh token a JWT too. **Don't.** A JWT refresh token gives you nothing:

- You have to hit the DB on every refresh anyway (rotation needs the previous token's row).
- An opaque random string is much simpler — no claims, no signing, no algorithm pinning.
- 32 random bytes (256 bits) base64-encoded is unguessable by definition.

Our format: `crypto/rand` → 32 bytes → `base64.RawURLEncoding` → ~43 character URL-safe string.

---

## 3. Rotation — the heart of the pattern

**Every refresh issues a new pair (access + refresh) AND revokes the old refresh token.** The client always holds exactly one valid refresh token at a time.

```
login → RT1 (active)
refresh w/ RT1 → RT1 revoked, replaced_by = RT2 → client holds RT2
refresh w/ RT2 → RT2 revoked, replaced_by = RT3 → client holds RT3
refresh w/ RT3 → RT3 revoked, replaced_by = RT4 → client holds RT4
...
```

Each row tracks `replaced_by_id` so the chain is reconstructable. This enables the next part.

---

## 4. Reuse detection — the theft signal

Here's the magic. Say an attacker steals RT2 from the user.

```
1. user logs in        → RT1
2. user refreshes      → RT1 revoked → user holds RT2
3. attacker steals RT2 (from local storage, an XSS, etc.)
4. attacker refreshes  → RT2 revoked → attacker holds RT3
5. user eventually refreshes with their RT2:
     server sees RT2 — but it's already revoked!
     → THIS IS PROOF OF THEFT
     → revoke the entire family descending from RT2 (RT3, etc.)
     → attacker is logged out; user must re-login.
```

The server can't tell *which* party is the attacker, only that **two parties hold tokens that descend from the same root**. The safe move is to invalidate everyone in the chain. Legitimate user logs in again. Attacker is locked out.

Implemented as a recursive CTE that walks `replaced_by_id` forward and revokes every descendant. See `RevokeFamilyDescendants` in [internal/auth/refresh_repository.go](internal/auth/refresh_repository.go).

> The user has to log in once. The attacker is permanently kicked. That's the trade — and the only theft model the stateless JWT alone couldn't defeat.

---

## 5. Hashing tokens in the DB

We don't store the raw refresh token — we store its SHA-256 hash. Same idea as bcrypt for passwords, **but we use plain SHA-256, not bcrypt.** Why?

- bcrypt is slow on purpose to defeat brute-force on *weak* passwords.
- A 256-bit random refresh token cannot be brute-forced regardless of hash speed.
- SHA-256 is fast, deterministic (we need to *look up* by hash on refresh), and provides the property we actually want: a DB breach reveals only hashes, which can't be used as tokens.

```go
func hashRefreshToken(plain string) string {
    h := sha256.Sum256([]byte(plain))
    return hex.EncodeToString(h[:])
}
```

The plaintext token never appears in the DB.

---

## 6. The schema

```sql
CREATE TABLE refresh_tokens (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash     TEXT NOT NULL UNIQUE,
    expires_at     TIMESTAMPTZ NOT NULL,
    revoked_at     TIMESTAMPTZ,                              -- null = still active
    replaced_by_id BIGINT REFERENCES refresh_tokens(id),    -- rotation chain
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_refresh_tokens_user_id ON refresh_tokens(user_id);
```

- `ON DELETE CASCADE` on `user_id` → deleting a user wipes their refresh tokens.
- `token_hash UNIQUE` → fast lookup, no duplicates.
- `replaced_by_id` references self → builds the rotation chain.

---

## 7. The endpoints

| Endpoint | Public? | What it does |
| --- | --- | --- |
| `POST /auth/login` | yes | password check + issue access + issue refresh |
| `POST /auth/refresh` | yes (refresh is the auth) | validate refresh + rotate + issue new pair |
| `POST /auth/logout` | yes | revoke the refresh token |
| `GET /auth/me` | requires access token | unchanged from Day 19 |

`/refresh` is **public** because it must work after the access token has expired. The refresh token *is* the credential for this endpoint.

`/logout` is also public — possessing the refresh token is sufficient to revoke it. If you want extra strictness ("require a valid access token AND a refresh token to log out") you can flip it to protected; we keep it simple.

### Login response now includes both tokens

```json
{
  "access_token":  "eyJhbGciOi...",
  "refresh_token": "K3p9hZ4-rE...",
  "token_type":    "Bearer",
  "expires_in":    900
}
```

The client stores both. The access token goes on every API call. The refresh token only on `/refresh` and `/logout`.

---

## 8. The full flow

```
register → POST /auth/register → 201
login    → POST /auth/login    → {access, refresh, expires_in:900}
   ↓
   ... 14 minutes of API calls with the access token ...
   ↓
refresh  → POST /auth/refresh {refresh_token: rt2}
         → server validates rt2, rotates → {access', rt3, expires_in:900}
   ↓
   ... 14 minutes ...
   ↓
refresh  → POST /auth/refresh {refresh_token: rt3}  → {access'', rt4, ...}
   ↓
logout   → POST /auth/logout {refresh_token: rt4}    → 204
```

The user stays logged in for 30 days without re-entering the password. Access tokens stay 15 min. Server can revoke any session instantly by deleting/revoking the refresh token.

---

## 9. What changed from Day 19

| File | Change |
| --- | --- |
| `internal/auth/refresh.go` | **NEW** — token generation + hashing + the `RefreshToken` model |
| `internal/auth/refresh_repository.go` | **NEW** — `RefreshTokenRepository` interface + Postgres impl with `RevokeFamilyDescendants` recursive CTE |
| `internal/auth/service.go` | `Login` now also issues a refresh token; new `Refresh` (rotate) and `Logout` (revoke) methods |
| `internal/auth/handler.go` | new `POST /refresh` + `POST /logout`; `LoginResponse` gained `refresh_token` |
| `internal/auth/errors.go` | added `ErrInvalidRefreshToken` |
| `internal/config/config.go` | added `REFRESH_TTL` (default `720h` = 30 days) |
| `migrations/000002_create_refresh_tokens.*` | the new table + index |

User repository, password code, the access-token issuer/verifier, the auth middleware — all unchanged.

---

## 10. Run it

```powershell
cd Day_20_refresh_tokens
docker compose up -d
go mod init day20
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

Walk the full lifecycle:

```powershell
# register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register

# login — get BOTH tokens
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json
$access  = $r.access_token
$refresh = $r.refresh_token

# protected route works with access token
curl.exe -i -H "Authorization: Bearer $access" http://localhost:8080/auth/me

# refresh — rotate
$r2 = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$refresh\"}" http://localhost:8080/auth/refresh | ConvertFrom-Json
$access2  = $r2.access_token
$refresh2 = $r2.refresh_token   # DIFFERENT from $refresh

# the old refresh is now revoked — reuse → 401
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$refresh\"}" http://localhost:8080/auth/refresh

# logout — revokes refresh2
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$refresh2\"}" http://localhost:8080/auth/logout
# 204
```

In `psql`, watch the rows:

```powershell
docker compose exec postgres psql -U app -d appdb -c "SELECT id, user_id, expires_at, revoked_at, replaced_by_id FROM refresh_tokens ORDER BY id;"
```

You'll see the chain: id 1 revoked + replaced_by 2, id 2 revoked + replaced_by 3, id 3 revoked (logout, no replacement).

---

## 11. What's next

**Day 21** — the Week 3 mini-project: the **Notes API**. Each user sees only their own notes. Every CRUD route uses Day 19's `RequireAuth` middleware + `auth.GetUserID(r.Context())` to filter. Today's refresh + logout flow is what makes the API usable for more than 15 minutes at a stretch.

After today the auth surface is complete:
- Day 17: password storage (bcrypt)
- Day 18: access tokens (JWT)
- Day 19: middleware (verify + context)
- Day 20: refresh tokens (rotation + revocation + theft defense)

That's a production-grade auth system. Everything else (OAuth providers, 2FA, magic links) layers on top of these four primitives.
