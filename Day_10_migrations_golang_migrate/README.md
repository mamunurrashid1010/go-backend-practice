# Day 10 — Migrations with `golang-migrate`

> **Goal:** stop running `CREATE TABLE` by hand. Put every schema change in a version-controlled file that can be applied (`up`) or reverted (`down`), and let `golang-migrate` keep track of which ones have run.

---

## 1. Why migrations exist

Through Day 9 you typed `CREATE TABLE`, `ALTER TABLE` etc. directly into `psql`. That works for you, on your laptop. It doesn't work when:

- You have a teammate who also needs the same schema.
- You deploy to staging *and* production — each has its own DB.
- You have to **roll back** a bad change.
- You need to know "what state is my DB schema in right now?"

A migration is a **versioned, idempotent SQL file** that gets applied in order. The tool (`golang-migrate`) keeps a tiny `schema_migrations` table in your DB that records "which versions have been applied so far". Run `migrate up` and it figures out the rest.

```
migrations/
├── 000001_create_users.up.sql        ← applied in order
├── 000001_create_users.down.sql      ← undoes the one above
├── 000002_create_todos.up.sql
├── 000002_create_todos.down.sql
├── 000003_link_todos_to_users.up.sql
└── 000003_link_todos_to_users.down.sql
```

The pattern is **always pairs** — every `.up.sql` has a `.down.sql` that exactly reverses it. Without the down file, you can never roll back.

---

## 2. Install `golang-migrate`

You'll use it in two ways: a **CLI** for everyday hand-running, and a **Go library** to run migrations on app startup.

### CLI (choose one)

```powershell
# Option A: scoop (recommended on Windows)
scoop install migrate

# Option B: go install
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Option C: download a binary from
#   https://github.com/golang-migrate/migrate/releases
```

Verify:

```powershell
migrate -version
```

### Library (Day 10's `main.go` uses this)

```powershell
go mod init day10
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/jackc/pgx/v5/stdlib
```

---

## 3. Reset the DB to "fresh" — migrations should own the schema

Day 8's `init.sql` created a `todos` table for you. From today on, **migrations own the schema** — `init.sql` becomes obsolete.

Wipe the Docker volume so today's migrations have a clean slate:

```powershell
cd ..\Day_08_postgres_sql_basics
docker compose down -v       # the -v destroys the volume → fresh DB
# Optional: edit init.sql to be empty so it doesn't re-seed.
docker compose up -d
```

> **Why this matters in real teams:** the moment you start writing migrations, the DB has *two* people trying to define schema — the `init.sql` and `migrate up`. They'll drift apart in days. Pick one. Migrations win.

---

## 4. Naming convention

Two styles are popular. Both work; pick one and stick to it.

| Style | Example | When |
| --- | --- | --- |
| **Sequential** | `000001_create_users.up.sql` | Solo / small team. Easy to read in order. |
| **Timestamp** | `20260520150000_create_users.up.sql` | Big team. Avoids merge conflicts on the next number. |

We use sequential 6-digit padding today — easy to scan. Tools generate these for you:

```powershell
migrate create -ext sql -dir migrations -seq -digits 6 add_priority_to_todos
# creates:
#   migrations/000004_add_priority_to_todos.up.sql
#   migrations/000004_add_priority_to_todos.down.sql
```

---

## 5. Writing today's migrations

We're building toward the Day 11/12 schema: a `users` table, a `todos` table that links to it.

Three migrations:

| # | Name | What |
| --- | --- | --- |
| 1 | `create_users` | new table |
| 2 | `create_todos` | new table |
| 3 | `link_todos_to_users` | adds `user_id BIGINT NOT NULL REFERENCES users(id)` to `todos` |

Open each file in [migrations/](migrations/) and read both halves. The pairs are mirrors:

```sql
-- 000001_create_users.up.sql
CREATE TABLE users (
    id    BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name  TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE
);

-- 000001_create_users.down.sql
DROP TABLE IF EXISTS users;
```

### Three habits to internalise about writing migrations

1. **`IF EXISTS` / `IF NOT EXISTS` everywhere in `down`.** A down migration may run on a DB where the up never finished — be defensive.
2. **One logical change per migration.** "Create users + add a column + create an index" should be three migrations, not one. Reverting one piece becomes possible.
3. **Never edit an applied migration.** Once `000001_*.up.sql` has run anywhere — yours, a teammate's, prod — it's frozen. Add a new migration to fix it.

---

## 6. Run the migrations (CLI)

```powershell
# DSN matches Day 8's docker-compose (host port 5433)
$env:DB_URL = "postgres://app:app@localhost:5433/appdb?sslmode=disable"

# Apply all pending migrations
migrate -path migrations -database $env:DB_URL up

# Confirm
docker compose -f ..\Day_08_postgres_sql_basics\docker-compose.yml exec postgres psql -U app -d appdb -c "\dt"
# you should see: schema_migrations, users, todos

# Walk a single step back
migrate -path migrations -database $env:DB_URL down 1

# Walk a single step forward
migrate -path migrations -database $env:DB_URL up 1

# Show current version
migrate -path migrations -database $env:DB_URL version
```

### The `schema_migrations` table

`golang-migrate` creates this automatically. Inside `psql`:

```sql
SELECT * FROM schema_migrations;
--  version | dirty
-- ---------+-------
--        3 | f
```

- `version` — the latest migration applied (3 = up to `000003_*`).
- `dirty` — `true` if a migration partially applied and crashed. Cleared on success. We'll handle the dirty state below.

---

## 7. Run migrations from Go (library mode)

The CLI is great for developers. In production you usually want the **app itself** to migrate on startup — fewer moving parts, no human required.

[main.go](main.go) shows how:

```go
m, err := migrate.New("file://migrations", os.Getenv("DATABASE_URL"))
if err != nil { ... }
if err := m.Up(); err != nil && err != migrate.ErrNoChange {
    log.Fatalf("migrate up: %v", err)
}
```

Then run it like any Go program:

```powershell
$env:DATABASE_URL = "postgres://app:app@localhost:5433/appdb?sslmode=disable"
go run .                  # applies pending migrations and prints the new version
go run . down             # rolls back one step (we add a flag for this)
```

Day 12 will mount this exact code at the top of the HTTP server's startup.

---

## 8. The "dirty" state — what to do when it happens

If a migration fails halfway, `golang-migrate` marks the DB **dirty**. Future `up` / `down` runs refuse to do anything — they don't trust the state.

You'll see:

```
error: Dirty database version 2. Fix and force version.
```

The fix:

1. Open `psql`, manually undo whatever the half-applied migration left behind.
2. Tell `migrate` what version you actually believe the DB is at:

   ```powershell
   migrate -path migrations -database $env:DB_URL force 1
   ```

   That sets `version = 1`, `dirty = false`. Now you can `up` again.

> **Real-world prevention:** wrap each migration's SQL in a transaction. Postgres' DDL is transactional, so this is almost free:
>
> ```sql
> BEGIN;
> -- changes...
> COMMIT;
> ```
>
> Some statements (CREATE INDEX CONCURRENTLY, ALTER TYPE in older versions) can't go inside a transaction. Those need extra care.

---

## 9. Common patterns you'll write a lot

```sql
-- add a column
ALTER TABLE todos ADD COLUMN priority TEXT;
-- backfill before NOT NULL (you can't go straight to NOT NULL on existing rows)
UPDATE todos SET priority = 'low' WHERE priority IS NULL;
-- now it's safe
ALTER TABLE todos ALTER COLUMN priority SET NOT NULL;

-- rename a column
ALTER TABLE todos RENAME COLUMN done TO completed;

-- add an index (CONCURRENTLY in prod to avoid locking writes)
CREATE INDEX idx_todos_user_done ON todos(user_id, done);

-- drop a column (the "down" of an add)
ALTER TABLE todos DROP COLUMN priority;
```

---

## 10. What's next

**Day 11** introduces the **handler / service / repository** layering on the Day 7 To-Do API. The schema you just created becomes the foundation for **Day 12**, where the repository switches from `sync.Mutex` map to Postgres — no API changes.

After this day:
- Your DB schema lives in a directory of versioned `.sql` files.
- Anyone can clone your repo, run one command, and have a matching DB.
- Bad change? `migrate down 1` reverts it.

That's the leap from "scripting against a DB" to "owning a schema".
