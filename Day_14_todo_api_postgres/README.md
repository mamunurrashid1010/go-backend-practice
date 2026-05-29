# Day 14 — To-Do REST API (Week 2 mini-project)

A production-shaped Go REST API for managing to-dos. **Same surface as Day 7, now real:** Postgres persistence, versioned migrations, layered code, typed config, graceful shutdown. The repo someone could clone and run.

This is not a teaching day — it's the **closer** for Week 2. Every concept from Days 8–13 lands here.

---

## Stack

| Concern | Choice |
| --- | --- |
| Language | Go 1.22+ (uses `r.PathValue` features chi-style) |
| Router | [`go-chi/chi/v5`](https://github.com/go-chi/chi) |
| DB | Postgres 16, talked to via `database/sql` + [`pgx/v5/stdlib`](https://github.com/jackc/pgx) |
| Migrations | [`golang-migrate/migrate/v4`](https://github.com/golang-migrate/migrate) (library mode, auto-applied on startup) |
| Config | [`caarlos0/env/v11`](https://github.com/caarlos0/env) + [`joho/godotenv`](https://github.com/joho/godotenv) for dev `.env` loading |
| Layering | handler / service / repository — handler/service depend on the `Repository` **interface**, not on Postgres |

---

## Quick start

```powershell
# 1. Postgres up (or use Day 8's compose — same port 5433)
docker compose up -d

# 2. Build / run
go mod init day14
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go run .
```

You should see something like:

```
loaded .env
config: env=development addr=:8080 db_pool=25/10
migrations up: 1
connected to postgres
listening on http://localhost:8080
```

`Ctrl+C` triggers graceful shutdown — the server stops accepting new connections, drains in-flight requests, then exits cleanly.

---

## API reference

Every response is JSON. Errors share one envelope:

```json
{ "error": { "code": "NOT_FOUND", "message": "todo not found" } }
```

Every request gets an `X-Request-ID` header on the way back (echoed if the client sent one).

| Method | Path | Body | Status codes |
| --- | --- | --- | --- |
| GET    | `/healthz`        | —                                  | 200 |
| GET    | `/todos`          | —                                  | 200 |
| GET    | `/todos?done=true&search=foo&limit=10` | —                     | 200, 400 |
| POST   | `/todos`          | `{"title":"x","done":false}`       | 201, 400 |
| GET    | `/todos/{id}`     | —                                  | 200, 400, 404 |
| PUT    | `/todos/{id}`     | `{"title":"y","done":true}`        | 200, 400, 404 |
| PATCH  | `/todos/{id}`     | `{"title":"y"}` or `{"done":true}` | 200, 400, 404 |
| DELETE | `/todos/{id}`     | —                                  | 204, 400, 404 |

### Examples

```powershell
# Create
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"title\":\"ship Week 2\"}" http://localhost:8080/todos

# List + filter
curl.exe http://localhost:8080/todos
curl.exe "http://localhost:8080/todos?done=false&search=ship"

# Partial update — only done flips, title untouched
curl.exe -i -X PATCH -H "Content-Type: application/json" `
  -d "{\"done\":true}" http://localhost:8080/todos/1

# Delete
curl.exe -i -X DELETE http://localhost:8080/todos/1
```

---

## Configuration

All knobs come from environment variables (see [`.env.example`](.env.example)). The full list:

| Var | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `development \| staging \| production` |
| `HTTP_ADDR` | `:8080` | server bind address |
| `HTTP_READ_TIMEOUT` | `10s` | |
| `HTTP_WRITE_TIMEOUT` | `10s` | |
| `HTTP_IDLE_TIMEOUT` | `60s` | keepalive timeout |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | drain window on Ctrl+C |
| `DATABASE_URL` | **required** | Postgres DSN |
| `DB_MAX_OPEN_CONNS` | `25` | |
| `DB_MAX_IDLE_CONNS` | `10` | |
| `DB_CONN_MAX_LIFETIME` | `5m` | |
| `DB_CONN_MAX_IDLE_TIME` | `2m` | |
| `DB_PING_TIMEOUT` | `5s` | fail-fast startup |

Bad config fails at startup with a clear error — the server doesn't come up half-broken.

---

## Project layout

```
Day_14_todo_api_postgres/
├── docker-compose.yml                  # Postgres (alpine, host port 5433)
├── .env.example                        # template — committed
├── .env                                # local values — gitignored
├── main.go                             # config → DB → migrate → repo → svc → handler → server (with shutdown)
├── migrations/
│   ├── 000001_create_todos.up.sql
│   └── 000001_create_todos.down.sql
└── internal/
    ├── config/config.go                # typed config struct (caarlos0/env)
    ├── middleware/middleware.go        # Recover, RequestID, Logger
    ├── respond/respond.go              # consistent JSON envelope
    └── todo/
        ├── todo.go                     # domain model + DTOs
        ├── errors.go                   # sentinel domain errors
        ├── repository.go               # Repository interface + InMemoryRepository
        ├── postgres_repository.go      # Postgres-backed Repository (INSERT...RETURNING, COALESCE PATCH)
        ├── service.go                  # business rules
        └── handler.go                  # HTTP transport
```

Read main.go top to bottom — it's a sentence: *"load config, open DB, migrate, wire layers, serve, shut down cleanly."*

---

## What this project demonstrates (Week 2 in summary)

- **Day 8** — Postgres + psql + SQL basics → the table schema below lives in migrations now, not `psql` history.
- **Day 9** — `database/sql` + pgx → the `*sql.DB` pool, context everywhere, `RETURNING`, `RowsAffected`.
- **Day 10** — `golang-migrate` → schema versioned in `migrations/`, applied automatically on startup.
- **Day 11** — handler / service / repository → swapping storage is one line in `main.go`.
- **Day 12** — Postgres-backed repository → `INSERT ... RETURNING *`, `COALESCE($n, col)` PATCH, `sql.ErrNoRows → ErrNotFound`.
- **Day 13** — typed config → `internal/config`, no scattered `os.Getenv`, validation at startup.

---

## What's NOT here yet

Honest about scope. Coming in Weeks 3+:

- **Validation library** (Day 15 / `go-playground/validator`) — today's validation is hand-rolled in the service.
- **Typed domain errors with extra fields** (Day 16) — sentinel errors only.
- **Auth / JWT** (Days 17–20) — public API, no users.
- **Tests** (Days 22–24) — the layering makes this trivial; Week 4 adds them.
- **Structured logging** (Day 25 / `slog`) — `log.Printf` for now.
- **Pagination beyond `limit`** (Day 26) — no cursor pagination yet.
- **Rate limiting / CORS** (Day 27) — none.

---

Either way, the README + `.env.example` mean someone can clone, copy `.env.example` to `.env`, set their `DATABASE_URL`, and `go run .` — a working API in three commands.
