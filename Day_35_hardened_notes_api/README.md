# Notes API — Hardened

[![Go](https://img.shields.io/badge/go-1.22+-00ADD8?logo=go)](https://go.dev)
[![Postgres](https://img.shields.io/badge/postgres-16-336791?logo=postgresql&logoColor=white)](https://www.postgresql.org)
[![Redis](https://img.shields.io/badge/redis-7-DC382D?logo=redis&logoColor=white)](https://redis.io)
[![OpenAPI](https://img.shields.io/badge/OpenAPI-3.0-6BA539?logo=openapiinitiative&logoColor=white)](internal/openapi/openapi.yaml)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

A production-shaped Go REST API for personal notes. Everything the Day 28 version had, now with **Phase 3 hardening**: Postgres transactions with an atomic audit log, sqlc-generated repositories, Redis cache-aside on `GET /notes/{id}` with singleflight, distributed rate limiting via Redis (sliding window log), retry-safe writes via `Idempotency-Key`, and a browsable OpenAPI 3.0 spec at `/docs/`.

> This is the Week 5 ship of a 15-week Go backend learning plan. Days 29–34 are folders next to this one; each day's `README.md` teaches one concept on top of the codebase that came before it. Day 35 keeps the surface area of Day 34 — it doesn't add a feature, it makes the cumulative result *clonable*.

---

## What it does

- **`POST /auth/register` / `login` / `refresh` / `logout` / `GET /me`** — JWT + rotating opaque refresh tokens (reuse detection kills the family).
- **`GET /notes`, `POST /notes`, `GET/PUT/PATCH/DELETE /notes/{id}`** — full CRUD, scoped to the authenticated user, IDOR-safe.
- **Cursor pagination** on the list (`?after=…&limit=…&sort=asc|desc&search=…`).
- **Atomic writes** — every note create/update/patch/delete runs inside one Postgres transaction alongside an `audit_logs` insert. Either both, or neither.
- **`GET /audit`** — the calling user's action history. `?include=notes` embeds each entry's target note via a JOIN, batched IN, or naive-N+1 (`?strategy=`) — the Day 30 teaching demo, permanently wired.
- **Redis cache-aside** on `GET /notes/{id}` — Redis first, Postgres on miss, cache-fill on the way out, singleflight coalesces concurrent misses, TTL invalidated after commit on writes.
- **Distributed rate limit** — Redis sliding-window log (`X-RateLimit-Limit`, `-Remaining`, `-Reset`, `Retry-After`). Same limit holds across replicas.
- **`Idempotency-Key`** header on POST/PUT/PATCH — same key + same body → replay; same key + different body → 422.
- **OpenAPI 3.0 spec** at `/docs/openapi.yaml`; **Swagger UI** at `/docs/`.
- Structured JSON logs in prod / Text in dev (`log/slog`), graceful shutdown, allowlist CORS, config validated at startup.

Everything below is implemented — the [API reference](#api-reference) walks the whole surface, or point a browser at [http://localhost:8080/docs/](http://localhost:8080/docs/) after `go run .`.

---

## Quick start

You need **Go 1.22+** and **Docker**.

```powershell
git clone <this repo>
cd Day_35_hardened_notes_api
cp .env.example .env                 # copy the dev defaults
docker compose up -d                 # postgres on 5433, redis on 6379
go mod init day35
go mod tidy
go run .
```

You should see:

```
loaded .env
config env=development addr=:8080 rate_limit_backend=redis
connected to postgres
migrations up version=5
connected to redis
listening addr=http://localhost:8080 docs=http://localhost:8080/docs/
```

Hit it:

```powershell
curl.exe http://localhost:8080/healthz
# {"status":"ok"}
```

Then open `http://localhost:8080/docs/` — you'll get the full API in Swagger UI. Click Authorize with a bearer token from `/auth/login` and every "Try it out" button actually hits your local server.

---

## Full curl walkthrough

```powershell
# 1. Register
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"email\":\"alice@example.com\",\"password\":\"hunter2pass\"}" `
  http://localhost:8080/auth/register
# 201 Created, Location: /users/1

# 2. Login — save tokens
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"email\":\"alice@example.com\",\"password\":\"hunter2pass\"}" `
  http://localhost:8080/auth/login | ConvertFrom-Json
$tok = $r.access_token; $ref = $r.refresh_token

# 3. Idempotent note create — same key + body replays; different body -> 422
$key = [guid]::NewGuid().ToString()
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -H "Idempotency-Key: $key" -d "{\"title\":\"buy milk\"}" http://localhost:8080/notes
# 201, Location: /notes/1

# 4. Retry — replays, adds Idempotent-Replayed: true header
curl.exe -i -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -H "Idempotency-Key: $key" -d "{\"title\":\"buy milk\"}" http://localhost:8080/notes

# 5. GET — hits Postgres, populates Redis
curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/notes/1 | Out-Null
# 2nd GET — hits Redis only (watch redis-cli MONITOR)
curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/notes/1 | Out-Null

# 6. Audit — the atomic write left a trail
curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes" | ConvertFrom-Json

# 7. Watch rate limit trip
1..70 | ForEach-Object {
    $r = curl.exe -s -o $null -w "%{http_code} R=%{header_x-ratelimit-remaining}`n" http://localhost:8080/healthz
    "$_ $r"
}
# 60 x 200 with X-RateLimit-Remaining counting down, then 429 with Retry-After.

# 8. Refresh rotation
$r = curl.exe -s -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$ref\"}" http://localhost:8080/auth/refresh | ConvertFrom-Json
$tok = $r.access_token; $ref = $r.refresh_token

# 9. Logout
curl.exe -i -X POST -H "Content-Type: application/json" `
  -d "{\"refresh_token\":\"$ref\"}" http://localhost:8080/auth/logout
# 204
```

---

## API reference

Full machine-readable spec at [internal/openapi/openapi.yaml](internal/openapi/openapi.yaml). Serve it live at **`GET /docs/openapi.yaml`**; browse it interactively at **`GET /docs/`**.

Every response is JSON. Errors share one envelope:

```json
{ "error": { "code": "NOT_FOUND", "message": "note not found" } }
```

Response headers on the authed surface:

| Header | Where | Meaning |
| --- | --- | --- |
| `X-Request-ID` | every response | echoes the request ID or generates one |
| `X-RateLimit-Limit` / `-Remaining` / `-Reset` | every response through the limiter | current window budget |
| `Retry-After` | 429 only | seconds until unblocked |
| `Idempotent-Replayed: true` | replayed responses | this is the cached original |
| `Location` | 201 responses | URI of the new resource |

### Auth

| Method | Path | Notes |
| --- | --- | --- |
| POST | `/auth/register` | tight-limit route (5/window) |
| POST | `/auth/login` | tight-limit route |
| POST | `/auth/refresh` | rotates refresh token, revokes the presented one |
| POST | `/auth/logout` | 204 |
| GET | `/auth/me` | requires Bearer |

### Notes (require Bearer)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/notes` | cursor paginated: `search`, `limit`, `after`, `sort` |
| POST | `/notes` | `Idempotency-Key` respected; atomic with audit insert |
| GET | `/notes/{id}` | cache-aside (Redis + singleflight) |
| PUT | `/notes/{id}` | atomic + invalidates cache |
| PATCH | `/notes/{id}` | at least one field; atomic + invalidates cache |
| DELETE | `/notes/{id}` | atomic + invalidates cache; 204 |

### Audit (require Bearer)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/audit` | plain by default; `?include=notes&strategy=join\|in_batch\|naive` embeds the target |

---

## Project layout

```
Day_35_hardened_notes_api/
├── docker-compose.yml            # postgres 16 (5433) + redis 7 (6379)
├── .env.example                  # committed template
├── .env                          # gitignored local values
├── .gitignore
├── LICENSE                       # MIT
├── Makefile                      # up / down / migrate / run / test / sqlc / swagger
├── tools.go                      # pins swag + sqlc + migrate as dev deps
├── sqlc.yaml                     # sqlc config
├── main.go                       # config -> logger -> db -> redis -> repos -> services -> handlers -> server
├── queries/                      # sqlc annotated SQL — the source of truth
│   ├── users.sql
│   └── notes.sql
├── migrations/                   # golang-migrate, auto-applied on startup
│   ├── 000001_create_users.{up,down}.sql
│   ├── 000002_create_refresh_tokens.{up,down}.sql
│   ├── 000003_create_notes.{up,down}.sql
│   ├── 000004_index_notes_user_id_desc.{up,down}.sql
│   └── 000005_create_audit_logs.{up,down}.sql
└── internal/
    ├── config/                   # env-driven Config, validated at startup
    ├── logging/                  # slog wrapper + ctx logger
    ├── respond/                  # error envelope + JSON writers
    ├── validate/                 # go-playground/validator wrapper
    ├── httpjson/                 # shared DecodeJSON helper
    ├── dbtx/                     # Transactor + tx-on-context (Day 29)
    ├── db/                       # sqlc-generated (committed) — Day 31
    ├── cache/                    # redis JSON wrapper + jitter (Day 32)
    ├── ratelimit/                # Limiter interface + Memory + Redis sliding window (Day 33)
    ├── idempotency/              # Redis store + middleware (Day 33)
    ├── openapi/                  # openapi.yaml + Swagger UI handler (Day 34)
    ├── middleware/               # RequestID, Logger, Recover, CORS, RateLimit
    ├── auth/                     # user + JWT + refresh tokens, RequireUserID helper
    └── notes/, audit/            # domain packages
```

Reading order for a stranger: [main.go](main.go) → [internal/auth/service.go](internal/auth/service.go) → [internal/notes/service.go](internal/notes/service.go) → [internal/ratelimit/redis.go](internal/ratelimit/redis.go) → [internal/idempotency/middleware.go](internal/idempotency/middleware.go). The rest follows.

---

## Configuration

Every knob is an env var (see [.env.example](.env.example)). Complete list:

| Var | Default | Notes |
| --- | --- | --- |
| `APP_ENV` | `development` | `development \| staging \| production` |
| `LOG_LEVEL` | `info` | `debug \| info \| warn \| error` |
| `HTTP_ADDR` | `:8080` | |
| `HTTP_*_TIMEOUT` | 10s / 10s / 60s / 10s | read / write / idle / shutdown |
| `DATABASE_URL` | **required** | Postgres DSN |
| `DB_MAX_OPEN_CONNS` | `25` | |
| `DB_MAX_IDLE_CONNS` | `25` | matched to open conns for read-heavy workloads |
| `DB_CONN_MAX_LIFETIME` | `5m` | connection recycle |
| `DB_CONN_MAX_IDLE_TIME` | `2m` | idle eviction |
| `DB_PING_TIMEOUT` | `5s` | fail-fast startup |
| `REDIS_URL` | `redis://localhost:6379/0` | |
| `REDIS_PING_TIMEOUT` | `3s` | |
| `REDIS_NOTES_TTL` | `5m` | notes cache TTL |
| `REDIS_JITTER` | `0.1` | 0..1; additive ttl jitter |
| `JWT_SECRET` | **required, ≥32 chars** | HS256 |
| `JWT_ACCESS_TTL` | `15m` | |
| `JWT_ISSUER` | `day35-auth` | |
| `REFRESH_TTL` | `720h` (30 days) | must be > `JWT_ACCESS_TTL` |
| `RATE_LIMIT_BACKEND` | `redis` | `memory \| redis` |
| `RATE_LIMIT_GLOBAL_RPS` / `_BURST` | `10` / `30` | memory backend only |
| `RATE_LIMIT_AUTH_RPS` / `_BURST` | `1` / `5` | memory backend only |
| `RATE_LIMIT_TTL` | `5m` | memory-backend visitor eviction |
| `RATE_LIMIT_GLOBAL_MAX_PER_MINUTE` | `60` | redis backend |
| `RATE_LIMIT_AUTH_MAX_PER_MINUTE` | `10` | redis backend, only on `/auth/register`+`/login` |
| `RATE_LIMIT_WINDOW` | `1m` | redis backend window |
| `IDEMPOTENCY_TTL` | `24h` | cached response lifetime |
| `IDEMPOTENCY_LEASE_TTL` | `60s` | `pending` lease; must be ≤ TTL |
| `CORS_ALLOWED_ORIGINS` | `http://localhost:3000` | no `*` with credentials |
| `CORS_ALLOWED_METHODS` | `GET,POST,PUT,PATCH,DELETE,OPTIONS` | |
| `CORS_ALLOWED_HEADERS` | includes `Idempotency-Key` | |
| `CORS_EXPOSED_HEADERS` | includes rate-limit + `Idempotent-Replayed` | |
| `CORS_MAX_AGE` | `5m` | preflight cache |
| `CORS_ALLOW_CREDENTIALS` | `true` | |

Bad config fails at startup with a clear error.

---

## Architecture in one paragraph

The handler layer parses HTTP and never touches Postgres. The service layer enforces business rules and depends on a `Repository` interface. Cross-statement writes (`create note` + `write audit log`) run through the `dbtx.Transactor` — same context, same tx, atomic. Repository code resolves its runner from context (`RunnerFor`), so a repo method works identically inside or outside a transaction; the sqlc-generated wrapper picks up whichever runner ctx carries. The `notes` service adds a cache layer between itself and the repository: `GET` is Redis-first with a `singleflight.Group` to coalesce herd misses, and every write invalidates after commit. The `ratelimit.Limiter` interface is satisfied by two backends (in-process token bucket + Redis sliding-window log via one atomic Lua script) — swap via config. The idempotency middleware buffers request bodies, hashes them, uses `SETNX` on Redis to claim a slot, buffers responses, and writes them back on success so a retry gets a byte-for-byte replay. Every request carries an `X-Request-ID` on its context and its structured logger; every 500 logs with that context. There is no global mutable state — config, logger, `*sql.DB`, `*redis.Client`, `*dbtx.Transactor` are all constructed once in `main.go` and passed in.

---

## Testing

```powershell
go test ./...                       # unit + handler tests
go test -tags integration ./...     # + testcontainers Postgres + Redis integration
```

The layering pays off: the service layer takes interfaces, so tests use in-memory fakes; integration tests (Days 24, 33) exercise real Postgres and real Redis via [testcontainers-go](https://golang.testcontainers.org).

---

## `make` / scripts

```
make up          # docker compose up -d
make down        # docker compose down
make logs        # tail postgres + redis logs
make psql        # open psql in the running postgres container
make redis-cli   # open redis-cli in the running redis container
make redis-monitor
make migrate     # run migrations (via the pinned tool)
make sqlc        # regenerate internal/db/ from queries/
make swagger     # sanity-check the openapi.yaml (needs npx)
make run         # go run .
make test        # go test ./...
make test-int    # go test -tags integration ./...
make tidy fmt vet lint
```

`make` needs `make.exe` or a Git Bash shell on Windows. The same commands are spelled out in PowerShell in [TASKS.md](TASKS.md) if you don't have make.

---

## What's intentionally NOT here (yet)

Honest scope. These are coming in later phases:

- **Concurrency / profiling** (Week 6) — no `errgroup`-based worker pools, no `pprof` integrations, no load tests wired.
- **CI** (Day 42) — the badges above (except OpenAPI) are placeholders.
- **File uploads / WebSockets / SSE / webhooks** (Week 9).
- **Kafka / RabbitMQ / Temporal** (Weeks 10–12).
- **Prometheus / OpenTelemetry / Sentry** (Week 13).
- **Cloud deploy / Helm** (Week 14).

The codebase is layered so these slot in without churn.

---

## License

[MIT](LICENSE).

---

## Acknowledgements

Week 5 ship of a 15-week Go backend learning plan. Each day's folder (`Day_NN_*/`) contains the teaching README + the codebase snapshot that emerged. Reading the days in order is the actual pedagogical product; this folder is where Phase 3 Part A's cumulative shape lives.
