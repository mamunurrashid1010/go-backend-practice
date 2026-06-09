# Day 24 — Practice Tasks

The slow tests are where the real bugs hide.

> **Before you start:**
>
> 1. Docker Desktop running.
> 2. Install deps:
>    ```powershell
>    go mod init day24
>    go get github.com/go-chi/chi/v5
>    go get github.com/jackc/pgx/v5/stdlib
>    go get github.com/jackc/pgx/v5/pgconn
>    go get -tags 'postgres' github.com/golang-migrate/migrate/v4
>    go get github.com/golang-migrate/migrate/v4/database/postgres
>    go get github.com/golang-migrate/migrate/v4/source/file
>    go get github.com/joho/godotenv
>    go get github.com/caarlos0/env/v11
>    go get github.com/go-playground/validator/v10
>    go get golang.org/x/crypto/bcrypt
>    go get github.com/golang-jwt/jwt/v5
>    go get github.com/testcontainers/testcontainers-go
>    go get github.com/testcontainers/testcontainers-go/modules/postgres
>    ```
> 3. Run:
>    ```powershell
>    go test -v ./internal/notes/                   # fast (Day 22+23)
>    go test -v -tags integration ./internal/notes/ # slow (Day 22+23+24)
>    ```

The first integration run pulls postgres:16-alpine — give it a minute. Later runs are ~10s for container boot + ~50ms per test.

---

## Warm-up

- [ ] Fast tests are green (no Docker needed).
- [ ] Integration tests are green (Docker required). Look for `--- PASS: TestPGRepo_*`.
- [ ] Watch the Docker desktop UI while a run happens: a temporary container appears, runs, terminates.

---

## Task 1 — Break the SQL, watch a test fail

This is the day's most valuable exercise.

- [ ] In [internal/notes/repository.go](internal/notes/repository.go), change `Get`'s SQL:
  ```sql
  WHERE id = $1 AND user_id = $2     -- before
  WHERE id = $1                       -- AFTER (the IDOR bug)
  ```
- [ ] Run `go test -tags integration ./internal/notes/`.
- [ ] `TestPGRepo_Get_ScopedByUserID` should FAIL — user B can now see A's note.
- [ ] Revert. **This bug is invisible to unit tests with a mock repository. Only the integration test catches it.**

---

## Task 2 — Force a unique-violation

The `auth.PostgresUserRepository.Create` translates `23505` into `*ConflictError`. Prove it.

- [ ] Add an integration test (same file or a new `auth_repository_test.go`):
  ```go
  func TestUserRepo_DuplicateEmail(t *testing.T) {
      resetDB(t)
      repo := auth.NewPostgresUserRepository(testDB)
      _, err := repo.Create(testCtx, "a@b.dev", "hash")
      if err != nil { t.Fatal(err) }
      _, err = repo.Create(testCtx, "a@b.dev", "hash2")
      var conflict *auth.ConflictError
      if !errors.As(err, &conflict) {
          t.Fatalf("want *ConflictError, got %v", err)
      }
      if conflict.Field != "email" {
          t.Errorf("field: want email, got %q", conflict.Field)
      }
  }
  ```
- [ ] This test ONLY runs against real Postgres — a mock can't simulate the SQLSTATE 23505.

---

## Task 3 — Test the refresh-token cascade

The `refresh_tokens.user_id` FK has `ON DELETE CASCADE`. Prove it.

- [ ] Add:
  ```go
  func TestRefreshToken_DeletedWithUser(t *testing.T) {
      resetDB(t)
      repo := auth.NewPostgresRefreshTokenRepository(testDB)
      uid := seedUser(t, "a@b.dev")
      _, _ = repo.Create(testCtx, uid, "hash1", time.Now().Add(time.Hour))
      _, _ = repo.Create(testCtx, uid, "hash2", time.Now().Add(time.Hour))
      
      // Delete the user; FK cascade should wipe both tokens.
      _, _ = testDB.Exec("DELETE FROM users WHERE id = $1", uid)
      
      var n int
      testDB.QueryRow("SELECT COUNT(*) FROM refresh_tokens").Scan(&n)
      if n != 0 {
          t.Errorf("cascade failed: %d tokens remain", n)
      }
  }
  ```

**Why:** schema invariants (constraints, cascades, defaults) are part of your contract. Tests catch regressions when someone "cleans up" the migration.

---

## Task 4 — Per-test isolation via `t.Cleanup`

`resetDB` runs at the **start** of each test. An alternative pattern uses `t.Cleanup` to reset at the **end** — slightly nicer reads.

- [ ] Add an alternative helper:
  ```go
  func newCleanRepo(t *testing.T) *PostgresRepository {
      resetDB(t)
      t.Cleanup(func() { resetDB(t) })
      return NewPostgresRepository(testDB)
  }
  ```
- [ ] Use it in one test. The "before+after" pattern is more paranoid; the "before only" pattern is faster. Decide what you like.

---

## Task 5 — Coverage now

```powershell
go test -tags integration -coverprofile=cover.out ./internal/notes/
go tool cover -html=cover.out
```

- [ ] What's still red? Probably some error paths in `repository.go` that the happy-path tests don't hit. Add 1 more test that triggers `rows.Err()` for a deliberately bad SQL (advanced; you might skip).

---

## Stretch — only if you're flying

- [ ] Look at `t.Skip()` — skip an integration test if `os.Getenv("CI") == ""` (or some similar gate). Useful when an integration test depends on a service you don't have locally.
- [ ] **Make the container reusable across runs** with `testcontainers.WithReuse`. First `go test` boots Postgres; second `go test` skips boot — tests start in milliseconds. Trade-off: the container outlives your test runs. Useful for the dev loop, off in CI.
- [ ] Read the testcontainers-go Postgres module's source — under 200 lines: <https://github.com/testcontainers/testcontainers-go/blob/main/modules/postgres/postgres.go>. The whole "spin up Postgres" magic is just a wait strategy + env vars.
- [ ] Compare to `dockertest` (an older library that does the same job differently). testcontainers is the modern answer.

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
