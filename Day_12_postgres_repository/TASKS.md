# Day 12 — Practice Tasks

The big lesson is done before you start: **the same API, now backed by Postgres**. These tasks tighten the corners.

> **Before you start:**
>
> ```powershell
> # Day 8's Postgres must be running on host port 5433
> cd ..\Day_08_postgres_sql_basics
> docker compose ps        # "healthy"
>
> cd ..\Day_12_postgres_repository
> go mod init day12
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go run .
> ```

---

## Warm-up — prove persistence works

- [ ] `curl.exe http://localhost:8080/todos` — empty array on a fresh DB.
- [ ] `curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"persisted\"}" http://localhost:8080/todos` — returns 201 with `id: 1` (or whatever IDENTITY assigned).
- [ ] **Stop the server (Ctrl+C). Start it again (`go run .`). List again.** The row survives — that's the whole story.
- [ ] In `psql`: `SELECT * FROM todos;` — your Go row is right there.
- [ ] Hit `GET /todos/9999` — 404 NOT_FOUND. Hit `DELETE /todos/9999` — also 404. Both routes translate `sql.ErrNoRows` or `RowsAffected = 0` to the same domain error.

---

## Task 1 — Confirm "only main.go changed"

- [ ] `diff` Day 11's `internal/todo/handler.go` against Day 12's. Expect: identical.
- [ ] Same for `service.go`, `errors.go`, `todo.go`.
- [ ] The new file is [internal/todo/postgres_repository.go](internal/todo/postgres_repository.go). Read it top to bottom — it's about 150 lines, all SQL.

This is the lesson. Write 2 lines in your "What I learned" section about how the layering paid off.

---

## Task 2 — Inspect the COALESCE PATCH trick

- [ ] In `psql`, run:
  ```sql
  INSERT INTO todos (title) VALUES ('orig') RETURNING id;     -- note the id
  ```
- [ ] From Go (or curl):
  ```powershell
  # PATCH only done — title is NULL on the wire, COALESCE keeps it
  curl.exe -i -X PATCH -H "Content-Type: application/json" `
    -d "{\"done\":true}" http://localhost:8080/todos/<id>
  ```
- [ ] `SELECT title, done FROM todos WHERE id = <id>;` — title unchanged, done is true.
- [ ] Repeat with `{"title":"renamed"}` — title changes, done unchanged.
- [ ] Re-read the SQL in `Patch` — see how **one statement** handles every combination of provided fields.

**Why:** the SQL mirror of Day 3's pointer-field DTO. Hand-rolling dynamic SQL with `if`/`else` to skip columns is the alternative; COALESCE wins.

---

## Task 3 — Watch a query get cancelled

Day 9 Task 5 showed `context.WithTimeout` at the driver layer. Today: from the HTTP side.

- [ ] Add a temporary handler in [main.go](main.go) (right next to `/healthz`):
  ```go
  r.Get("/slow", func(w http.ResponseWriter, r *http.Request) {
      var n int
      err := db.QueryRowContext(r.Context(),
          `SELECT count(*) FROM pg_sleep(5)`).Scan(&n)
      // ...handle err...
      respond.JSON(w, 200, map[string]int{"n": n})
  })
  ```
- [ ] Run `curl.exe http://localhost:8080/slow` and **Ctrl+C** the curl 1 second in.
- [ ] In your server log, look for the `pgx` error: query was cancelled.
- [ ] Open `psql` and run:
  ```sql
  SELECT pid, query, state FROM pg_stat_activity WHERE datname = 'appdb';
  ```
  No leftover `pg_sleep` query. The DB got the cancel signal.

**Why:** prove to yourself that `r.Context()` flowing handler → service → repo → driver actually does something. Day 6 set this up; today it's wired through to Postgres.

---

## Task 4 — Make the repo testable: ping the in-memory version

- [ ] In `main.go`, temporarily change THE LINE:
  ```go
  repo := todo.NewInMemoryRepository()  // Day 11's version
  ```
- [ ] `go run .` — everything still works *except* `psql` is no longer involved.
- [ ] Restart, lose your data. Switch back to Postgres.

This is the proof: the abstraction works both ways. Day 22's unit tests use exactly this — a non-Postgres `Repository` implementation so tests don't need a database.

---

## Task 5 — Connection-pool sanity check

- [ ] While the server is running, open another terminal:
  ```powershell
  docker compose -f ..\Day_08_postgres_sql_basics\docker-compose.yml exec postgres `
    psql -U app -d appdb -c "SELECT pid, state FROM pg_stat_activity WHERE datname='appdb';"
  ```
- [ ] Note the connection count. Try a few:
  - `SetMaxOpenConns(25)` (default in our `main.go`) → up to 25 connections under load.
  - Drop it to `SetMaxOpenConns(2)` → only 2 even under load. Hit `/todos` concurrently with many curls and watch the others wait.

**Why:** Day 30 makes pool tuning a real exercise. Today is the muscle memory of "the pool is real, you can see it".

---

## Task 6 — Soft delete (Day 10 migration becomes real)

- [ ] Add a new migration: `000002_soft_delete.up.sql`
  ```sql
  ALTER TABLE todos ADD COLUMN deleted_at TIMESTAMPTZ;
  CREATE INDEX idx_todos_alive ON todos(id) WHERE deleted_at IS NULL;
  ```
- [ ] `.down.sql`:
  ```sql
  DROP INDEX IF EXISTS idx_todos_alive;
  ALTER TABLE todos DROP COLUMN IF EXISTS deleted_at;
  ```
- [ ] Run the server — migrations auto-apply, you see `migrations up: 2`.
- [ ] In `postgres_repository.go`, change `Delete` to set `deleted_at = now()` instead of `DELETE FROM`. Update `Get` / `List` to add `WHERE deleted_at IS NULL`.
- [ ] Verify: a deleted todo no longer shows in `GET /todos`, but is still in the table (`SELECT * FROM todos;` in psql — it's there with `deleted_at` set).

**Why:** soft delete is the most common "after MVP" schema decision. Doing it as a migration + repo change shows the full update flow.

---

## Task 7 — Restore endpoint

- [ ] Add `POST /todos/{id}/restore` that clears `deleted_at` on a soft-deleted row.
- [ ] Service: a new method `Restore(ctx, id)`. Repository: a new method `Restore`. Handler: a new sub-route.
- [ ] Confirm: delete → list (gone) → restore → list (back).

**Why:** practising the "add a feature" loop across all three layers in one task.

---

## Stretch — only if you're flying

- [ ] Replace `database/sql` with **native pgx** (`github.com/jackc/pgx/v5/pgxpool`). The repository methods change from `QueryRowContext` / `Scan` to `pool.QueryRow(...).Scan(...)`. Same shape, slightly nicer types. Decide which you'd ship.
- [ ] In `psql`, manually `INSERT` a row with an explicit `id`. Notice Postgres rejects it (because of `GENERATED ALWAYS AS IDENTITY`). Try `OVERRIDING SYSTEM VALUE` and see it succeed. That's why we used `ALWAYS` instead of `BY DEFAULT`.
- [ ] Add a unique constraint on `title` (`ALTER TABLE todos ADD CONSTRAINT u_todos_title UNIQUE (title);`). Try to create a duplicate from the API. Notice the 500 — the repo returns a wrapped pgx error. **Translating** unique-violation to `ErrConflict` is the Day 16 lesson; sketch the code.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
