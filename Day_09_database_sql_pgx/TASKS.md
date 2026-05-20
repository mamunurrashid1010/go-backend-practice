# Day 9 — Practice Tasks

Each task layers one more `database/sql` idea onto today's program.

> **Before you start:**
>
> ```powershell
> cd ..\Day_08_postgres_sql_basics
> docker compose ps           # must show "healthy"
> cd ..\Day_09_database_sql_pgx
> go mod init day09
> go get github.com/jackc/pgx/v5/stdlib
> go run .
> ```

---

## Warm-up

- [ ] Run [main.go](main.go). Verify the output walks list → create → get → not-found → update → delete → list.
- [ ] Open `psql` in another terminal (`docker compose exec postgres psql -U app -d appdb`) and run `SELECT * FROM todos;` between Go runs — see the rows the program created/updated.
- [ ] Look at the program's source for **all** the `// MUST` / pool / context comments. Re-read each one.

---

## Task 1 — Turn it into a CLI

Right now the program does one fixed sequence. Make it real:

- [ ] Read the first command-line argument (`os.Args[1]`).
- [ ] Support these subcommands:
  ```
  go run . list                   # list all
  go run . add "buy milk"         # insert, prints new ID
  go run . get 3                  # fetch one, errors if missing
  go run . done 3                 # mark done
  go run . delete 3
  ```
- [ ] Print a usage message if the subcommand is unknown.

**Why:** trains you on argument parsing + the same handler pattern you'll use over HTTP later (`switch` on a string → call a function). Day 12 just replaces "subcommand" with "HTTP method + path".

---

## Task 2 — `ErrNotFound` instead of formatted strings

`markDone` and `deleteTodo` return `fmt.Errorf("no todo with id=%d", id)`. That's a string. A caller can't `errors.Is` it.

- [ ] Declare `var ErrNotFound = errors.New("todo not found")`.
- [ ] Return `fmt.Errorf("markDone id=%d: %w", id, ErrNotFound)` from both functions.
- [ ] In `main`, check with `errors.Is(err, ErrNotFound)` and print a friendly "not found" — fatal-log everything else.

**Why:** typed errors with `%w` + `errors.Is` is the Day 16 pattern. You're previewing it.

---

## Task 3 — Search by title

- [ ] Add `searchTodos(ctx, db, term)` that runs:
  ```sql
  SELECT id, title, done FROM todos
  WHERE  title ILIKE '%' || $1 || '%'
  ORDER  BY id
  ```
- [ ] Hook it up to a `go run . search "sql"` subcommand.
- [ ] **Test the safety:** try `go run . search "%'"`. It should return 0 rows, not blow up — that's parameterised queries doing their job. Try the same with `fmt.Sprintf` to feel the difference (then revert).

**Why:** muscle memory for SQL injection avoidance.

---

## Task 4 — Read DSN from env

- [ ] Replace the hard-coded `dsn` with `os.Getenv("DATABASE_URL")`.
- [ ] If empty, fall back to the local default.
- [ ] Test:
  ```powershell
  $env:DATABASE_URL = "postgres://app:app@localhost:5432/appdb?sslmode=disable"
  go run . list
  ```
- [ ] Bonus: print "using env DATABASE_URL" vs "using default DSN" on startup.

**Why:** Day 13 makes a `config` package. Today's `os.Getenv` is the first step.

---

## Task 5 — `context.WithTimeout` on a query

- [ ] Add a `go run . slow` subcommand that runs `SELECT pg_sleep(3)`.
- [ ] Wrap the query call in `context.WithTimeout(ctx, 1*time.Second)`.
- [ ] Run it. The query should fail with `context deadline exceeded`.
- [ ] Check `psql`'s output — the cancelled query shows up as `cancelling statement due to user request`.

**Why:** runaway queries are a real prod problem. Every query gets a `ctx`, every `ctx` should have a deadline.

---

## Task 6 — `LastInsertId` does NOT work on Postgres — prove it

- [ ] Replace your `INSERT ... RETURNING id` with a plain `INSERT INTO todos (title) VALUES ($1)` and try `res, _ := db.Exec(...); id, _ := res.LastInsertId()`.
- [ ] Observe that `LastInsertId()` returns an error like `LastInsertId is not supported by this driver`. (MySQL would have worked; Postgres doesn't.)
- [ ] Revert. This is why `RETURNING id` is the Postgres-with-Go idiom.

**Why:** writing the bug yourself once means you'll never write it for real.

---

## Task 7 — A simple transaction (preview Day 29)

- [ ] Add a `go run . swap A B` subcommand that **swaps the `done` flags** of two todos in one transaction.
- [ ] Use `tx, err := db.BeginTx(ctx, nil)`, then `tx.ExecContext(...)` twice, then `tx.Commit()`.
- [ ] On any error, `tx.Rollback()` and return.
- [ ] Verify: in `psql`, do `BEGIN; UPDATE todos SET done = NOT done WHERE id IN (1, 2); ROLLBACK;` — same semantics, manual.

**Why:** transactions are coming up in Day 29 properly. Today's task is the smallest-thing-that-works version.

---

## Stretch — only if you're flying

- [ ] **Switch to native `pgxpool`.** Replace `database/sql` with `github.com/jackc/pgx/v5/pgxpool`. Notice that the API is nearly identical (`pool.Exec`, `pool.QueryRow`) but you lose the `database/sql` interface. Decide which you'd ship.
- [ ] **Inspect connections.** While your program is running, in `psql`:
  ```sql
  SELECT pid, state, query FROM pg_stat_activity WHERE datname = 'appdb';
  ```
  See your pool's idle connections. Watch them appear and disappear with different `SetMaxOpenConns` values.
- [ ] **Read** `database/sql.DB.Query` source — about 50 lines. Skim — see how `db.Query` calls `db.conn()` which actually borrows from the pool.

