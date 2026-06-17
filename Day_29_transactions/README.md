# Day 29 — Postgres Transactions

> **Goal:** turn "two SQL statements that should agree" into "two SQL statements that *will* agree, or both undo." Wire `db.BeginTx` + commit/rollback into a `Transactor` helper, propagate the tx through `context.Context` so the repository code stays clean, and use it to wrap **"create a note + write an audit log row"** as one atomic operation.

This is Day 1 of Week 5. From here Phase 3 builds — transactions, indexes, sqlc, Redis, OpenAPI. None of it works without transactions first.

---

## 1. Why transactions

The case for them is the case against them: if you don't have one, you can lose data in five different ways.

The classic example:

```go
// 1. INSERT a new note for the user
note, err := repo.Create(ctx, userID, in)
if err != nil { return err }

// 2. INSERT an audit row describing what just happened
err = audit.Log(ctx, userID, "note.created", note.ID)
if err != nil {
    // ...now what? The note exists. The audit row doesn't.
}
```

Failure modes you've already shipped if you don't use a tx:

- **App crashes between (1) and (2).** Note exists. Audit doesn't. Forever.
- **The audit insert fails** (constraint, disk, network). Same.
- **Another transaction reads** between (1) and (2) and sees a note with no audit row. They make a decision on bad state.
- **(1) and (2) get reordered** in a retry loop and now duplicate-write.
- **Foreign keys** point at half-built rows.

A transaction collapses all five into "either both, or neither." It is the *one* primitive Postgres gives you that you cannot replicate in application code.

---

## 2. `db.BeginTx` — the Go API

```go
tx, err := db.BeginTx(ctx, &sql.TxOptions{
    Isolation: sql.LevelReadCommitted,
    ReadOnly:  false,
})
if err != nil { return err }
defer tx.Rollback() // safe: no-op after Commit

if _, err := tx.ExecContext(ctx, q1, args...); err != nil { return err }
if _, err := tx.ExecContext(ctx, q2, args...); err != nil { return err }

return tx.Commit()
```

Three things people get wrong here:

1. **Forgetting `defer tx.Rollback()`.** A panic mid-transaction leaves the tx open until the connection is closed; Postgres holds locks until then.
2. **Calling methods on `db` instead of `tx` inside the block.** `db.QueryContext(...)` uses a *different* connection — not part of your tx. Your write isn't atomic; it's two writes.
3. **Holding the tx open across a network call** (Stripe, email, …). The tx pins a connection out of the pool for the duration. Don't.

---

## 3. The `Transactor` + tx-in-context pattern

You don't want every repository method to take a `*sql.Tx` parameter — your call sites get noisy and the "either tx or db" decision pollutes everything.

The pattern Go production code mostly settles on:

1. A **`Transactor`** struct that owns `BeginTx` / `Commit` / `Rollback`. It exposes a single method `InTx(ctx, fn)` that takes a function and runs it inside a transaction.
2. **The tx is stored on `context.Context`** inside `fn`. Repository methods don't know whether they're "in a tx" — they just resolve a runner from context.
3. A **`DBTX` interface** satisfied by both `*sql.DB` and `*sql.Tx`. Repos type their executor as `DBTX`.

```go
// internal/dbtx/dbtx.go
type DBTX interface {
    QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
    QueryRowContext(ctx context.Context, q string, args ...any) *sql.Row
    ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

type ctxKey struct{}

func WithTx(ctx context.Context, tx *sql.Tx) context.Context {
    return context.WithValue(ctx, ctxKey{}, tx)
}
func RunnerFor(ctx context.Context, db *sql.DB) DBTX {
    if tx, ok := ctx.Value(ctxKey{}).(*sql.Tx); ok {
        return tx
    }
    return db
}

type Transactor struct{ db *sql.DB }

func (t *Transactor) InTx(ctx context.Context, fn func(context.Context) error) error {
    tx, err := t.db.BeginTx(ctx, nil)
    if err != nil { return fmt.Errorf("begin: %w", err) }
    defer func() {
        if p := recover(); p != nil {
            _ = tx.Rollback()
            panic(p)
        }
    }()
    if err := fn(WithTx(ctx, tx)); err != nil {
        _ = tx.Rollback()
        return err
    }
    return tx.Commit()
}
```

And in the repo:

```go
func (r *PostgresRepository) Create(ctx context.Context, ...) (Note, error) {
    return scanOne(dbtx.RunnerFor(ctx, r.db).QueryRowContext(ctx, q, args...))
}
```

`r.db` is still `*sql.DB` for the simple case. If the caller wrapped the context with `WithTx(...)`, the same code path uses the tx instead.

---

## 4. The teaching example: notes + audit

Migration `000005` adds:

```sql
CREATE TABLE audit_logs (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action      TEXT        NOT NULL,
    target_type TEXT        NOT NULL,
    target_id   BIGINT      NOT NULL,
    metadata    JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_audit_logs_user_id_created_at ON audit_logs (user_id, created_at DESC);
```

The notes service now wraps create/patch/delete in a transaction:

```go
func (s *Service) Create(ctx context.Context, userID int64, in CreateRequest) (Note, error) {
    var out Note
    err := s.tx.InTx(ctx, func(ctx context.Context) error {
        n, err := s.repo.Create(ctx, userID, in)
        if err != nil { return err }
        out = n
        return s.audit.Log(ctx, userID, "note.created", "note", n.ID, map[string]any{
            "title": n.Title,
        })
    })
    return out, err
}
```

Both `repo.Create` and `audit.Log` resolve the same `*sql.Tx` from context via `RunnerFor`. They're in one transaction without ever passing `tx` as an argument.

A new `GET /audit?limit=N` endpoint lets you see your own log:

```json
{
  "data": [
    {"id": 12, "action": "note.created", "target_type": "note", "target_id": 7, "metadata": {"title":"buy milk"}, "created_at": "..."},
    ...
  ]
}
```

---

## 5. Isolation levels — the short version

| Level | Reads see | Stops |
| --- | --- | --- |
| **Read Committed** *(Postgres default)* | committed data only | dirty reads |
| **Repeatable Read** | the same snapshot all tx long | non-repeatable reads, phantoms (in PG) |
| **Serializable** | as if transactions ran one at a time | every anomaly; can fail with `serialization_failure` |

You don't choose by reading a table; you choose by asking *"what concurrent writes can ruin this query?"*

- **Read counter, write counter back +1.** Two concurrent transactions at Read Committed both read N and both write N+1 — you lost one increment. Use `SELECT ... FOR UPDATE` to lock the row, or use Serializable and retry on `40001`.
- **Audit log insert.** Independent rows, no read-then-write. Read Committed is fine.
- **Bank transfer.** Read source balance, check ≥ amount, debit source, credit dest. Serializable handles the entire pattern; Read Committed needs explicit row locking.

In our `Transactor`, the default is Read Committed (Postgres default; `sql.TxOptions{Isolation: 0}` means "use the database's default"). Task 5 has you opt into Serializable for one method and observe the `serialization_failure` retry path.

---

## 6. Savepoints — nested rollbacks within one tx

Sometimes inside a transaction you want to try something that might fail without aborting the whole tx. Postgres gives you savepoints:

```sql
BEGIN;
INSERT INTO notes (...) VALUES (...);
SAVEPOINT before_audit;
INSERT INTO audit_logs (...) VALUES (...);   -- this fails for some reason
ROLLBACK TO SAVEPOINT before_audit;
-- the note insert is still alive; the audit insert isn't
INSERT INTO audit_logs (...) VALUES (...);   -- retry / fix / log
RELEASE SAVEPOINT before_audit;
COMMIT;
```

`database/sql` doesn't have a `Savepoint` method — you just do `tx.ExecContext(ctx, "SAVEPOINT name")`. Task 6 has you implement a `SubTx(ctx, fn)` helper on top.

---

## 7. The deferred-rollback idiom

Two patterns float around. Here's why I use the explicit form:

**The "always defer" form** (most code you'll see):
```go
tx, err := db.BeginTx(ctx, nil)
if err != nil { return err }
defer tx.Rollback() // commit makes this a no-op
...
return tx.Commit()
```

Pro: short. Con: `tx.Rollback()` after a successful `Commit()` returns `sql.ErrTxDone` — you have to silently swallow it, which means you also silently swallow the case where commit *itself* failed. (Yes, `Commit` can fail — disk, network, constraint.)

**The explicit form** (what the `Transactor` uses):
```go
defer func() {
    if p := recover(); p != nil { _ = tx.Rollback(); panic(p) }
}()
if err := fn(ctx); err != nil {
    _ = tx.Rollback()
    return err
}
return tx.Commit()
```

You still cover the panic path. And on the happy path it's `Commit()` only — no swallowed `ErrTxDone` and no ambiguity if commit failed.

---

## 8. Middleware order is unchanged

```
RequestID → Logger → CORS → RateLimit → Recover → routes
```

Transactions live below routes — at the service layer. Middleware doesn't know about them. This is by design: transactions are a business-layer concern (where the "this should be atomic" decision is made), not a transport concern.

---

## 9. What changed from Day 28

| File | Change |
| --- | --- |
| `internal/dbtx/` | **NEW** — `DBTX` interface, `Transactor`, `WithTx`, `RunnerFor`, tests |
| `internal/audit/` | **NEW** — Entry model, Postgres repo (`Log` + `List`), handler (`GET /audit`) |
| `internal/notes/repository.go` | uses `dbtx.RunnerFor(ctx, r.db)` instead of `r.db` directly |
| `internal/notes/service.go` | `Create`, `Patch`, `Delete` now wrap their work in `s.tx.InTx(...)` and call `audit.Log` |
| `migrations/000005_create_audit_logs.{up,down}.sql` | **NEW** |
| `main.go` | builds the `Transactor`, audit repo + handler, mounts `/audit` (auth-required) |

`handler.go` files are unchanged — the transaction boundary lives at the service layer where it belongs.

---

## 10. Run it

```powershell
cd Day_29_transactions
docker compose up -d
go mod init day29
go mod tidy
go run .
```

Walk a transaction:

```powershell
# Register, login (as before)
$tok = (curl.exe -s -H "Content-Type: application/json" -d "{\"email\":\"a@b.dev\",\"password\":\"hunter2pass\"}" http://localhost:8080/auth/login | ConvertFrom-Json).access_token

# Create a note — both the notes row AND an audit row appear, atomically
curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
  -d "{\"title\":\"milk\",\"body\":\"and bread\"}" http://localhost:8080/notes

# Read your audit log
curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/audit | ConvertFrom-Json

# Verify in psql: both inserts share a created_at within microseconds
docker exec -it day29-pg psql -U app -d appdb -c "SELECT id, action, target_id, created_at FROM audit_logs;"
```

---

## 11. What's next

**Day 30** — N+1, indexes, `EXPLAIN ANALYZE`, pool tuning. Reading transactions is half the picture; the other half is *what they're doing under load.*
