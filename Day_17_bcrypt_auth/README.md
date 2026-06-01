# Day 17 — Password Hashing with bcrypt (Register + Login)

> **Goal:** store passwords the only acceptable way — hashed with bcrypt, never plaintext. Build `POST /auth/register` and `POST /auth/login` that verify credentials. **No JWT yet** (Day 18) — login just confirms the password is correct.

This is a fresh, auth-focused module. The To-Do app stays at Day 16; from here Week 3 builds toward the Notes API (Day 21) where each user sees only their own data.

---

## 1. Never store plaintext passwords

If your `users` table stores `password = 'hunter2'` and the DB leaks (backups, SQL injection, a rogue admin, a misconfigured replica), **every user's password is exposed** — and because people reuse passwords, their email/bank/everything is now at risk too.

The rule, no exceptions: **store a one-way hash, never the password.** When a user logs in, you hash what they typed and compare hashes. You never need the original.

Even hashing isn't enough by itself — a plain `SHA-256(password)` is crackable in seconds with a GPU and a rulebook of common passwords. You need a hash that's:

1. **Slow on purpose** (so brute-force is expensive).
2. **Salted** (so two users with the same password get different hashes, defeating rainbow tables).
3. **Adaptive** (you can crank the cost up as hardware gets faster).

bcrypt is all three.

---

## 2. Why bcrypt

`golang.org/x/crypto/bcrypt` gives you a battle-tested implementation:

```go
hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.DefaultCost)
// hash looks like: $2a$10$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy
```

Decode that string:

```
$2a$ 10 $ N9qo8uLOickgx2ZMRZoMye IjZAgcfl7p92ldGxad68LJZdL17lhWy
 │   │          │                         │
 │   │          └ salt (22 chars)         └ the actual hash
 │   └ cost (work factor) = 2^10 rounds
 └ bcrypt version
```

**Everything bcrypt needs to verify a password is embedded in that one string** — version, cost, salt, hash. You store just this string. No separate salt column.

### Cost / work factor

`bcrypt.DefaultCost` is 10 (= 2¹⁰ rounds). Higher = slower = more brute-force-resistant, but slower logins. 10–12 is the common range. The point of bcrypt being *slow* (~50–100ms per hash) is the feature: an attacker who steals your hashes can only try a few thousand guesses per second per core, not billions.

### The 72-byte limit

bcrypt only looks at the **first 72 bytes** of the input. Anything past that is silently ignored. That's why our `RegisterRequest` validates `password max=72` (Day 15 set this up). Longer "passwords" (passphrases) need a pre-hash step (SHA-256 then bcrypt) — out of scope today, but that's *why* the limit exists.

---

## 3. The `password.go` helpers

[internal/auth/password.go](internal/auth/password.go) wraps bcrypt in two functions:

```go
func HashPassword(plain string) (string, error) {
    b, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
    return string(b), err
}

func CheckPassword(hash, plain string) bool {
    return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain)) == nil
}
```

`CompareHashAndPassword` does a **constant-time** comparison — it takes the same time whether the password is wrong at character 1 or character 50. That defeats timing attacks. Never compare hashes with `==`.

---

## 4. The `users` table

[migrations/000001_create_users.up.sql](migrations/000001_create_users.up.sql):

```sql
CREATE TABLE users (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    email         TEXT        NOT NULL UNIQUE,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

- `email UNIQUE` → the DB enforces "one account per email". A duplicate insert is a `23505` → our `ConflictError` from Day 16 → `409`.
- `password_hash` stores the bcrypt string. **Never** a column called `password`.

---

## 5. The register flow

```
POST /auth/register  {"email":"a@b.com","password":"hunter2pass"}
   │
   ├─ validate DTO (email format, password 8..72)            → 422 on failure
   ├─ normalise email (lowercase + trim)
   ├─ HashPassword(password)                                  → bcrypt string
   ├─ repo.Create(email, hash)                                → 409 if email taken
   └─ 201 Created  {"id":1,"email":"a@b.com","created_at":...} ← NO password field
```

The response is the `User` struct, whose `PasswordHash` field has `json:"-"` — so the hash **physically cannot** appear in any JSON response. (Day 3 taught this tag; today it's load-bearing security.)

---

## 6. The login flow — and the security subtlety

```
POST /auth/login  {"email":"a@b.com","password":"hunter2pass"}
   │
   ├─ validate DTO                                            → 422 on failure
   ├─ normalise email
   ├─ repo.GetByEmail(email)
   │     ├─ not found → return ErrInvalidCredentials  (NOT "email not found")
   │     └─ found     → CheckPassword(hash, password)
   │                       ├─ false → return ErrInvalidCredentials
   │                       └─ true  → success
   └─ 200 OK  {"id":1,"email":"a@b.com",...}
```

**The critical detail:** "email doesn't exist" and "password is wrong" return the **exact same error** (`ErrInvalidCredentials` → `401`). If you returned `404 "no such email"` for unknown emails and `401 "wrong password"` for known ones, an attacker could **enumerate which emails are registered** by watching the status codes. Same response for both closes that hole.

(There's a subtler timing version — a missing email skips the bcrypt check and returns faster, leaking existence via response time. Mitigating that fully means hashing a dummy password even when the email is missing. Task 5 explores it.)

---

## 7. Email normalisation

`A@B.com`, `a@b.com`, and ` a@b.com ` are the same account to a human. We normalise in the service:

```go
email = strings.ToLower(strings.TrimSpace(in.Email))
```

…before both `Create` and `GetByEmail`, so registration and login agree. (Production sometimes goes further — stripping `+tags` in Gmail addresses — but lowercase+trim covers the common case.)

---

## 8. What's NOT here yet

- **JWT / tokens** (Day 18) — login confirms the password but doesn't issue a session/token yet. There's no "stay logged in".
- **Auth middleware** (Day 19) — no protected routes yet.
- **Refresh tokens** (Day 20).
- **Login rate limiting** (Day 27) — right now you could brute-force the login endpoint. bcrypt's slowness helps, but rate limiting is the real defence.
- **Password reset, email verification** — beyond the plan's scope.

---

## 9. Run it

```powershell
cd Day_17_bcrypt_auth
docker compose up -d
go mod init day17
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
go run .
```

Walk the flows:

```powershell
# register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register
# 201, body has id + email + created_at, NO password

# register again → 409 conflict
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"whatever1\"}" http://localhost:8080/auth/register

# login OK
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login
# 200

# login wrong password → 401 "invalid email or password"
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"a@b.com\",\"password\":\"wrongpass\"}" http://localhost:8080/auth/login

# login unknown email → ALSO 401, SAME message (no enumeration)
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"ghost@b.com\",\"password\":\"whatever1\"}" http://localhost:8080/auth/login
```

Then confirm in `psql` that the stored value is a bcrypt hash, never the password:

```powershell
docker compose exec postgres psql -U app -d appdb -c "SELECT id, email, password_hash FROM users;"
# password_hash starts with $2a$10$...
```

---

## 10. What's next

**Day 18** — JWT theory + issuing access tokens on login. Today's `Login` returns the user; tomorrow it returns a signed token the client sends on future requests. Then Day 19 wires an auth middleware that validates the token and injects `userID` into the request context — the same context-passing you built on Day 6.
