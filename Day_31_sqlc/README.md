# Day 31 — `sqlc`

> **Goal:** delete the hand-rolled `rows.Scan` boilerplate for every fixed-shape query. Move the SQL into `.sql` files with annotations; `sqlc generate` turns them into typed Go. Side-effects:
> - the SQL is readable in isolation (no Go quoting),
> - the columns/types are checked at codegen time against the actual schema,
> - the function signature you call from Go is generated from the SQL — refactor the SQL, the Go re-compiles or fails loudly.
>
> Cost: dynamic SQL doesn't fit. We **don't** sqlc-ify the notes List (search + cursor + sort), the audit JOIN strategies (Day 30), or the refresh-token recursive CTE (Day 20). That's the realistic distribution — sqlc for the 80%, hand-written for the 20%.

---

## 1. The shape of an sqlc workflow

Three files do all the work:

1. **`sqlc.yaml`** — what schema, where the queries live, where the generated Go goes.
2. **`queries/*.sql`** — your SQL, annotated with `-- name: Foo :one` style comments.
3. **`internal/db/*`** — committed, generated. You re-run `sqlc generate` when the SQL changes.

```
Day_31_sqlc/
├── sqlc.yaml              <-- config (committed)
├── queries/
│   ├── users.sql          <-- you write
│   └── notes.sql          <-- you write
├── internal/
│   ├── db/                <-- generated; committed; re-run sqlc when SQL changes
│   │   ├── db.go
│   │   ├── models.go
│   │   ├── users.sql.go
│   │   └── notes.sql.go
│   ├── notes/
│   │   └── repository.go  <-- thin adapter: db.Notes ↔ notes.Note
│   └── auth/
│       └── repository.go  <-- thin adapter: db.User ↔ auth.User
└── migrations/            <-- sqlc reads these as the schema
```

### Why generated code is committed

A "committed generated" file is one you don't write but everyone reads. sqlc emits stable, deterministic Go — same input, same output. Committing it means:

- New contributors don't need sqlc installed to build.
- CI doesn't have to regenerate before tests.
- Code review can see what changed when the SQL did.

Re-running `sqlc generate` is part of the **schema-change workflow**, like running migrations. Day 56's Makefile will wire it.

---

## 2. The annotation language

Each query is a single SQL statement preceded by `-- name: FunctionName :return_kind`.

| Annotation | Generated signature shape | Use for |
| --- | --- | --- |
| `:one` | `(T, error)` — returns `sql.ErrNoRows` if no row | single-row SELECT / `INSERT ... RETURNING` / `UPDATE ... RETURNING` |
| `:many` | `([]T, error)` | list SELECT |
| `:exec` | `error` | INSERT/UPDATE/DELETE you don't care about the row count |
| `:execrows` | `(int64, error)` | DELETE/UPDATE where you check 0 rows-affected |

Example — [queries/notes.sql](queries/notes.sql):

```sql
-- name: GetNote :one
SELECT id, user_id, title, body, created_at, updated_at
FROM   notes
WHERE  id = $1 AND user_id = $2;

-- name: DeleteNote :execrows
DELETE FROM notes WHERE id = $1 AND user_id = $2;
```

sqlc emits:

```go
type GetNoteParams struct {
    ID     int64
    UserID int64
}
func (q *Queries) GetNote(ctx context.Context, arg GetNoteParams) (Note, error)

type DeleteNoteParams struct {
    ID     int64
    UserID int64
}
func (q *Queries) DeleteNote(ctx context.Context, arg DeleteNoteParams) (int64, error)
```

Multi-param queries get a `<FuncName>Params` struct. Single-param queries take the value directly: `GetUserByID(ctx, id int64)`.

---

## 3. The runner — playing nicely with `dbtx`

sqlc's `Queries` struct holds a `DBTX` — its own interface, satisfied by `*sql.DB` and `*sql.Tx`. Ours from Day 29 is similar but smaller. The repo bridges them:

```go
func (r *PostgresRepository) q(ctx context.Context) *db.Queries {
    // dbtx.RunnerFor returns *sql.DB or *sql.Tx wrapped in our DBTX.
    // The type assertion to db.DBTX (a wider interface that includes
    // PrepareContext) succeeds because both concrete types satisfy it.
    return db.New(dbtx.RunnerFor(ctx, r.db).(db.DBTX))
}
```

Result: every sqlc query honours an in-flight transaction on `ctx` for free. The Day 29 audit-log write keeps working without sqlc knowing transactions exist.

---

## 4. What we moved to sqlc — and what we didn't

### Moved

- **`auth.PostgresUserRepository`** — `Create`, `GetByEmail`, `GetByID`. Three identical-shape `rows.Scan` blocks → three generated functions.
- **`notes.PostgresRepository`** — `Create`, `Get`, `Update`, `Patch`, `Delete`. The `Patch` query uses `COALESCE($3, title)` and the param type becomes `sql.NullString` — sqlc reads the nullable shape from `?` markers on the param. (See [queries/notes.sql](queries/notes.sql) for the `sqlc.narg` annotation.)

### Not moved

- **`notes.PostgresRepository.List`** — dynamic WHERE (search optional, cursor optional, sort direction param). sqlc has limited dynamic-SQL support; the cost of contortion is more than the benefit. Kept hand-written.
- **`audit.PostgresRepository` (all three JOIN strategies)** — same: dynamic, and the schema strategy is the lesson. The Day 30 code stands.
- **`auth.PostgresRefreshTokenRepository`** — `RevokeFamilyDescendants` uses a recursive CTE. sqlc handles it, but the rest of the file is small enough to keep symmetric.

This is the realistic distribution. sqlc isn't a religion; it's a tool that pays for itself on routine CRUD and stays out of the way for the hard SQL.

---

## 5. The repo before and after

**Before** (Day 30) — every method scans by hand:

```go
func (r *PostgresRepository) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
    const q = `INSERT INTO notes (user_id, title, body) VALUES ($1, $2, $3)
               RETURNING id, user_id, title, body, created_at, updated_at`
    var n Note
    err := r.run(ctx).QueryRowContext(ctx, q, userID, in.Title, in.Body).
        Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.CreatedAt, &n.UpdatedAt)
    if err != nil {
        return Note{}, fmt.Errorf("create: %w", err)
    }
    return n, nil
}
```

**After** (Day 31) — sqlc owns the SQL + scan; repo translates types:

```go
func (r *PostgresRepository) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
    row, err := r.q(ctx).CreateNote(ctx, db.CreateNoteParams{
        UserID: userID, Title: in.Title, Body: in.Body,
    })
    if err != nil {
        return Note{}, fmt.Errorf("create: %w", err)
    }
    return fromDB(row), nil
}
```

The `fromDB(row)` converter is two lines and lives at the bottom of the file. The signature change is mechanical; the diff is small.

What you gain:
- Rename a column in SQL → `sqlc generate` regenerates → Go fails to compile at every wrong reference. No "I forgot to update this scan call."
- The SQL is testable in isolation against psql.
- The `Patch` nullable shape is type-safe — you literally can't pass a wrong type for `title`.

What you lose:
- Two more files to keep in sync (the SQL and the generated Go). The Makefile (Day 53) makes this a one-command target.

---

## 6. The alternatives, briefly

The plan asks for a comparison vs `sqlx` and `squirrel`.

### `sqlx` — annotated `database/sql`

```go
type Note struct {
    ID    int64  `db:"id"`
    Title string `db:"title"`
}
var n Note
err := db.GetContext(ctx, &n, "SELECT * FROM notes WHERE id = $1", id)
```

- **Pro:** zero codegen. One library, ~100 lines of glue, you're done. Maps any query into any struct.
- **Pro:** dynamic SQL is fine — you build the string yourself.
- **Con:** no codegen → no compile-time guarantee that columns match. You can `SELECT id, nname` and `sqlx` cheerfully scans whatever happens to align.
- **When:** prototypes, very small projects, or codebases that need lots of dynamic SQL.

### `squirrel` — fluent query builder

```go
q, args, _ := sq.
    Select("id", "title").From("notes").
    Where(sq.Eq{"user_id": userID}).
    OrderBy("id DESC").Limit(20).
    ToSql()
rows, _ := db.QueryContext(ctx, q, args...)
```

- **Pro:** dynamic SQL feels natural — composition is in Go, not string concat. Great for our notes List.
- **Con:** you still write the `Scan` by hand. The complexity moves from SQL strings to Go method chains.
- **When:** the dynamic-SQL portion of your codebase is large enough that hand-building strings becomes brittle.

### sqlc

- **Pro:** type safety, schema-checked at codegen, zero runtime overhead, the queries.sql files are independently readable.
- **Con:** dynamic SQL needs escape valves. Codegen step in the dev loop.
- **When:** most CRUD-heavy services. The sweet spot for "Postgres + Go" in 2024+.

The truthful answer is most production codebases use **two of these**: sqlc for the fixed surface, squirrel (or hand-built) for the genuinely dynamic 5–20% — which is exactly what this repo now does (sqlc + hand-written cursor/JOIN queries).

---

## 7. Installing and running sqlc

```powershell
# One-time install (Go tooling):
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest

# Regenerate after editing queries/*.sql:
sqlc generate

# Verify SQL against the schema without writing files:
sqlc vet     # (with database connection)
sqlc compile # (offline check)
```

The generated `internal/db/*` files are **committed in this repo** so `go run .` works out of the box without you needing sqlc installed. Re-running `sqlc generate` should produce the same output unless you change SQL or `sqlc.yaml`.

---

## 8. What changed from Day 30

| File | Change |
| --- | --- |
| `sqlc.yaml` | **NEW** — config |
| `queries/users.sql`, `queries/notes.sql` | **NEW** |
| `internal/db/db.go`, `models.go`, `users.sql.go`, `notes.sql.go` | **NEW** (generated, committed) |
| `internal/auth/repository.go` | rewritten — wraps sqlc |
| `internal/notes/repository.go` | `Create`/`Get`/`Update`/`Patch`/`Delete` rewritten to wrap sqlc; `List` unchanged (dynamic) |

Day 30's audit-with-JOIN strategies stay hand-written — that's the dynamic-SQL escape valve in action.

---

## 9. Run it

```powershell
cd Day_31_sqlc
docker compose up -d
go mod init day31
go mod tidy
go run .
```

Walk the curl sequence from prior days — same API, same responses. The interesting thing here isn't the API, it's the diff in [internal/notes/repository.go](internal/notes/repository.go) vs Day 30.

```powershell
# Same Day 29/30 sanity check works
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token
curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -d "{\"title\":\"sqlc test\"}" http://localhost:8080/notes
curl.exe -s -H "Authorization: Bearer $tok" "http://localhost:8080/audit?include=notes" | ConvertFrom-Json
```

---

## 10. What's next

**Day 32 — Redis + cache-aside.** Same notes API; now `GET /notes/{id}` hits Redis first, falls back to Postgres on miss, fills the cache on the way out. TTLs, invalidation on update/delete, the thundering-herd / single-flight problem.
