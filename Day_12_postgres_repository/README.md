# Day 12 — Postgres-backed Repository

> **Goal:** swap Day 11's in-memory `Repository` for a Postgres-backed one. `handler.go`, `service.go`, `errors.go`, the routes, the JSON envelope — **none of them change.** That's the whole point.

---

## 1. The one line that matters

```go
// Day 11, main.go
repo := todo.NewInMemoryRepository()

// Day 12, main.go
repo := todo.NewPostgresRepository(db)
```

That single substitution is the lesson. Everything else in [main.go](main.go) is the new infrastructure around it: opening the DB pool, running migrations on startup, wiring `DATABASE_URL`.

Everything in `internal/todo/handler.go`, `service.go`, `errors.go`, `todo.go` is **unchanged from Day 11**. Re-open them and confirm: they don't import `database/sql`, they don't know about SQL, they don't reference Postgres.

This is what "the right abstraction" looks like in practice. Day 11 paid the cost; Day 12 collects.

---

## 2. The new file: `postgres_repository.go`

[internal/todo/postgres_repository.go](internal/todo/postgres_repository.go) implements the same `Repository` interface as `InMemoryRepository`, just with SQL.

Six methods, six small SQL statements. The complete file is ~150 lines and worth reading top-to-bottom.

### Three SQL tricks you'll use over and over

**a) `INSERT ... RETURNING *` — get the new row in one round-trip.**

```sql
INSERT INTO todos (title, done)
VALUES ($1, $2)
RETURNING id, title, done, created_at, updated_at
```

No second `SELECT` to read back `id` and the server-set timestamps. Postgres-specific; MySQL needs `LastInsertId()` (Day 9 Task 6 was the proof).

**b) `COALESCE($n, column)` — the PATCH-with-pointer-fields pattern.**

```sql
UPDATE todos
SET title      = COALESCE($2, title),
    done       = COALESCE($3, done),
    updated_at = now()
WHERE id = $1
RETURNING id, title, done, created_at, updated_at
```

Pass a `*string` (or `*bool`) that's nil when the client didn't send the field. The `database/sql` driver sends `NULL`, and `COALESCE` keeps the existing value. **The same SQL handles every combination of provided / not-provided fields.**

This is the SQL mirror of Day 3's pointer-field DTO.

**c) `RowsAffected() == 0` → `ErrNotFound`.**

```go
res, err := r.db.ExecContext(ctx, `DELETE FROM todos WHERE id = $1`, id)
if err != nil { return err }
n, _ := res.RowsAffected()
if n == 0 {
    return ErrNotFound
}
```

SQL doesn't consider "DELETE matched 0 rows" an error — but your API should. Translating it to the same `ErrNotFound` the in-memory repo returns means the handler reacts the same way: 404.

---

## 3. `sql.ErrNoRows` → `ErrNotFound` translation

When `QueryRow` finds nothing, the driver returns `sql.ErrNoRows`. This is a Postgres-specific implementation detail. The handler shouldn't know about it.

The repository **catches and translates** before returning:

```go
err := r.db.QueryRowContext(ctx, q, id).Scan(...)
if errors.Is(err, sql.ErrNoRows) {
    return Todo{}, ErrNotFound
}
if err != nil {
    return Todo{}, fmt.Errorf("get id=%d: %w", id, err)  // wrap for context
}
```

After this, the handler maps `ErrNotFound` → 404 (in `writeServiceErr`) and the API behaviour matches Day 11 exactly.

> **The pattern:** errors live in three layers:
> - **Driver-specific** (`sql.ErrNoRows`, `pgconn.PgError`) — never leave the repository.
> - **Domain** (`ErrNotFound`, `ErrConflict`) — what the service+handler speak.
> - **HTTP** (`404`, `409`) — what the wire speaks.

---

## 4. Migrations on startup

Day 10 introduced `golang-migrate`. Day 12 wires it into the server's startup so deployment is "one binary, no scripts":

```go
func runMigrations(dsn string) error {
    m, err := migrate.New("file://migrations", dsn)
    if err != nil { return err }
    defer m.Close()
    if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
        return err
    }
    return nil
}
```

Called at the top of `main()` before the server starts listening. If migrations fail, the server doesn't come up — exactly what you want.

The migrations directory in this folder is **a fresh set for Day 12**, not the Day 10 ones — we don't have users / auth yet, so todos doesn't need `user_id`. Day 21's Notes mini-project will reintroduce users.

```
migrations/
├── 000001_create_todos.up.sql
└── 000001_create_todos.down.sql
```

---

## 5. `context.Context` end-to-end

Today is where context cancellation becomes real. A `GET /todos` that takes 30 seconds because a client uploads slowly used to leak a goroutine; now it leaks a goroutine **and** holds a Postgres connection. With context, both die when the client disconnects.

Trace the path:

```
1.  client sends GET /todos
2.  chi handler runs:        ctx := r.Context()
3.  handler calls service:    h.Svc.List(ctx, filter)
4.  service calls repo:        s.repo.List(ctx, filter)
5.  repo calls driver:         r.db.QueryContext(ctx, q)
6.  driver sends the query to Postgres, attaches the ctx
7.  client disconnects → r.Context() is cancelled
8.  the cancellation propagates down: driver sends Postgres CANCEL
9.  query aborts, connection returns to the pool
```

Day 6 set up the plumbing (the middleware passes the request's context through). Day 9 introduced the `*Context` query variants. Today is where they pay off.

---

## 6. Connection pool — back to it

`*sql.DB` is a pool (Day 9 was very explicit about this). Today's `main()` opens **one** pool at startup and hands it to the repository:

```go
db, err := sql.Open("pgx", dsn)
if err != nil { ... }
defer db.Close()

db.SetMaxOpenConns(25)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(5 * time.Minute)
db.SetConnMaxIdleTime(2 * time.Minute)
```

The repository's `*sql.DB` field is the pool, not a connection. Every method borrows-and-returns automatically.

---

## 7. Run it

```powershell
# Prereq: Day 8's Postgres on host port 5433
cd ..\Day_08_postgres_sql_basics
docker compose ps        # "healthy"
# (Optional) wipe the volume if you want a fresh DB
# docker compose down -v && docker compose up -d

cd ..\Day_12_postgres_repository
go mod init day12
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv

# .env is already populated (DATABASE_URL → host port 5433)
go run .
```

Expected logs:

```
loaded .env
migrations up: 1
connected to postgres
listening on http://localhost:8080
GET    /todos       200  4ms rid=...
```

Hit the same Day 11 curls:

```powershell
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"title\":\"persist me\"}" http://localhost:8080/todos
# 201, Location: /todos/1

curl.exe -i http://localhost:8080/todos

# Stop the server (Ctrl+C), start it again
go run .
curl.exe http://localhost:8080/todos
# ← the row is still there. That's the whole product story of today.
```

In another terminal:

```powershell
docker compose -f ..\Day_08_postgres_sql_basics\docker-compose.yml exec postgres `
  psql -U app -d appdb -c "SELECT * FROM todos;"
```

See your Go-created rows live in real Postgres.

---

## 8. What you'll feel different

After Day 11, the API still lost state when you restarted the server. After today, **it doesn't**. The data is in Postgres. You can:

- Kill the server, restart, list — your todos are still there.
- Have two clients hit `POST /todos` simultaneously — Postgres assigns sequential IDs, no race.
- Open `psql`, edit a row by hand, refresh the API — your changes are visible.

That's the line between "demo app" and "could ship this".

---

## 9. What's next

**Day 13** — config via env (proper typing, validation), so the DSN, port, pool size, etc. all flow from one `config` package instead of `os.Getenv` calls scattered around. Then Days 14 ships the Postgres-backed To-Do API as the Week 2 mini-project.

After today's swap:
- Your handler/service/repository code is "real" — the same skeleton you'd ship.
- Your DB schema lives in version-controlled migration files.
- Your DB connection is pool-managed, context-aware, and pings on startup.
- Your migrations run automatically.

That's the entire skeleton of a production Go service. Everything from here is **features, polish, scale.**
