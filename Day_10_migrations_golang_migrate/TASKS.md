# Day 10 — Practice Tasks

Each task adds one more migration concept. Type the SQL by hand; the `migrate create` shortcut is fine after Task 1.

> **Before you start:**
>
> ```powershell
> # 1. Wipe Day 8's volume so migrations own the schema
> cd ..\Day_08_postgres_sql_basics
> docker compose down -v
> docker compose up -d
>
> # 2. Set the DSN once for this session
> cd ..\Day_10_migrations_golang_migrate
> $env:DATABASE_URL = "postgres://app:app@localhost:5433/appdb?sslmode=disable"
>
> # 3. Install the Go library deps
> go mod init day10
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> ```

---

## Warm-up

- [ ] Confirm the DB is empty: in `psql`, `\dt` should show no tables (or only `schema_migrations` after the first `up`).
- [ ] Run the migrations forward and back:
  ```powershell
  go run .             # up — should report "done" and "version: 3, dirty: false"
  go run . version     # should print: version: 3  dirty: false
  go run . down 1      # rolls back 000003
  go run . version     # version: 2
  go run .             # up again — back to 3
  ```
- [ ] In `psql`, `\dt` → `schema_migrations`, `users`, `todos`. `\d todos` → see `user_id` column with the FK.
- [ ] `SELECT * FROM schema_migrations;` → confirm `version=3, dirty=false`.

---

## Task 1 — Add a `priority` column to `todos` (the boring CRUD pattern)

This is the most common migration shape: add a non-null column with a default.

- [ ] Generate a pair of files:
  ```powershell
  migrate create -ext sql -dir migrations -seq -digits 6 add_priority_to_todos
  ```
  (If you didn't install the CLI, create the two files by hand: `000004_add_priority_to_todos.up.sql` and `.down.sql`.)
- [ ] In `.up.sql`:
  ```sql
  ALTER TABLE todos
      ADD COLUMN priority TEXT NOT NULL DEFAULT 'low'
      CHECK (priority IN ('low', 'medium', 'high'));
  ```
- [ ] In `.down.sql`:
  ```sql
  ALTER TABLE todos DROP COLUMN IF EXISTS priority;
  ```
- [ ] `go run .` — should report "done, version: 4".
- [ ] `go run . down 1` — version 3 again, column gone.
- [ ] `go run .` — back to 4. **Idempotency** — running multiple times is safe.

**Why:** the `CHECK` constraint enforces a domain rule at the DB. You can never insert an "urgent" todo even if the app forgets to validate.

---

## Task 2 — Index for the most common query

- [ ] Generate `000005_index_todos_user_done`.
- [ ] `.up.sql`:
  ```sql
  CREATE INDEX idx_todos_user_done ON todos(user_id, done);
  ```
- [ ] `.down.sql`:
  ```sql
  DROP INDEX IF EXISTS idx_todos_user_done;
  ```
- [ ] Run up + verify in `psql`: `\d todos` shows the index.

**Why:** composite indexes match the "list my open todos" query — Day 30 uses exactly this.

---

## Task 3 — A `deleted_at` soft-delete column

- [ ] Generate `000006_soft_delete_todos`.
- [ ] `.up.sql`:
  ```sql
  ALTER TABLE todos ADD COLUMN deleted_at TIMESTAMPTZ;
  -- partial index — only the "alive" rows
  CREATE INDEX idx_todos_alive ON todos(id) WHERE deleted_at IS NULL;
  ```
- [ ] `.down.sql`:
  ```sql
  DROP INDEX IF EXISTS idx_todos_alive;
  ALTER TABLE todos DROP COLUMN IF EXISTS deleted_at;
  ```
- [ ] Apply, verify, then `down 1`, verify, then `up`.

**Why:** soft-delete is the schema decision you'll make on day-one of every real product. Now you have the migration pattern for it.

---

## Task 4 — Simulate a "dirty" DB and recover

- [ ] Make a deliberately-broken migration: `000007_break_things.up.sql`:
  ```sql
  CREATE TABLE tmp_test (id BIGINT);
  SELECT 1/0;       -- intentional error AFTER something already happened
  ```
- [ ] `.down.sql`:
  ```sql
  DROP TABLE IF EXISTS tmp_test;
  ```
- [ ] `go run .` — fails. `go run . version` reports `dirty: true`.
- [ ] **Recover:** in `psql`, `DROP TABLE tmp_test;` manually. Then:
  ```powershell
  go run . force 6
  go run . version       # version: 6  dirty: false
  ```
- [ ] Delete the broken migration files. **Never** edit a migration that's already been applied to *another* DB — but on your laptop, before sharing, deleting is fine.

**Why:** dirty state is the most common migration headache. Practising the recovery once means it's just an annoyance, not a disaster.

---

## Task 5 — Rename a column without breaking anything

A rename is **two migrations**, not one. The first adds the new column; the app reads/writes BOTH for a while; the second drops the old one.

For practice, do the simpler one-shot version today:

- [ ] Generate `000007_rename_done_to_completed`.
- [ ] `.up.sql`:
  ```sql
  ALTER TABLE todos RENAME COLUMN done TO completed;
  ```
- [ ] `.down.sql`:
  ```sql
  ALTER TABLE todos RENAME COLUMN completed TO done;
  ```
- [ ] Apply. `\d todos` — column is now `completed`.
- [ ] `down 1` — back to `done`.
- [ ] Stretch yourself: think about what would go wrong if your Go code referenced `done` while you were doing a rolling deploy. The "two-migration dance" (add column → dual-write → backfill → drop old) is how real teams avoid downtime.

---

## Task 6 — Run migrations on app startup

In real apps, the API process applies migrations as its first act. Let's preview that.

- [ ] Open [main.go](main.go) and read the `migrate.New` + `m.Up()` lines.
- [ ] Imagine pasting those 5 lines at the top of your Day 12 HTTP server's `main()`. That's it — your app is now self-healing.
- [ ] Optional: wrap `m.Up()` with a `slog.Info("migrations up", "version", v)` for nice startup logs.

**Why:** Day 12 will do exactly this. No CLI needed for deploys.

---

## Stretch — only if you're flying

- [ ] Read the `golang-migrate` README's "Best Practices": <https://github.com/golang-migrate/migrate/blob/master/MIGRATIONS.md>
- [ ] Wrap a migration in `BEGIN; ... COMMIT;` explicitly. Confirm a deliberate error mid-migration rolls back atomically (no half-applied schema).
- [ ] Generate a migration that adds a new table with **two foreign keys** in one shot — confirm the order matters (the referenced tables must already exist).
