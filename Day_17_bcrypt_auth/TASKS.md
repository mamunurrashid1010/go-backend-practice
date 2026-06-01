# Day 17 — Practice Tasks

Auth is where mistakes have the highest blast radius. These tasks build the right instincts.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day17
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
> go run .
> ```

---

## Warm-up — confirm the security properties

- [ ] Register `a@b.com` / `hunter2pass` → `201`. The response body has **no** password or hash field.
- [ ] In `psql`: `SELECT email, password_hash FROM users;` — confirm `password_hash` starts with `$2a$10$` and is NOT the plaintext.
- [ ] Register `a@b.com` again → `409 CONFLICT`.
- [ ] Register `A@B.COM` (uppercase) → also `409` — normalisation makes it the same account.
- [ ] Login with the right password → `200`. Wrong password → `401`. **Unknown email → also `401` with the identical message.**
- [ ] Register with `password=short` (< 8) → `422` with a field detail.

---

## Task 1 — Add a password-confirm field

- [ ] Add `Confirm string` to `RegisterRequest` with `validate:"required,eqfield=Password"`.
- [ ] Test: mismatched confirm → `422 "must match Password"`.
- [ ] Note: `eqfield` references the **Go field name** (`Password`), not the JSON name.

---

## Task 2 — Tune the bcrypt cost and feel it

- [ ] In `password.go`, temporarily change `bcrypt.DefaultCost` (10) to `14`.
- [ ] Register a user and time it (`Measure-Command { curl.exe ... }`). Cost 14 = 16× slower than 10 — you'll feel ~1 second.
- [ ] Set it back to `DefaultCost`. The lesson: higher cost = more brute-force resistance but slower logins. 10–12 is the sweet spot in 2026.

---

## Task 3 — Close the timing side-channel

Right now, a missing email returns *faster* than a wrong password (it skips bcrypt). An attacker can measure this to enumerate emails.

- [ ] In `service.Login`, when `GetByEmail` returns `ErrNotFound`, run a **dummy** bcrypt comparison before returning `ErrInvalidCredentials`:
  ```go
  // a precomputed hash of a random string, package-level
  var dummyHash = mustHash("a-fixed-dummy-password")
  // in Login's not-found branch:
  CheckPassword(dummyHash, in.Password)   // burn ~the same time
  return User{}, ErrInvalidCredentials
  ```
- [ ] Now both paths take ~the same time. (Time both with `Measure-Command` to confirm.)

**Why:** real auth systems do exactly this. Equal-time failure is the gold standard.

---

## Task 4 — Add a `GET /auth/me`-style lookup (no auth yet)

You can't really do "me" without a token (Day 18), but practise the repo path:

- [ ] Add `GET /auth/users/{id}` that calls `repo.GetByID` and returns the user (no hash).
- [ ] `404` if missing. This route goes away once Day 19's middleware can derive the user from a token — but it's good repo practice now.

---

## Task 5 — An in-memory user repository for tests

- [ ] Write `InMemoryUserRepository` implementing `UserRepository` (a `map[string]User` keyed by email + a `map[int64]User`).
- [ ] Return `*ConflictError` on duplicate email, `ErrNotFound` on missing.
- [ ] Write `internal/auth/service_test.go`:
  - `Register` then `Login` with the right password → success.
  - `Login` with wrong password → `ErrInvalidCredentials`.
  - `Login` with unknown email → `ErrInvalidCredentials` (same error).
  - `Register` twice → `*ConflictError`.
- [ ] `go test ./internal/auth/...` — green.

**Why:** Day 22 is all about this. Auth logic is exactly what you want under test — it's security-critical and pure (no HTTP).

---

## Task 6 — Reason about what login should return (Day 18 setup)

A thinking task.

- [ ] Right now `POST /auth/login` returns the `User`. That's useless to a client — there's no way to "stay logged in"; the next request is anonymous again.
- [ ] What does the client need back so its *next* request proves "I'm user 1"? (Answer: a token — Day 18.)
- [ ] Sketch the shape you'd want: `{"access_token":"...","token_type":"Bearer","expires_in":900}`. Write 3 lines in "What I learned" about why a token beats sending email+password on every request.

---

## Stretch — only if you're flying

- [ ] Read the OWASP "Password Storage Cheat Sheet" intro: <https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html>. Note where bcrypt, scrypt, and argon2 sit. (argon2 is the modern winner; bcrypt is still perfectly fine and simpler.)
- [ ] Try `golang.org/x/crypto/argon2` for one hash. Notice you must store salt + params yourself (bcrypt bundles them for you — a real ergonomic win).
- [ ] Add a `CHECK (length(email) <= 254)` constraint (the RFC max email length) via a migration. Defense in depth at the DB.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
