# Day 9 — `database/sql` + `pgx` Driver

> **Goal:** connect a Go program to your Day 8 Postgres, run real SQL, and internalise the single most important fact about `database/sql` — **`*sql.DB` is a pool, not a connection.**

---

## 1. Prereq — Day 8's Postgres is running

```powershell
cd ..\Day_08_postgres_sql_basics
docker compose ps
```

You should see `day08-postgres` `(healthy)`. If not: `docker compose up -d`. The DB is reachable on `localhost:5432` (user `app`, password `app`, database `appdb`, table `todos`).

---

## 2. The two layers: `database/sql` + a driver

```
┌─────────────────────────────────────┐
│  your Go code                       │
│  ↓ calls                            │
│  database/sql (stdlib)              │  ← the API: Query, Exec, etc.
│  ↓ delegates to                     │
│  driver (e.g. pgx, pq, lib/pq…)     │  ← the Postgres-specific bits
│  ↓ talks over TCP                   │
│  Postgres                           │
└─────────────────────────────────────┘
```

`database/sql` is the stdlib abstraction. It doesn't know how to speak to Postgres on its own — you pair it with a **driver**. Drivers register themselves on `import`, so you import the driver "for side effects" (the underscore import) and never reference it by name in your code.

The most common Postgres drivers:

| Driver | Mode | Notes |
| --- | --- | --- |
| `github.com/jackc/pgx/v5` (native) | Bypass `database/sql` entirely, use `pgxpool` directly | Fastest, richest types. The choice for new code in 2026. |
| `github.com/jackc/pgx/v5/stdlib` | Wraps `pgx` as a `database/sql` driver | What we use today. Lets you keep the stdlib API. |
| `github.com/lib/pq` | Older driver, still works | No longer recommended — `pgx` is faster and better-typed. |

We're going with **`pgx/v5/stdlib`** because:
- The learning plan says `database/sql` + `pgx`.
- Your code stays portable — every `*sql.DB` method works the same on any driver.
- You can switch to native `pgxpool` later (Day 12 will show how) without changing your DSN or your SQL.

---

## 3. Install

```powershell
go mod init day09
go get github.com/jackc/pgx/v5/stdlib
```

The driver registers itself when you import it:

```go
import _ "github.com/jackc/pgx/v5/stdlib"
```

That underscore means "import for its side effects only" — `pgx`'s `init()` calls `sql.Register("pgx", ...)` so the string `"pgx"` is now a known driver name.

---

## 4. The DSN — connection string

Two formats are accepted. Pick one:

```
postgres://app:app@localhost:5432/appdb?sslmode=disable
```

```
host=localhost port=5432 user=app password=app dbname=appdb sslmode=disable
```

| Piece | What it means |
| --- | --- |
| `app:app@` | username:password |
| `localhost:5432` | DB host + port |
| `/appdb` | database name |
| `sslmode=disable` | no TLS — local-only; in prod you'd use `require` or `verify-full` |

> **In real apps the DSN comes from `os.Getenv("DATABASE_URL")`** — Day 13 covers that. Never hard-code a password in source.

---

## 5. `sql.Open` doesn't open

The most surprising line in the Go database story:

```go
db, err := sql.Open("pgx", dsn)
```

This **does not connect**. It just constructs a `*sql.DB` and validates the arguments. The first real connection happens on the first query (or when you call `db.Ping`).

To verify connectivity at startup, ping explicitly:

```go
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()
if err := db.PingContext(ctx); err != nil {
    log.Fatalf("ping: %v", err)
}
```

If the DSN is wrong, the host is down, or credentials are bad — this is when you find out.

---

## 6. `*sql.DB` is a POOL — internalise this

```go
db, _ := sql.Open("pgx", dsn)
defer db.Close()    // ← call ONCE, at program exit. NEVER between queries.
```

Common beginner instinct: "I should open a connection, use it, close it." That's wrong with `database/sql`.

`*sql.DB` is a **pool** that manages many connections under the hood. You hand it queries; it borrows a connection from the pool, runs the query, and puts the connection back. You do **not** close the pool between requests.

A typical lifecycle:

```
app starts → sql.Open → db.PingContext → ... 1000s of queries ... → db.Close on shutdown
```

This is identical to how chi/middleware uses a shared DB: pass `*sql.DB` around like a global service handle.

### Pool settings

```go
db.SetMaxOpenConns(25)              // max simultaneous DB connections
db.SetMaxIdleConns(25)              // max kept idle in the pool
db.SetConnMaxLifetime(5 * time.Minute) // recycle conns periodically (helps with cloud load balancers)
db.SetConnMaxIdleTime(2 * time.Minute) // close idle conns after this long
```

Two rules of thumb:

1. **Don't pick a number bigger than what Postgres can handle.** Each app gets a slice of Postgres's `max_connections` (default 100 — divide across all your replicas).
2. **Day 30 covers tuning in detail.** Today's defaults are fine.

---

## 7. The three query functions

`database/sql` gives you three flavors. Pick the one that matches "what comes back":

| Method | Returns | When |
| --- | --- | --- |
| `db.ExecContext`     | `sql.Result` (rows affected, last insert ID*) | `INSERT`, `UPDATE`, `DELETE` — no rows back |
| `db.QueryRowContext` | A single `*sql.Row`                          | You expect exactly one row (e.g. `WHERE id = $1`) |
| `db.QueryContext`    | A `*sql.Rows` you iterate                    | A set of rows |

*\* `LastInsertId()` doesn't work for Postgres — use `INSERT ... RETURNING id` and `QueryRowContext` instead.*

### Pattern 1 — INSERT with `RETURNING`

```go
var id int64
const q = `INSERT INTO todos (title) VALUES ($1) RETURNING id`
err := db.QueryRowContext(ctx, q, "buy milk").Scan(&id)
```

### Pattern 2 — one row

```go
var t Todo
const q = `SELECT id, title, done FROM todos WHERE id = $1`
err := db.QueryRowContext(ctx, q, id).Scan(&t.ID, &t.Title, &t.Done)
if errors.Is(err, sql.ErrNoRows) {
    // not found — usually translate to a 404 in the handler
}
```

### Pattern 3 — many rows

```go
const q = `SELECT id, title, done FROM todos ORDER BY id`
rows, err := db.QueryContext(ctx, q)
if err != nil { ... }
defer rows.Close()      // ← MUST. Or the connection leaks.

var out []Todo
for rows.Next() {
    var t Todo
    if err := rows.Scan(&t.ID, &t.Title, &t.Done); err != nil {
        return nil, err
    }
    out = append(out, t)
}
return out, rows.Err()  // ← MUST also check this after the loop
```

The two `// MUST` lines catch the two most common bugs:
- `defer rows.Close()` — without it, the pool runs out of connections.
- `rows.Err()` — `rows.Next() == false` can mean "done iterating" OR "an error occurred". `rows.Err()` tells you which.

### Pattern 4 — write that should affect rows

```go
res, err := db.ExecContext(ctx, `UPDATE todos SET done = true WHERE id = $1`, id)
if err != nil { ... }
n, _ := res.RowsAffected()
if n == 0 {
    // no row matched — 404
}
```

---

## 8. Parameterised queries — non-negotiable

**Always use `$1`, `$2`, `$3` placeholders.** Never build SQL with `fmt.Sprintf` or string concatenation.

```go
// ❌ SQL injection — never do this
q := fmt.Sprintf("SELECT * FROM users WHERE email = '%s'", email)
db.QueryContext(ctx, q)

// ✅ parameterised — safe
db.QueryContext(ctx, "SELECT * FROM users WHERE email = $1", email)
```

The driver sends the SQL and the parameters separately to Postgres. The user input never touches the SQL grammar. This is the single best practice in the entire database story.

(Postgres uses `$1`-style placeholders. MySQL uses `?`. The Go stdlib's interface is the same; the driver translates.)

---

## 9. NULL — two ways

A `SELECT col` where `col` is `NULL` in Postgres makes `Scan` complain unless the destination can hold NULL.

```go
// Way 1: sql.Null* wrappers
var bio sql.NullString
rows.Scan(&id, &bio)
if bio.Valid {
    fmt.Println(bio.String)
}

// Way 2: pointer to the type
var bio *string
rows.Scan(&id, &bio)
if bio != nil {
    fmt.Println(*bio)
}
```

Pointers are usually nicer in Go code. `sql.Null*` types are useful when you need both "is it set?" and "what's the value?" in one struct field.

---

## 10. `context.Context` everywhere — yes, every call

Every `database/sql` call has a `...Context` variant. **Always use it.** Two reasons:

1. **Cancellation.** If the client disconnects mid-request, your DB query keeps running, holding a connection. With `ctx` bound to `r.Context()`, the driver cancels the query when the client disappears.
2. **Timeouts.** `context.WithTimeout(ctx, 2*time.Second)` makes the DB return an error if a query takes too long, instead of pinning a goroutine forever.

```go
queryCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
defer cancel()
rows, err := db.QueryContext(queryCtx, q, args...)
```

Day 11/12 wires this through the handler-service-repository layers.

---

## 11. Common gotchas

- **`sql.Open` doesn't connect.** Always `PingContext` at startup.
- **`db.Close()` is for shutdown.** Don't call it after each query.
- **Forgot `defer rows.Close()`** → connections leak → pool exhausted → app hangs. Race detector + load test catch it.
- **Comparing `err` to `sql.ErrNoRows` with `==`** sometimes fails when err is wrapped. Use `errors.Is(err, sql.ErrNoRows)`.
- **`time.Time` vs `TIMESTAMPTZ`.** `pgx` handles this cleanly; older drivers (`lib/pq`) had quirks. With `pgx` you can just scan into `time.Time`.
- **`int` vs `int64`.** Postgres `BIGINT` is 64-bit. Always scan into `int64` for ID columns.

---

## 12. Today's code

[main.go](main.go) is a small standalone program (no HTTP yet) that demonstrates every concept above in order:

1. `sql.Open` with the pgx driver
2. `db.SetMaxOpenConns` etc. — pool tuning
3. `db.PingContext` to verify
4. `db.QueryContext` + `rows.Next()` + `rows.Scan()` + `defer rows.Close()` to list todos
5. `db.QueryRowContext` with `RETURNING id` to insert
6. `db.QueryRowContext` to fetch one (and handle `sql.ErrNoRows`)
7. `db.ExecContext` for UPDATE and DELETE with `RowsAffected()`

Run it:

```powershell
go mod init day09
go get github.com/jackc/pgx/v5/stdlib
go run .
```

You'll see logs walking through each operation. Open `psql` in another terminal to watch the rows change in real time.

---

## 13. What's next

**Day 10** introduces migrations with `golang-migrate` — versioned schema changes you can run forward and backward. Day 11 puts the handler/service/repository layering on top so the HTTP API can swap from in-memory to Postgres without touching the handlers. Day 12 wires it all together.
