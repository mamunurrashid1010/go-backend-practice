# Day 21 — Notes API (Week 3 mini-project)

A multi-user notes service. Every authenticated user sees only their own notes. Full CRUD + the Day-17-through-20 auth stack: bcrypt, JWT access tokens, refresh tokens with rotation, reuse detection.

> This is the **closer** for Week 3. Days 17–20 built the auth primitives; today they get exercised against a real feature where data isolation is the whole point.

---

## Stack

| Concern | Choice |
| --- | --- |
| Language | Go 1.22+ |
| Router | `go-chi/chi/v5` |
| DB | Postgres 16 via `database/sql` + `pgx/v5/stdlib` |
| Migrations | `golang-migrate/migrate/v4` (library mode, auto on startup) |
| Config | `caarlos0/env/v11` + `joho/godotenv` |
| Auth | `golang-jwt/jwt/v5` (HS256) + opaque refresh tokens (SHA-256 in DB) |
| Validation | `go-playground/validator/v10` |
| Layering | handler / service / repository in two packages: `auth` and `notes` |

---

## Quick start

```powershell
docker compose up -d
go mod init day21
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

You should see:

```
loaded .env
config: env=development addr=:8080 access_ttl=15m refresh_ttl=720h0m0s
migrations up: 3
connected to postgres
listening on http://localhost:8080
```

---

## API reference

### Auth (Days 17–20)

| Method | Path | Auth | Body | Status |
| --- | --- | --- | --- | --- |
| POST | `/auth/register` | — | `{email,password}` | 201, 409, 422 |
| POST | `/auth/login` | — | `{email,password}` | 200 + tokens, 401, 422 |
| POST | `/auth/refresh` | — | `{refresh_token}` | 200 + new tokens, 401, 422 |
| POST | `/auth/logout` | — | `{refresh_token}` | 204 |
| GET | `/auth/me` | Bearer | — | 200, 401 |

### Notes (today)

**All endpoints require `Authorization: Bearer <access_token>`.**

| Method | Path | Body | Status |
| --- | --- | --- | --- |
| GET | `/notes` | — | 200 |
| GET | `/notes?search=foo&limit=10` | — | 200, 400, 422 |
| POST | `/notes` | `{title,body}` | 201, 401, 422 |
| GET | `/notes/{id}` | — | 200, 401, 404 |
| PUT | `/notes/{id}` | `{title,body}` | 200, 401, 404, 422 |
| PATCH | `/notes/{id}` | `{title?,body?}` | 200, 400, 401, 404, 422 |
| DELETE | `/notes/{id}` | — | 204, 401, 404 |

Error envelope (same as previous days):

```json
{ "error": { "code": "NOT_FOUND", "message": "note not found" } }
```

---

## The multi-tenant trick — IDOR defense

This is the security idea that matters. Every notes query has **two** WHERE clauses:

```sql
SELECT id, title, body, ... FROM notes WHERE id = $1 AND user_id = $2
DELETE FROM notes WHERE id = $1 AND user_id = $2
UPDATE notes SET ... WHERE id = $1 AND user_id = $2 RETURNING ...
```

If user A tries to `GET /notes/42` and note 42 belongs to user B, the row doesn't match → `404 NOT_FOUND`. Same response as if note 42 didn't exist at all.

**You never compare `note.UserID == ctxUserID` in the service.** You bake `user_id = $N` into every query. That way:

1. A new endpoint can't accidentally forget the check — there's no in-memory `if` to skip.
2. The DB does the filtering, so a "list" can't include foreign rows.
3. Probing IDs reveals nothing — every miss looks the same.

This is the canonical fix for **IDOR** (Insecure Direct Object Reference) — one of the OWASP Top 10.

---

## Project layout

```
Day_21_notes_api/
├── docker-compose.yml
├── .env.example
├── .env                              # gitignored
├── main.go                           # config → DB → migrate → wire layers → serve (graceful shutdown)
├── migrations/
│   ├── 000001_create_users.up.sql
│   ├── 000002_create_refresh_tokens.up.sql
│   └── 000003_create_notes.up.sql    # the new table
└── internal/
    ├── config/
    ├── middleware/                   # Recover + RequestID + Logger
    ├── respond/
    ├── validate/
    ├── auth/                         # carried from Day 20 — register, login, refresh, logout, /me, RequireAuth, GetUserID
    └── notes/                        # NEW today
        ├── notes.go                  # Note + DTOs + ListFilter
        ├── errors.go                 # ErrNotFound (= "not yours or doesn't exist")
        ├── repository.go             # Repository interface + Postgres impl (every query has WHERE user_id)
        ├── service.go                # business rules (none beyond PATCH "nothing to update")
        └── handler.go                # routes; reads userID from auth.GetUserID(ctx)
```

`main.go` is wiring only:

```go
notesRepo := notes.NewPostgresRepository(db)
notesSvc := notes.NewService(notesRepo)
notesHandler := &notes.Handler{Svc: notesSvc}

r.Group(func(r chi.Router) {
    r.Use(auth.RequireAuth(verifier))   // every route inside needs a valid token
    r.Mount("/notes", notesHandler.Router())
})
```

---

## Full curl walkthrough

```powershell
# 1. register two users
curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@x.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register | Out-Null
curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"b@x.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/register | Out-Null

# 2. log in as A — grab the access token
$tokA = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@x.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# 3. A creates a note → 201, Location: /notes/1
curl.exe -i -H "Authorization: Bearer $tokA" -H "Content-Type: application/json" `
  -d "{\"title\":\"my first note\",\"body\":\"hello\"}" http://localhost:8080/notes

# 4. A lists her notes → 1 row
curl.exe -i -H "Authorization: Bearer $tokA" http://localhost:8080/notes

# 5. log in as B — different user
$tokB = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"b@x.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# 6. B tries to GET A's note → 404 NOT_FOUND (the IDOR test)
curl.exe -i -H "Authorization: Bearer $tokB" http://localhost:8080/notes/1

# 7. B lists her notes → empty array
curl.exe -i -H "Authorization: Bearer $tokB" http://localhost:8080/notes

# 8. B creates one — own note → 201, Location: /notes/2
curl.exe -i -H "Authorization: Bearer $tokB" -H "Content-Type: application/json" `
  -d "{\"title\":\"B's note\"}" http://localhost:8080/notes

# 9. A still sees only A's notes; B still sees only B's. PERFECT ISOLATION.
curl.exe -s -H "Authorization: Bearer $tokA" http://localhost:8080/notes
curl.exe -s -H "Authorization: Bearer $tokB" http://localhost:8080/notes
```

Step 6 is the moment. User B authenticated successfully — that part of the system trusts B. But the **query** with `WHERE user_id = B's_id` returns no rows for note 1, so the API returns 404. B can't even tell whether note 1 exists.

---

## What this project demonstrates (Week 3 in summary)

- **Day 15** — validator → every DTO has tags; 422 with field details on bad input.
- **Day 16** — typed errors → repository translates `sql.ErrNoRows`/`23505` to domain errors; handler maps once.
- **Day 17** — bcrypt → passwords never stored plaintext; `json:"-"` keeps the hash off the wire.
- **Day 18** — JWT issuance → login returns OAuth2-shaped `{access_token, refresh_token, token_type, expires_in}`.
- **Day 19** — auth middleware → `RequireAuth` validates signature/issuer/exp, puts `userID` on context.
- **Day 20** — refresh tokens → rotation with `replaced_by_id`, reuse detection via recursive CTE.
- **Day 21 (today)** — multi-user feature with **every query filtered by `user_id` from the verified token**.

The lesson Week 3 builds: **auth is a substrate, not a feature.** Days 17–20 set up the chain so today's notes service can be 7 SQL queries that simply add `AND user_id = $N`. There's no per-handler "is this their note?" check — there can't be, because the data layer enforces it.

---

## What's NOT here yet (honest about scope)

- **Tests** — Week 4 (Days 22–24) adds unit, handler, and integration tests. The layering is ready.
- **Structured logging** — Day 25 (`slog`). Today: `log.Printf`.
- **Pagination beyond `limit`** — Day 26 brings cursor pagination.
- **Rate limiting + CORS** — Day 27.
- **2FA / email verification / password reset** — out of scope; layer on top of the existing primitives.
- **Sharing a note with another user** — would mean a `note_shares` table and a more interesting query. The shape stays the same.

---

## Configuration

| Var | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | dev / staging / production |
| `HTTP_ADDR` | `:8080` | |
| `HTTP_*_TIMEOUT` | various | server timeouts |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | drain window on Ctrl+C |
| `DATABASE_URL` | **required** | Postgres DSN |
| `DB_*` | various | pool tuning |
| `JWT_SECRET` | **required, ≥32 chars** | HMAC secret |
| `JWT_ACCESS_TTL` | `15m` | short for security |
| `JWT_ISSUER` | `day21-auth` | pinned by the verifier |
| `REFRESH_TTL` | `720h` | 30 days; must be > access TTL |

---

## What's next

**Week 4** — quality: tests, structured logging, pagination, rate limiting, polish. The handler/service/repository layering pays off in Day 22 (unit-test the service with a mock repo) and Day 23 (handler tests with `httptest`). Day 24 spins up `testcontainers` Postgres for full integration tests.

After Week 3 you have a production-grade auth + first multi-user feature. After Week 4 you have it under test.
