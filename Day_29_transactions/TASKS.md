# Day 29 — Practice Tasks

The implementation gives you working transactions on every notes mutation. The tasks make you *prove* the atomicity, then explore the corners — isolation levels and savepoints — that the basic happy path doesn't touch.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day29
> go mod tidy   # pulls every dep including any new ones
> go run .
> ```

---

## Warm-up — see both rows commit together

- [ ] Register + login. Save `$tok`.
- [ ] Create a note:
  ```powershell
  curl.exe -s -H "Authorization: Bearer $tok" -H "Content-Type: application/json" `
    -d "{\"title\":\"first\"}" http://localhost:8080/notes
  ```
- [ ] Read your audit log:
  ```powershell
  curl.exe -s -H "Authorization: Bearer $tok" http://localhost:8080/audit | ConvertFrom-Json
  ```
- [ ] In psql, confirm both rows share roughly the same `created_at` (they were committed together):
  ```powershell
  docker exec -it day29-pg psql -U app -d appdb -c "SELECT id, action, target_id, created_at FROM audit_logs ORDER BY id;"
  ```

---

## Task 1 — Force the audit insert to fail; watch the note disappear

Temporarily break the audit insert and confirm the note insert rolls back.

- [ ] In [internal/audit/repository.go](internal/audit/repository.go), inside `Log`, add a forced failure right before the `ExecContext`:
  ```go
  return errors.New("forced failure for Task 1")
  ```
- [ ] Restart the server. `POST /notes` should now return **500 Internal Server Error**.
- [ ] In psql:
  ```sql
  SELECT count(*) FROM notes WHERE user_id = <your id> AND title = 'forced fail test';
  ```
  **Expect 0 rows.** Even though the note insert succeeded inside the tx, the audit insert failed → tx rolled back → note is gone.
- [ ] Revert the change.

Without the transaction, the note would still exist. This is the entire teaching of the day.

---

## Task 2 — Confirm rollback in mid-flight via panic

Same idea with a different failure mode:

- [ ] In `audit.Log`, replace the forced error with `panic("boom")`.
- [ ] Hit `POST /notes`. The Recover middleware catches the panic and returns 500.
- [ ] The Transactor's deferred recover-and-rollback runs *before* the middleware's recover — so the note doesn't survive.
- [ ] Confirm in psql.
- [ ] Revert.

---

## Task 3 — Verify DELETE is transactional too

- [ ] Create a second note with `POST /notes`.
- [ ] Inside `audit.Log`, force-fail again (this time it'll happen on delete).
- [ ] Try `DELETE /notes/<id>` → 500.
- [ ] In psql, confirm the note **still exists**. The delete inside the tx was rolled back.
- [ ] Revert.

---

## Task 4 — `defer tx.Rollback()` antipattern

Read `dbtx.InTxOpts` carefully. Compare to the "always defer rollback" form:

```go
defer tx.Rollback() // ignored after Commit
```

- [ ] Add this version as a second method `InTxLazy` for comparison.
- [ ] Inside it, deliberately have `Commit()` "fail" by closing the underlying connection just before (`db.Close()` is too brutal; instead, just simulate by manually returning a fake commit error).
- [ ] Notice that the `defer Rollback` swallows the actual commit error because it returns `sql.ErrTxDone`. Bug city.
- [ ] Delete `InTxLazy`. Write 2 lines under "What I learned" about why the explicit form is worth the extra code.

---

## Task 5 — Try Serializable isolation

The current `InTx` runs at Read Committed. Most operations are fine there. But the **"create a note only if user has < 10 notes"** rule has a classic race:

- Tx A: `SELECT count(*) ... → 9`. About to insert.
- Tx B: `SELECT count(*) ... → 9`. About to insert.
- Both insert. User now has 11.

- [ ] Add a `CreateLimited` method to `notes.Service` that:
  1. Counts the user's notes.
  2. Returns `ErrTooManyNotes` if ≥ 10.
  3. Otherwise inserts (with audit).
- [ ] Wrap it in `tx.InTxOpts(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable}, ...)`.
- [ ] Add a `POST /notes/limited` route that calls it.
- [ ] Open two psql sessions both running:
  ```sql
  BEGIN ISOLATION LEVEL SERIALIZABLE;
  SELECT count(*) FROM notes WHERE user_id = 1;
  INSERT INTO notes (user_id, title) VALUES (1, '...');
  COMMIT;
  ```
  Run them concurrently. One should fail with `40001 serialization_failure`.
- [ ] In Go, that surfaces as a `*pgconn.PgError` with `Code == "40001"`. Add a retry loop around `InTxOpts` (max 3 attempts).

This is the core of Day 30's optimistic-concurrency-control teaser.

---

## Task 6 — Savepoints

`database/sql` doesn't expose `Savepoint` — you do it via `tx.ExecContext`.

- [ ] In `dbtx`, add:
  ```go
  func (t *Transactor) SubTx(ctx context.Context, name string, fn func(context.Context) error) error
  ```
  which inside an existing tx issues `SAVEPOINT name`, runs fn, then `RELEASE SAVEPOINT name` or `ROLLBACK TO SAVEPOINT name` on error.
- [ ] Use it in a service method that does:
  1. Insert a note.
  2. Try to insert an audit row tagged `optional`. If it fails, roll back the savepoint but continue.
  3. Commit the outer tx with the note intact.
- [ ] Show in psql that the note exists, the audit row doesn't.

**Note:** in production you almost never need savepoints — they're useful when *one part* of a multi-step operation is genuinely optional and you don't want a single failure to abort the whole tx.

---

## Task 7 — A test for the Transactor

The dbtx package has a smoke test but no end-to-end test. Add:

- [ ] `internal/dbtx/dbtx_pg_test.go` with build tag `integration`:
  ```go
  //go:build integration
  ```
- [ ] Spin up a `testcontainers-go` Postgres (Day 24 pattern).
- [ ] Create a temp table, write two rows via `InTx`, commit. Read them back.
- [ ] Write a third row, return an error from the fn. Read — should NOT find the third row.

---

## Stretch — only if you're flying

- [ ] **Retry on `40001`**: extract a `RunWithRetry(ctx, fn, attempts)` helper in `dbtx`. Default to 3. Apply it to any Serializable operation.
- [ ] **Read-only tx**: a `GET /audit` that *might* benefit from `&sql.TxOptions{ReadOnly: true}` — does it? When?
- [ ] **`SELECT ... FOR UPDATE`**: an alternative to Serializable for the "count + insert" race. Implement it; compare.
- [ ] **Audit pagination**: `GET /audit` returns up to 200 entries. Add cursor pagination like notes.

---

## What I learned (Day 29)

> 3 bullets in your own words.

-
-
-
