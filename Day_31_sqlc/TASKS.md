# Day 31 — Practice Tasks

The code already wires sqlc into `notes` and `auth`. The tasks make you actually run `sqlc generate`, break the SQL on purpose, and add an audit query — so the codegen step becomes muscle memory.

> **Before you start:**
>
> ```powershell
> docker compose up -d
> go mod init day31
> go mod tidy
> go run .
> ```

You'll want sqlc installed for Tasks 1, 3, 4:

```powershell
go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest
sqlc version  # confirm 1.27.x
```

---

## Warm-up — same API, different implementation

- [ ] Walk the standard curl loop (login → create note → audit). Everything works.
- [ ] Open [internal/notes/repository.go](internal/notes/repository.go) and notice `Create` is now five lines (was eleven on Day 30). The SQL moved to [queries/notes.sql](queries/notes.sql).
- [ ] Open [internal/db/notes.sql.go](internal/db/notes.sql.go) and confirm it's deterministic — no comments above functions you'd want to keep across regenerations, no hand-edits.

---

## Task 1 — Regenerate from scratch

- [ ] Delete `internal/db/` entirely.
- [ ] `sqlc generate` from the day folder.
- [ ] Confirm the output matches the committed version byte-for-byte (use `git diff` or `git status`).

If anything is different, your sqlc version is off — the README claims 1.27. Lock the version in CI by committing a `.sqlc-version` or using `go install ...@v1.27.0`.

---

## Task 2 — Break the SQL, watch the Go fail

- [ ] In [queries/notes.sql](queries/notes.sql), rename `title` to `headline` in just one query, e.g. `GetNote`:
  ```sql
  SELECT id, user_id, headline, body, created_at, updated_at ...
  ```
- [ ] Run `sqlc generate`. Expect a compile-time error from sqlc — `headline` is not a column of `notes` according to the schema in `migrations/`.

This is the win: bad SQL doesn't compile. Compare to Day 30, where the same typo would only show up at runtime.

- [ ] Revert.

---

## Task 3 — Add an `audit` query in sqlc

Move *just* the `Log` insert from `internal/audit/repository.go` to sqlc. Leave the JOIN strategies hand-written.

- [ ] In `sqlc.yaml`, add `migrations/000005_create_audit_logs.up.sql` to the `schema:` list.
- [ ] Create `queries/audit_logs.sql`:
  ```sql
  -- name: InsertAuditLog :exec
  INSERT INTO audit_logs (user_id, action, target_type, target_id, metadata)
  VALUES ($1, $2, $3, $4, $5);
  ```
- [ ] `sqlc generate`. Expect new files in `internal/db/`.
- [ ] Rewrite `audit.PostgresRepository.Log` to call the generated `InsertAuditLog`. The metadata `[]byte` becomes the 5th arg (sqlc maps JSONB → `[]byte` by default).
- [ ] Run the create-note flow; confirm the audit row still appears.

---

## Task 4 — Try to sqlc the dynamic List query (and fail well)

The notes List has a dynamic WHERE. Try and confirm it doesn't fit.

- [ ] In `queries/notes.sql`, write:
  ```sql
  -- name: ListNotes :many
  SELECT id, user_id, title, body, created_at, updated_at
  FROM   notes
  WHERE  user_id = $1
  AND    ($2::text = '' OR LOWER(title) LIKE $2)
  AND    ($3::bigint = 0 OR id < $3)
  ORDER  BY id DESC
  LIMIT  $4;
  ```
- [ ] `sqlc generate`. It works. But notice you've lost the ability to flip `ORDER BY` direction or swap `id <` vs `id >`. The price of sqlc is **fixed query shape**.
- [ ] Decide: do you want to ship that less-flexible version, or stick with the hand-written one? Justify in "What I learned."
- [ ] Revert (we keep the hand-written List).

---

## Task 5 — Compare line counts

- [ ] Day 30 `internal/notes/repository.go`: count lines.
- [ ] Day 31 `internal/notes/repository.go`: count lines.
- [ ] Day 31 `internal/db/notes.sql.go` + `queries/notes.sql` + `sqlc.yaml`: count lines.

Total goes up, not down. Per-call boilerplate goes down. The bet: as the project grows from 1 to N repositories, sqlc pays for itself. Make a guess where that crossover is for your team.

---

## Task 6 — Pin sqlc with a tools.go

The Go-canonical way to lock developer tools as deps:

- [ ] Create `tools.go`:
  ```go
  //go:build tools

  package tools

  import (
      _ "github.com/sqlc-dev/sqlc/cmd/sqlc"
  )
  ```
- [ ] `go mod tidy`. sqlc is now in `go.mod`.
- [ ] `go install github.com/sqlc-dev/sqlc/cmd/sqlc` installs the locked version.

This pattern works for `migrate`, `golangci-lint`, anything else CI needs that isn't an import dependency.

---

## Stretch — only if you're flying

- [ ] **`sqlc vet`**: enable rule checks for missing parameters, dangerous queries (no `WHERE` on `UPDATE`). Worth it.
- [ ] **`emit_interface: true`**: regenerate; the `Queries` struct now has a `Querier` interface. Useful for mocking in tests.
- [ ] **`emit_json_tags: true`**: regenerate; the generated structs get JSON tags. Could the handler return `db.Note` directly? (No — DB shape ≠ wire shape. But it's a tempting shortcut.)
- [ ] **`pgx/v5` driver**: change `sql_package: pgx/v5` and regenerate. The interface and types differ from `database/sql`. When would you switch?

---

## What I learned (Day 31)

> 3 bullets in your own words.

-
-
-
