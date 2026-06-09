# Day 24 — Integration Tests with `testcontainers-go`

> **Goal:** test the **real** `PostgresRepository` against a **real** Postgres in a Docker container. This is what catches SQL typos, missing indexes, wrong COALESCE arguments, and IDOR bugs (the `WHERE user_id = $N` clause that mocks can't verify).

After today, every layer is tested:
- **Day 22** — service logic, with a fake repo (microseconds).
- **Day 23** — HTTP handlers, with a real service + fake repo (milliseconds).
- **Day 24 (today)** — Postgres SQL, with `testcontainers`-managed real Postgres (~10s startup, then microseconds per test).

---

## 1. Why integration tests can't be skipped

A fake repository will tell you the service propagates errors correctly. It will **not** tell you:

- That your `COALESCE($n, column)` PATCH actually keeps the existing value when `$n` is `NULL`.
- That `WHERE id = $1 AND user_id = $2` rejects requests from the wrong user (the **IDOR defense** Day 21 relied on).
- That `RETURNING id, user_id, title, body, created_at, updated_at` produces non-zero `created_at`.
- That a unique constraint actually fires (`23505`).
- That a missing migration breaks the build before deploy.

Those bugs only surface when SQL runs. Day 24's job is to surface them in CI, not in prod.

---

## 2. `testcontainers-go` — what it does

[`github.com/testcontainers/testcontainers-go`](https://github.com/testcontainers/testcontainers-go) is a Go library that:

- Pulls and starts Docker containers from your test code.
- Waits for the container to be ready (health-check style).
- Exposes the container's `host:port` to your tests.
- Terminates the container when the test ends.

The `postgres` module wraps the official Postgres image with a friendly API:

```go
import (
    "github.com/testcontainers/testcontainers-go"
    "github.com/testcontainers/testcontainers-go/modules/postgres"
    "github.com/testcontainers/testcontainers-go/wait"
)

container, _ := postgres.Run(ctx, "postgres:16-alpine",
    postgres.WithDatabase("testdb"),
    postgres.WithUsername("test"),
    postgres.WithPassword("test"),
    testcontainers.WithWaitStrategy(
        wait.ForLog("database system is ready to accept connections").
            WithOccurrence(2).
            WithStartupTimeout(60 * time.Second),
    ),
)
dsn, _ := container.ConnectionString(ctx, "sslmode=disable")
```

You get a DSN. Apply your migrations to it. Open a `*sql.DB`. Hand it to the real `PostgresRepository`. Run tests.

Prereqs: **Docker Desktop running.** No special setup beyond that.

---

## 3. The shared-container pattern

Three ways to manage the container:

| Strategy | Container lifecycle | Pros / Cons |
| --- | --- | --- |
| Per test | Start + migrate + terminate inside every test | Most isolated; **dog slow** (10s startup × N tests). |
| Per test, with **reusable** container | First test starts it; later tests skip startup | Faster, but cross-test ordering matters. |
| **Per package (recommended)** | `TestMain` starts it once, terminates at the end. Each test `TRUNCATE`s the tables. | One container per `go test` invocation; tests are fast and isolated by data, not by container. |

We use the third. [`TestMain`](https://pkg.go.dev/testing#hdr-Main) is a special function `testing` calls once per package — perfect for shared setup.

```go
func TestMain(m *testing.M) {
    container, db, cleanup := startPostgres()
    defer cleanup()
    testDB = db
    os.Exit(m.Run())
}

func resetDB(t *testing.T) {
    t.Helper()
    _, err := testDB.Exec(`TRUNCATE notes, refresh_tokens, users RESTART IDENTITY CASCADE`)
    if err != nil { t.Fatalf("reset: %v", err) }
}
```

Every test starts with `resetDB(t)` → empty tables → seed users → run the operation → assert. Identity sequences restart so IDs are predictable.

---

## 4. Build tags — keep slow tests off the default path

Integration tests are slow (~10s container boot, ~1ms per test after). You don't want them in every `go test` invocation. The fix: a **build tag** at the top of the file:

```go
//go:build integration

package notes
```

That comment is special syntax. The file is compiled **only** when you pass `-tags integration`:

```powershell
go test ./internal/notes/                       # Day 22 + 23 only (fast, no Docker)
go test -tags integration ./internal/notes/     # Day 22 + 23 + 24 (slow, needs Docker)
```

CI usually runs both — fast tests on every push, integration tests on PRs or nightly.

---

## 5. The tests we write today

Each test is a tiny round-trip against real Postgres:

| Test | What it proves |
| --- | --- |
| `TestPGRepo_Create_RoundTrip` | INSERT returns id + server timestamps |
| `TestPGRepo_Get_NotFound` | `sql.ErrNoRows` → `ErrNotFound` translation works |
| `TestPGRepo_Get_ScopedByUserID` | **THE IDOR test.** Note 1 belongs to user A; user B's `Get` returns `ErrNotFound`. |
| `TestPGRepo_Update_WrongUser_NotFound` | Update on someone else's note returns `ErrNotFound`, not "updated 0 rows" |
| `TestPGRepo_Patch_KeepsExistingField` | `COALESCE($n, column)` actually keeps the column when `$n` is `NULL` |
| `TestPGRepo_Delete_RowsAffected` | `RowsAffected = 0` → `ErrNotFound` |
| `TestPGRepo_List_FilterAndLimit` | `search`, `limit` propagate to SQL correctly |

The **most important one** is `TestPGRepo_Get_ScopedByUserID`. Mocks can't catch the bug where someone refactors the SQL to `WHERE id = $1` (drops `AND user_id = $2`). Real Postgres can.

---

## 6. What's in this folder

| File | Status |
| --- | --- |
| `internal/notes/postgres_repository_test.go` | **NEW** with `//go:build integration` |
| `internal/notes/service_test.go` | carried from Day 22 (no build tag, runs always) |
| `internal/notes/handler_test.go` | carried from Day 23 (no build tag, runs always) |
| Everything else | unchanged from Day 23 |

Three test files, three layers, **three speeds**:

```powershell
# fast — no Docker needed
go test -v -race ./internal/notes/

# slow — needs Docker; spins up Postgres
go test -v -race -tags integration ./internal/notes/
```

---

## 7. Run today's tests

```powershell
cd Day_24_integration_tests
go mod init day24
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/jackc/pgx/v5/pgconn
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go get golang.org/x/crypto/bcrypt
go get github.com/golang-jwt/jwt/v5

# the new ones for testcontainers
go get github.com/testcontainers/testcontainers-go
go get github.com/testcontainers/testcontainers-go/modules/postgres

# fast tests first
go test -v -race ./internal/notes/

# then integration tests (needs Docker Desktop running)
go test -v -race -tags integration ./internal/notes/
```

The integration run looks like:

```
=== RUN   TestPGRepo_Create_RoundTrip
--- PASS: TestPGRepo_Create_RoundTrip (0.01s)
=== RUN   TestPGRepo_Get_ScopedByUserID
--- PASS: TestPGRepo_Get_ScopedByUserID (0.01s)
... etc
PASS
ok    day24/internal/notes  12.4s   ← ~10s of that is container startup
```

After the first run, the Postgres image is cached locally, so subsequent runs are faster.

---

## 8. CI tips

GitHub Actions and similar runners have Docker available — integration tests run natively. A typical job:

```yaml
- run: go test -race ./...                       # fast tests
- run: go test -race -tags integration ./...     # integration
```

When the integration job fails on a typo in SQL, it fails before the code reaches prod. That's the whole win.

---

## 9. What's next

**Day 25** — structured logging with `log/slog` (stdlib). The `log.Printf` calls scattered through the middleware and handlers become typed key-value pairs you can ship to Datadog/Loki/Elastic.

After today, your project has full test coverage of all three layers — and you can prove it. The handler's `200/201/204/400/401/404/422` matrix is covered (Day 23). The service's business rules are covered (Day 22). The Postgres SQL is covered (today). That's a quality story you can put on a resume.
