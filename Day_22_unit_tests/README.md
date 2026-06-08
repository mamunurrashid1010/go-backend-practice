# Day 22 — Unit Testing the Service Layer

> **Goal:** prove the Notes service does what it claims — without a database, without HTTP, without external dependencies. Use Go's stdlib `testing` package + table-driven tests + a fake repository that implements the `Repository` interface in a `_test.go` file.

This kicks off Week 4 (Quality). The Day 11 layering paid for itself when Day 12 swapped the storage; today it pays off again when we mock the storage for tests.

---

## 1. Why the service layer first?

A service test is the cheapest, fastest, highest-value test you can write:

- **No DB, no HTTP** — runs in microseconds. You can run thousands per second.
- **The service is where the business rules live** — empty-PATCH detection, error mapping, future "title can't contain forbidden words", etc.
- **It catches regressions early** — break a rule and a test fails before you ever boot the server.

The handler layer is mostly plumbing (decode → call service → encode). Tested Day 23.
The repository against real Postgres is real but slow. Tested Day 24 with `testcontainers`.
The service is the sweet spot.

---

## 2. Go's testing model in 60 seconds

- A test file lives next to the code it tests and ends in `_test.go`.
- It declares `package <same_as_code>` (white-box testing — access to unexported names).
- Each test is `func TestX(t *testing.T)`.
- `t.Run("name", func(t *testing.T){ ... })` declares a **subtest** — useful for table-driven cases.
- `t.Errorf` marks the test failed but keeps running. `t.Fatalf` marks it failed and stops.
- No `assert` keyword — you write `if got != want { t.Errorf(...) }`. **No magic, no panic. Boring on purpose.**

Run them:

```powershell
go test ./...                 # everything
go test ./internal/notes/...  # one package
go test -v ./internal/notes/  # show each test
go test -run TestService_Get  # match by name
go test -race ./...           # data race detector
go test -cover ./...          # coverage %
```

---

## 3. The fake repository

The notes service depends on the `Repository` **interface** (not the Postgres implementation). That's the whole point of Day 11's layering. In a test, we provide our own implementation that returns whatever we tell it to.

```go
// fakeRepository implements Repository for tests. Each field is what the
// matching method will return; the *Calls counters and Last* fields let
// the test assert that the service actually called through.
type fakeRepository struct {
    // Returns
    ListNotes  []Note
    ListErr    error
    GetNote    Note
    GetErr     error
    CreateNote Note
    CreateErr  error
    UpdateNote Note
    UpdateErr  error
    PatchNote  Note
    PatchErr   error
    DeleteErr  error

    // Captured args + counters
    ListCalls, GetCalls, CreateCalls, UpdateCalls, PatchCalls, DeleteCalls int
    LastGetUserID, LastGetID                                               int64
    LastCreateUserID                                                       int64
    LastCreateIn                                                           CreateRequest
    // ...
}

func (r *fakeRepository) Get(_ context.Context, userID, id int64) (Note, error) {
    r.GetCalls++
    r.LastGetUserID, r.LastGetID = userID, id
    return r.GetNote, r.GetErr
}
// ... five more methods to satisfy the interface ...
```

`go test` won't compile if the fake doesn't satisfy the interface — that's a free check.

> No `testify`, no `gomock`, no `mockery`. Just a struct with fields. **The minimum thing that works** is also the easiest thing to read in 6 months when a test fails. Day 22's stretch tasks let you compare to those libraries.

---

## 4. Table-driven tests — the Go idiom

When a function has 5 distinct cases, you don't write 5 test functions. You write **one** with a table of cases:

```go
func TestService_Get(t *testing.T) {
    tests := []struct {
        name     string

        // fake repo setup
        repoNote Note
        repoErr  error

        // expectations
        wantErr  error
        wantTitle string
    }{
        {
            name:      "found",
            repoNote:  Note{ID: 1, Title: "hello"},
            wantTitle: "hello",
        },
        {
            name:    "not found is propagated",
            repoErr: ErrNotFound,
            wantErr: ErrNotFound,
        },
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            svc := NewService(&fakeRepository{GetNote: tc.repoNote, GetErr: tc.repoErr})
            got, err := svc.Get(context.Background(), 42, 1)

            if tc.wantErr != nil {
                if !errors.Is(err, tc.wantErr) {
                    t.Fatalf("want err %v, got %v", tc.wantErr, err)
                }
                return
            }
            if err != nil {
                t.Fatalf("unexpected err: %v", err)
            }
            if got.Title != tc.wantTitle {
                t.Errorf("title: want %q, got %q", tc.wantTitle, got.Title)
            }
        })
    }
}
```

Three things this teaches:

- **One row = one case.** Add a row, not a function.
- **`t.Run(name, ...)`** — each row gets its own subtest. `go test -v` prints them; `go test -run TestService_Get/found` runs only that one.
- **Error assertions use `errors.Is`**, not `==`. Wrapped errors (the `%w` chain from Day 16) still match.

---

## 5. The "AAA" pattern

Every test follows the same shape:

```go
// Arrange — set up the world
repo := &fakeRepository{GetErr: ErrNotFound}
svc := NewService(repo)

// Act — call the thing under test
_, err := svc.Get(context.Background(), 1, 99)

// Assert — check what happened
if !errors.Is(err, ErrNotFound) {
    t.Fatalf("want ErrNotFound, got %v", err)
}
if repo.GetCalls != 1 {
    t.Errorf("repo.Get should be called once, got %d", repo.GetCalls)
}
```

Three sections, in order, no nesting. When this gets tangled, your test is doing too much.

---

## 6. What's in this folder

| Layer | Tested how |
| --- | --- |
| **notes service** | **TODAY** — [internal/notes/service_test.go](internal/notes/service_test.go) with a fake repo |
| notes handler | Day 23 — `net/http/httptest` |
| notes Postgres repository | Day 24 — `testcontainers-go` |
| auth service | a stretch task today |

The application code is **carried forward unchanged** from Day 21 — the only new file you'll write is `service_test.go`. That's the lesson: tests get added without changing the code being tested.

---

## 7. Coverage — useful, not a goal

```powershell
go test -cover ./internal/notes/
# coverage: 78.4% of statements
```

For a percent-and-a-link:

```powershell
go test -coverprofile=cover.out ./internal/notes/
go tool cover -html=cover.out
# opens a browser; green = covered, red = not
```

Treat coverage as a **discovery tool**, not a KPI. Reading the highlighted file tells you "ah, I never test the error path on Update" much faster than browsing.

100% coverage is a trap — chasing it makes you write tests for trivial code. **80% with intent beats 100% with ceremony.**

---

## 8. The race detector

```powershell
go test -race ./...
```

Instruments the binary to detect concurrent unsynchronised access. If two goroutines touch the same map without a mutex, the detector prints a stack trace pointing at both. Catches the bug Day 3 explained when you first saw `sync.Mutex`.

Always run with `-race` in CI. It's 5-10× slower; that's fine — it's not running in prod.

---

## 9. Run today's tests

```powershell
cd Day_22_unit_tests
go mod init day22
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

# the actual test command
go test -v -race ./internal/notes/
```

You should see:

```
=== RUN   TestService_Patch_NothingToUpdate
--- PASS: TestService_Patch_NothingToUpdate (0.00s)
=== RUN   TestService_Get
=== RUN   TestService_Get/found
=== RUN   TestService_Get/not_found_is_propagated
=== RUN   TestService_Get/wrapped_not_found_still_matches
--- PASS: TestService_Get (0.00s)
... etc
PASS
ok  	day22/internal/notes	0.012s
```

Microseconds. No DB. No HTTP. That's the win.

---

## 10. What's next

**Day 23** — handler tests with `net/http/httptest.NewRecorder()`. Plumb a fake service in, make an HTTP request, assert the status + body.

**Day 24** — integration tests with `testcontainers-go`. Spin up a real Postgres in a container per test run, hit the actual repository code, prove the SQL is right.

After Week 4, your project doesn't just work — you can prove it.
