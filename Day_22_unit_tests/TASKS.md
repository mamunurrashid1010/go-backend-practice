# Day 22 — Practice Tasks

The tests give a sandbox: change one thing, see what fails. That's the feedback loop you want.

> **Before you start:**
>
> ```powershell
> go mod init day22
> go get github.com/go-chi/chi/v5
> go get github.com/jackc/pgx/v5/stdlib
> go get github.com/jackc/pgx/v5/pgconn
> go get -tags 'postgres' github.com/golang-migrate/migrate/v4
> go get github.com/golang-migrate/migrate/v4/database/postgres
> go get github.com/golang-migrate/migrate/v4/source/file
> go get github.com/joho/godotenv
> go get github.com/caarlos0/env/v11
> go get github.com/go-playground/validator/v10
> go get golang.org/x/crypto/bcrypt
> go get github.com/golang-jwt/jwt/v5
>
> go test -v -race ./internal/notes/
> ```

---

## Warm-up

- [ ] Tests pass. The output should list 6 PASS lines (one per `TestService_*`) plus subtests under `Get`.
- [ ] Run `go test -cover ./internal/notes/` — note the percentage.
- [ ] `go test -coverprofile=cover.out ./internal/notes/ && go tool cover -html=cover.out` — open the HTML and read which lines are red. The Postgres repo SQL methods are all red (we don't unit-test them; Day 24 does).

---

## Task 1 — Break the code, watch a test fail

Make sure the tests actually mean something.

- [ ] In [internal/notes/service.go](internal/notes/service.go), change the empty-PATCH check from:
  ```go
  if in.Title == nil && in.Body == nil {
  ```
  to:
  ```go
  if in.Title == nil { // forgot Body
  ```
- [ ] Re-run `go test ./internal/notes/`. `TestService_Patch_NothingToUpdate` should FAIL — the service now lets `{body: nil, title: nil}` through (because Body alone-nil doesn't trip the condition).
- [ ] Revert. Watch the test pass.

**Why:** a green test you can't break tells you nothing. This proves the tests actually exercise the rule.

---

## Task 2 — Add a test for `Update` propagating ErrNotFound

`TestService_Get` covers the not-found path; `Update` doesn't yet.

- [ ] Add `TestService_Update_NotFound`:
  ```go
  func TestService_Update_NotFound(t *testing.T) {
      repo := &fakeRepository{UpdateErr: ErrNotFound}
      svc := NewService(repo)
      _, err := svc.Update(context.Background(), 1, 42, UpdateRequest{Title: "x"})
      if !errors.Is(err, ErrNotFound) {
          t.Fatalf("want ErrNotFound, got %v", err)
      }
  }
  ```
- [ ] Run. Should pass.
- [ ] Now check coverage again — the `Update` line is green where it wasn't.

---

## Task 3 — `t.Helper()` for cleaner failures

Long tests grow utility funcs. Mark them so failure traces point at the test, not the helper.

- [ ] Add a helper in [service_test.go](internal/notes/service_test.go):
  ```go
  func assertErrIs(t *testing.T, got, want error) {
      t.Helper()  // <- tells `go test` to skip this frame in the failure stack
      if !errors.Is(got, want) {
          t.Fatalf("want err %v, got %v", want, got)
      }
  }
  ```
- [ ] Replace the manual `if !errors.Is(...) { t.Fatalf(...) }` blocks in a couple of tests with `assertErrIs(t, err, ErrNotFound)`.
- [ ] Without `t.Helper()`, a failed `assertErrIs` would point at the helper's line. With it, you see the test's call site. Tiny but high-quality-of-life.

---

## Task 4 — A subtest with `t.Parallel()`

If your tests don't share mutable state, run them concurrently for speed.

- [ ] Inside each `t.Run(tc.name, ...)` in `TestService_Get`, add `t.Parallel()` at the top:
  ```go
  t.Run(tc.name, func(t *testing.T) {
      t.Parallel()
      // ...
  })
  ```
- [ ] **Watch out:** the loop variable `tc` needs to be captured locally — Go 1.22+ fixes this for `for` loops (each iteration has its own `tc`), but if you write `for _, tc := range tests` on Go 1.21 or older, do:
  ```go
  for _, tc := range tests {
      tc := tc  // shadow
      t.Run(...)
  }
  ```
- [ ] Run again. Tests should still pass; you'll see them interleave in `-v` output.

**Why:** when a package has 200 service tests, parallelism takes the suite from 10s to 0.5s. Worth knowing.

---

## Task 5 — A "constructor returns a valid object" test for `NewService`

A sanity check that's easy to skip.

- [ ] Add:
  ```go
  func TestNewService(t *testing.T) {
      repo := &fakeRepository{}
      svc := NewService(repo)
      if svc == nil {
          t.Fatal("NewService returned nil")
      }
      // touch a method to ensure the field is wired
      _, _ = svc.List(context.Background(), 1, ListFilter{})
      if repo.ListCalls != 1 {
          t.Errorf("svc.List did not delegate to repo")
      }
  }
  ```
- [ ] Trivial test, but if someone refactors `NewService` and forgets to assign `repo`, this catches it. **Trivial tests catch trivial mistakes.**

---

## Task 6 — Compare to `testify` (don't ship it)

`github.com/stretchr/testify` is the most-downloaded Go test library.

- [ ] In a scratch file (`scratch_test.go`), rewrite `TestService_Patch_NothingToUpdate` using testify's `assert`:
  ```go
  import "github.com/stretchr/testify/assert"
  
  func TestPatchNothingToUpdate_Testify(t *testing.T) {
      svc := NewService(&fakeRepository{})
      _, err := svc.Patch(context.Background(), 1, 1, PatchRequest{})
      assert.ErrorIs(t, err, ErrNothingToUpdate)
  }
  ```
- [ ] Run both. Compare:
  - Stdlib: more lines per check, but no dep.
  - testify: terser, requires the lib.
- [ ] **Decide which you'd ship**, write 2 lines in "What I learned". For a public OSS library, stdlib is the conservative choice (one less dep). For team apps, testify often wins on readability.

---

## Stretch — only if you're flying

- [ ] Read the `testing` package doc — it's only ~200 lines: <https://pkg.go.dev/testing>. The `B` (benchmark) and `F` (fuzz) tests are interesting tangents.
- [ ] Try a fuzz test on `parseListFilter` from `handler.go`:
  ```go
  func FuzzParseLimit(f *testing.F) {
      f.Add("10")
      f.Fuzz(func(t *testing.T, s string) {
          // construct a request, call parseListFilter, ensure no panic
      })
  }
  ```
  Run with `go test -fuzz=. -fuzztime=10s`.
- [ ] Sketch what `service_test.go` would look like for the **auth service**. The fake replaces `UserRepository` AND `RefreshTokenRepository` — a whole-system test gets messy. That's a good reason to keep the auth service small and split its concerns; the refresh-rotation logic is its own beast and might want a dedicated `RefreshFlow` type. (Don't refactor today; just notice.)

---

## What I learned (fill at end of day)

> 3–5 bullets in your own words.

-
-
-
-
-
