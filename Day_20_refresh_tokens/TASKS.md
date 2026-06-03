# Day 20 — Practice Tasks

The rotation pattern is subtle. These tasks make every part of it concrete by hand.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day20
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

## Warm-up — the happy path

```powershell
# register + login
curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register | Out-Null
$r1 = curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json
$rt1 = $r1.refresh_token

# rotate
$r2 = curl.exe -s -H "Content-Type: application/json" -d "{\"refresh_token\":\"$rt1\"}" http://localhost:8080/auth/refresh | ConvertFrom-Json
$rt2 = $r2.refresh_token

# inspect the chain in psql
docker compose exec postgres psql -U app -d appdb -c "SELECT id, user_id, expires_at, revoked_at, replaced_by_id FROM refresh_tokens ORDER BY id;"
```

You should see id 1 revoked + `replaced_by_id = 2`, id 2 active.

- [ ] All commands behave as described.
- [ ] Confirm in `psql`: `token_hash` is a 64-char hex string (SHA-256), not the plaintext.

---

## Task 1 — The reuse-detection trap

This is the day's most important task.

- [ ] You still have `$rt1` (revoked) and `$rt2` (active). Attempt to refresh with `$rt1`:
  ```powershell
  curl.exe -i -H "Content-Type: application/json" -d "{\"refresh_token\":\"$rt1\"}" http://localhost:8080/auth/refresh
  # 401 — invalid or expired refresh token
  ```
- [ ] Check `psql` again. **`$rt2` is now also revoked.** The reuse of `$rt1` triggered `RevokeFamilyDescendants`, which walked the chain and killed everything that descended from `$rt1`.
- [ ] Confirm `$rt2` no longer works either:
  ```powershell
  curl.exe -i -H "Content-Type: application/json" -d "{\"refresh_token\":\"$rt2\"}" http://localhost:8080/auth/refresh
  # 401
  ```
- [ ] The legitimate user must log in again. The attacker (in this story, your earlier "you") is also kicked.

Write 3 lines about this in your "What I learned" section.

---

## Task 2 — Logout is idempotent

- [ ] Login. Save the refresh token.
- [ ] Logout once → 204.
- [ ] Logout AGAIN with the same token → still 204 (not 4xx).
- [ ] Why? Idempotent logout is friendly to client retry logic. Lookup-and-no-op is the right behaviour.

---

## Task 3 — Refresh expiry

- [ ] Set `REFRESH_TTL=20s` in `.env`. Restart.
- [ ] Login. Wait 25s. Try to refresh.
- [ ] `401`. The `expires_at` check fires.
- [ ] Restore `720h`.

---

## Task 4 — Same user, two devices

Refresh tokens are per-session. A user logging in twice (phone + laptop) gets two independent chains.

- [ ] Login twice with the same email → two different `refresh_token` values, two different rows in `refresh_tokens`.
- [ ] Logout the phone (the first token) → the laptop is unaffected.
- [ ] Reuse-detect the phone token → only its chain dies; the laptop is fine.
- [ ] In `psql`:
  ```sql
  SELECT id, user_id, revoked_at, replaced_by_id FROM refresh_tokens WHERE user_id = 1 ORDER BY id;
  ```
  Two independent chains for one user.

---

## Task 5 — "Log everyone out" (nuclear button)

Add a service method `RevokeAllForUser(ctx, userID)` and a protected route `POST /auth/sessions/revoke-all` that calls it.

- [ ] In `RefreshTokenRepository`, add:
  ```go
  RevokeAllForUser(ctx context.Context, userID int64) error
  ```
  Postgres impl:
  ```sql
  UPDATE refresh_tokens SET revoked_at = now()
  WHERE user_id = $1 AND revoked_at IS NULL;
  ```
- [ ] Service method `RevokeAllSessions(ctx, userID)`.
- [ ] Handler reads `userID` from context (Day 19 pattern), calls the service.
- [ ] Test: login twice. Hit revoke-all. Both refresh tokens dead.

**Why:** "log me out everywhere" is the password-reset companion. The user resets their password and this is the next call.

---

## Task 6 — A periodic cleanup job (sketch)

Revoked + expired rows pile up forever. Real apps prune them.

- [ ] Sketch (don't fully build) a function `PruneExpired(ctx) (int, error)` that deletes rows where `expires_at < now() - 30 days`.
- [ ] Where would you call it? On startup? On a timer? A daily cron? (Day 56's `robfig/cron` style.) Form an opinion.
- [ ] **Don't** prune `revoked_at` rows immediately — keeping them around for a window is what powers reuse detection.

---

## Task 7 — Read what's stored

A check that nothing leaks.

- [ ] `psql`: `SELECT token_hash FROM refresh_tokens LIMIT 1;` — confirm it's a 64-char hex string, NOT the plaintext you saw in the login response.
- [ ] Hash one of your plaintext tokens by hand:
  ```powershell
  $rt = "<your refresh token>"
  $bytes = [Text.Encoding]::UTF8.GetBytes($rt)
  $sha = [Security.Cryptography.SHA256]::Create()
  $hash = $sha.ComputeHash($bytes)
  -join ($hash | ForEach-Object { '{0:x2}' -f $_ })
  ```
- [ ] Confirm: that hash equals the DB's `token_hash`. Lesson: even with the DB, an attacker can't impersonate users — they only see hashes.

---

## Stretch — only if you're flying

- [ ] Read the OAuth 2.0 RFC §6 "Refreshing an Access Token": <https://www.rfc-editor.org/rfc/rfc6749#section-6>. We follow it.
- [ ] Add a `device_label` column (`refresh_tokens.device_label TEXT`). Show a "my sessions" page that lists `(device_label, created_at)` so the user can revoke individual sessions from a UI.
- [ ] Refresh-token rotation introduces a tiny race window: if two refreshes from the same client cross in flight, one wins and one triggers reuse detection (locking the legitimate user out). Read about how Auth0 and Okta handle this (e.g., a 30-second grace period). Decide if you'd add that.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
