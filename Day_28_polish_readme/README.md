# Notes API — Go + Postgres + JWT

[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![Postgres](https://img.shields.io/badge/postgres-16-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)
[![CI](https://img.shields.io/badge/CI-status%20placeholder-lightgrey)]()

A production-shaped Go REST API for personal notes. Multi-user, JWT-authenticated, cursor-paginated, rate-limited, CORS-aware, structured-logged, fully tested. **Day 28 is the polish day**: the README, the file layout, and the small refactors that make a hobby repo readable by a stranger.

> This is the Week 4 closer of a 15-week learning plan. The previous 27 days are folders next to this one; each day's `README.md` teaches one concept on top of the codebase that came before it. Day 28 keeps the surface area of Day 27 — it doesn't add a feature, it makes the result *clonable*.

---

## What it does

- `POST /auth/register` → create a user.
- `POST /auth/login` → get an access token (15 min) + opaque refresh token (30 days).
- `POST /auth/refresh` → trade a refresh token for a new pair (with rotation + reuse detection).
- `POST /auth/logout` → revoke a refresh token.
- `GET /auth/me` → read the current user.
- `GET /notes`, `POST /notes`, `GET/PUT/PATCH/DELETE /notes/{id}` → full notes CRUD, scoped to the authenticated user.
- Cursor pagination with `?after=…&limit=…&sort=desc|asc&search=…`.
- Rate limit (token bucket) + CORS (allowlist) on every route, with a tighter bucket on `/auth/register` and `/auth/login`.
- Structured JSON logs in production (`log/slog`), human-friendly text in development.
- Graceful shutdown on SIGINT/SIGTERM.

Everything below is implemented; the [API reference](#api-reference) walks the whole surface.

---

## Quick start

You need **Go 1.22+** and **Docker** (for Postgres).

```powershell
git clone <this repo>
cd Day_28_polish_readme
cp .env.example .env                 # copy the dev defaults
docker compose up -d                 # postgres on host port 5433
go mod init day28
go mod tidy                          # pull every dep listed below
go run .
```

You should see:

```
loaded .env
config env=development addr=:8080 ...
connected to postgres
migrations up version=4
listening addr=http://localhost:8080
```

Hit it:

```powershell
curl.exe http://localhost:8080/healthz
# {"status":"ok"}
```

### Dependencies pulled by `go mod tidy`

- [`go-chi/chi/v5`](https://github.com/go-chi/chi) — router
- [`jackc/pgx/v5`](https://github.com/jackc/pgx) — Postgres driver
- [`golang-migrate/migrate/v4`](https://github.com/golang-migrate/migrate) — migrations
- [`joho/godotenv`](https://github.com/joho/godotenv) + [`caarlos0/env/v11`](https://github.com/caarlos0/env) — config
- [`go-playground/validator/v10`](https://github.com/go-playground/validator) — input validation
- [`golang-jwt/jwt/v5`](https://github.com/golang-jwt/jwt) + [`golang.org/x/crypto/bcrypt`](https://pkg.go.dev/golang.org/x/crypto/bcrypt) — auth
- [`golang.org/x/time/rate`](https://pkg.go.dev/golang.org/x/time/rate) — rate limiting

---

## Full curl walkthrough

A complete loop — register, login, create a note, refresh, list with pagination, log out.

```powershell
# 1. Register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"alice@example.com\",\"password\":\"hunter2pass\"}" `
  http://localhost:8080/auth/register
# 201 Created, Location: /users/1

# 2. Login (save the tokens)
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"email\":\"alice@example.com\",\"password\":\"hunter2pass\"}" `
  http://localhost:8080/auth/login | ConvertFrom-Json
$tok = $r.access_token
$ref = $r.refresh_token

# 3. Create a note
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -d "{\"title\":\"buy milk\",\"body\":\"and bread\"}" http://localhost:8080/notes
# 201 Created, Location: /notes/1

# 4. List with pagination
curl.exe -s -H "Authorization: Bearer $tok" `
  "http://localhost:8080/notes?limit=10" | ConvertFrom-Json
# { "data": [ ... ], "next_cursor": "<base64>" }

# 5. Refresh — rotates the refresh token
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$ref\"}" `
  http://localhost:8080/auth/refresh | ConvertFrom-Json
$tok = $r.access_token; $ref = $r.refresh_token

# 6. Logout — revokes the refresh token
curl.exe -i -X POST -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$ref\"}" http://localhost:8080/auth/logout
# 204 No Content
```

---

## API reference

Every response is JSON. Errors share one envelope:

```json
{ "error": { "code": "NOT_FOUND", "message": "note not found" } }
```

Every response carries `X-Request-ID`. Rate-limited routes carry `X-RateLimit-Limit` and `X-RateLimit-Remaining`; 429 responses add `Retry-After`.

### Auth

| Method | Path | Body | Notes |
| --- | --- | --- | --- |
| POST | `/auth/register` | `{email, password}` | 201, returns the user. |
| POST | `/auth/login` | `{email, password}` | 200, returns `{access_token, refresh_token, token_type, expires_in}`. |
| POST | `/auth/refresh` | `{refresh_token}` | 200, returns new pair; **old refresh token is revoked**. Re-using a revoked token kills the family. |
| POST | `/auth/logout` | `{refresh_token}` | 204. |
| GET | `/auth/me` | — (Bearer) | 200, returns the current user. |

### Notes (all require `Authorization: Bearer <access_token>`)

| Method | Path | Body | Notes |
| --- | --- | --- | --- |
| GET | `/notes` | — | List. Query: `search`, `limit` (1..100, default 20), `after` (cursor), `sort` (desc\|asc). Returns `{data, next_cursor?}`. |
| POST | `/notes` | `{title, body?}` | 201, Location header. |
| GET | `/notes/{id}` | — | 200 or 404. |
| PUT | `/notes/{id}` | `{title, body}` | Full replace. |
| PATCH | `/notes/{id}` | `{title?, body?}` | At least one field required. |
| DELETE | `/notes/{id}` | — | 204. |

**404 enforces tenant isolation** — asking for another user's note returns the same 404 as a non-existent id, by design. The data exists in one table; the API enforces ownership at every query.

---

## Project layout

```
Day_28_polish_readme/
├── docker-compose.yml             # Postgres 16-alpine on host port 5433
├── .env.example                   # committed template; copy to .env
├── .env                           # local values; gitignored
├── .gitignore
├── LICENSE                        # MIT
├── Makefile                       # make up / migrate / test / lint
├── main.go                        # config -> db -> migrate -> wire -> serve -> shutdown
├── migrations/                    # golang-migrate, auto-applied on startup
│   ├── 000001_create_users.{up,down}.sql
│   ├── 000002_create_refresh_tokens.{up,down}.sql
│   ├── 000003_create_notes.{up,down}.sql
│   └── 000004_index_notes_user_id_desc.{up,down}.sql
└── internal/
    ├── config/                    # caarlos0/env Config with validation
    ├── logging/                   # slog wrapper + context helpers
    ├── respond/                   # consistent error envelope + writers
    ├── validate/                  # go-playground/validator wrapper
    ├── httpjson/                  # *NEW* shared JSON decode helper
    ├── ratelimit/                 # token-bucket limiter with TTL eviction
    ├── middleware/                # RequestID, Logger, Recover, CORS, RateLimit
    ├── auth/                      # user model, password, JWT, refresh tokens
    │   ├── user.go errors.go password.go
    │   ├── repository.go          # users
    │   ├── refresh.go refresh_repository.go
    │   ├── token.go               # JWT issuer + verifier
    │   ├── service.go             # register / login / refresh / logout / me
    │   ├── middleware.go          # RequireAuth -> userID in context
    │   └── handler.go
    └── notes/
        ├── notes.go errors.go cursor.go
        ├── repository.go          # cursor SQL + LIMIT N+1 trick
        ├── service.go
        └── handler.go
```

Reading order for someone new: [main.go](main.go) → [internal/auth/service.go](internal/auth/service.go) → [internal/notes/repository.go](internal/notes/repository.go). The rest follows.

---

## Configuration

All knobs are environment variables (see [.env.example](.env.example)). The full list:

| Var | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `development` \| `staging` \| `production` |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `HTTP_ADDR` | `:8080` | |
| `HTTP_*_TIMEOUT` | 10s / 10s / 60s / 10s | read / write / idle / shutdown |
| `DATABASE_URL` | **required** | Postgres DSN |
| `DB_MAX_OPEN_CONNS` | `25` | |
| `DB_MAX_IDLE_CONNS` | `10` | |
| `DB_CONN_MAX_LIFETIME` | `5m` | |
| `DB_CONN_MAX_IDLE_TIME` | `2m` | |
| `DB_PING_TIMEOUT` | `5s` | fail-fast startup |
| `JWT_SECRET` | **required, ≥32 chars** | HS256 signing key |
| `JWT_ACCESS_TTL` | `15m` | |
| `JWT_ISSUER` | `day28-auth` | |
| `REFRESH_TTL` | `720h` (30 days) | must be > `JWT_ACCESS_TTL` |
| `RATE_LIMIT_GLOBAL_RPS` / `_BURST` | `10` / `30` | every route |
| `RATE_LIMIT_AUTH_RPS` / `_BURST` | `1` / `5` | only `/auth/register`, `/auth/login` |
| `RATE_LIMIT_TTL` | `5m` | visitor map eviction |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | comma-separated; no `*` with credentials |
| `CORS_ALLOWED_METHODS` | `GET,POST,PUT,PATCH,DELETE,OPTIONS` | |
| `CORS_ALLOWED_HEADERS` | `Authorization,Content-Type,X-Request-ID` | |
| `CORS_EXPOSED_HEADERS` | `X-Request-ID,X-RateLimit-Limit,X-RateLimit-Remaining` | |
| `CORS_MAX_AGE` | `5m` | preflight cache |
| `CORS_ALLOW_CREDENTIALS` | `true` | |

Bad config fails at startup with a clear error — the server doesn't come up half-broken.

---

## Architecture in one paragraph

The handler layer parses HTTP and is the only place that knows about `http.Request`/`http.ResponseWriter`. The service layer enforces business rules and depends on a `Repository` interface, never on Postgres directly. The Postgres repository is one implementation of that interface; the fake repository in tests is another. Every cross-layer call passes a `context.Context` so cancellation, deadlines, request ID, and user ID flow end-to-end. Errors travel as wrapped Go errors with sentinels (`ErrNotFound`, `ErrInvalidCredentials`, …) at the boundary; the handler maps them to HTTP status codes. There is no global mutable state — config, logger, and `*sql.DB` are constructed once in `main.go` and passed in.

---

## Testing

```powershell
go test ./...                           # all unit + handler tests
go test -tags integration ./...         # adds testcontainers-go integration tests
```

The repo is unusually well covered for its size — every layer has its own tests:

- `internal/auth` — password hashing, token issue/verify, refresh rotation, reuse detection.
- `internal/notes` — service (table-driven, fake repo), handler (httptest), cursor encode/decode, response wrapping.
- `internal/ratelimit` — burst+refill, key independence, eviction.
- `internal/middleware` — CORS preflight, allowed/disallowed origin, plain OPTIONS fall-through.

Integration tests (Day 24) spin up real Postgres via testcontainers and exercise the Postgres repositories end-to-end.

---

## Make / scripts

A `Makefile` is included for one-letter habits:

```
make up         # docker compose up -d
make down       # docker compose down
make migrate    # run migrations against $DATABASE_URL
make run        # go run .
make test       # go test ./...
make test-int   # go test -tags integration ./...
make tidy       # go mod tidy
make fmt        # go fmt ./...
make vet        # go vet ./...
```

`make` works on Windows via `make.exe` or in a Git Bash shell; the same commands are spelled out in PowerShell in [TASKS.md](TASKS.md) if you don't have make.

---

## What's intentionally NOT here

Honest scope. These come in the next phases of the plan:

- **Transactions** (Day 29) — currently single-statement repo methods. Cross-statement operations would need explicit `db.BeginTx`.
- **N+1 / EXPLAIN / pool tuning** (Day 30) — pool is conservative; query plans haven't been audited at scale.
- **`sqlc`** (Day 31) — repositories still hand-roll `rows.Scan`.
- **Redis** (Day 32–33) — no caching, no distributed rate limit, no idempotency keys.
- **OpenAPI / Swagger** (Day 34) — this README is the doc.
- **CI/CD** (Day 42) — the badge above is a placeholder.
- **Tracing / Prometheus / Sentry** (Days 89–91).

The codebase is layered the way it is so these slot in without churn.

---

## License

[MIT](LICENSE).

---

## Acknowledgements

This is the Week 4 ship of a personal 15-week Go backend learning plan. Each day's folder (`Day_NN_*/`) contains a teaching README for the concept of the day and the codebase that emerged from it. Reading the days in order is the entire pedagogical product; this folder is just where the Week 4 cumulative shape lives.
