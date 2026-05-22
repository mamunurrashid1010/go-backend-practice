# Day 11 — Practice Tasks

These tasks make the layering pay off. Most are small; one is genuinely valuable (Task 5 — the mock repository).

> **Before you start:**
>
> ```powershell
> go mod init day11
> go get github.com/go-chi/chi/v5
> go run .
> ```

---

## Warm-up — prove the API didn't change

- [ ] Curl every endpoint with the same commands you used on Day 7. Body, status, headers should match exactly.
  ```powershell
  curl.exe -i http://localhost:8080/todos
  curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"x\"}" http://localhost:8080/todos
  curl.exe -i -X PATCH -H "Content-Type: application/json" -d "{\"done\":true}" http://localhost:8080/todos/1
  curl.exe -i -X DELETE http://localhost:8080/todos/1
  ```
- [ ] Hit `POST /todos` with `{"title":""}` — should be `400 BAD_REQUEST` with message `"title is required"`. The error came from the **service** (not the handler), and the handler mapped it to 400.
- [ ] Hit `PATCH /todos/1` with `{}` — `400 BAD_REQUEST` `"nothing to update"`.
- [ ] Hit `GET /todos/999` — `404 NOT_FOUND` `"todo not found"`.

---

## Task 1 — Read the wiring code carefully

- [ ] Open [main.go](main.go). It's 60 lines. Identify which lines are:
  - building the **repository**
  - building the **service**
  - building the **handler**
  - building the **router**
- [ ] Convince yourself: if you wanted to swap the in-memory repo for *anything else* that implements `todo.Repository`, you'd change **one line**.

This is the whole lesson.

---

## Task 2 — Move "title length" validation into the service

The plan: cap title length at 200 chars at the service layer.

- [ ] Add to [errors.go](internal/todo/errors.go):
  ```go
  var ErrTitleTooLong = errors.New("title too long (max 200)")
  ```
- [ ] In [service.go](internal/todo/service.go), check `len(in.Title) > 200` in `Create`, `Update`, and the `*in.Title` case of `Patch`. Return `ErrTitleTooLong`.
- [ ] Add a case to `writeServiceErr` in [handler.go](internal/todo/handler.go) mapping it to `400 BAD_REQUEST`.
- [ ] Test:
  ```powershell
  curl.exe -i -H "Content-Type: application/json" `
    -d "{\"title\":\"$('a' * 201)\"}" http://localhost:8080/todos
  # expect 400 BAD_REQUEST "title too long (max 200)"
  ```

**Why:** trains the muscle memory for "where does this rule belong?" Title length is a domain invariant, not an HTTP concern.

---

## Task 3 — Add a `MarkAllDone` service method

This is an operation that doesn't fit cleanly in one repository method — perfect for the service layer.

- [ ] Add to `Service`:
  ```go
  func (s *Service) MarkAllDone(ctx context.Context) (int, error) {
      // 1. list everything (done=false)
      // 2. for each, call Patch with done=true
      // 3. return count
  }
  ```
- [ ] Add a `POST /todos/done-all` route that calls it and returns `{"updated": <n>}`.
- [ ] Test it.

**Why:** the repository has narrow CRUD methods; the service can compose them into higher-level operations. (For the real Day 30 lesson: this is *exactly* where Postgres transactions matter. We'll do it right then.)

---

## Task 4 — Pull the validation up *into the handler*, then revert

This is a "feel why it doesn't belong here" exercise.

- [ ] Move the `Title == ""` check from `service.Create` into `handler.create` (right after `decodeJSON`).
- [ ] Test it works the same.
- [ ] Now imagine adding a CLI (`go run . add ""`). The CLI doesn't go through `handler.go` — it goes straight to the service. **The validation would be silently bypassed.**
- [ ] Revert. Write 2 lines in your "What I learned" section about why the service is the right home.

---

## Task 5 — A mock repository (preview of Day 22)

Make a tiny test-only repository that lets you control what `Get` returns.

- [ ] Create `internal/todo/mock_repository_test.go`:
  ```go
  package todo

  import "context"

  type mockRepository struct {
      // pre-seed maps of "what should each call return"
      getResult Todo
      getErr    error
  }

  func (m *mockRepository) List(_ context.Context, _ ListFilter) ([]Todo, error) { return nil, nil }
  func (m *mockRepository) Get(_ context.Context, _ int64) (Todo, error) {
      return m.getResult, m.getErr
  }
  func (m *mockRepository) Create(_ context.Context, _ CreateRequest) (Todo, error) { return Todo{}, nil }
  func (m *mockRepository) Update(_ context.Context, _ int64, _ UpdateRequest) (Todo, error) { return Todo{}, nil }
  func (m *mockRepository) Patch(_ context.Context, _ int64, _ PatchRequest) (Todo, error)   { return Todo{}, nil }
  func (m *mockRepository) Delete(_ context.Context, _ int64) error                          { return nil }
  ```
- [ ] Write a tiny test:
  ```go
  func TestServiceGet_NotFound(t *testing.T) {
      repo := &mockRepository{getErr: ErrNotFound}
      svc := NewService(repo)
      _, err := svc.Get(context.Background(), 42)
      if !errors.Is(err, ErrNotFound) {
          t.Fatalf("want ErrNotFound, got %v", err)
      }
  }
  ```
- [ ] `go test ./internal/todo/...` — should pass.

**Why:** this is the proof the layering pays for itself. Day 22 will use exactly this pattern with table-driven tests.

---

## Task 6 — Compare line counts to Day 7

- [ ] Day 7's handler.go vs Day 11's handler.go — count lines. (Day 11's should be shorter because validation moved out.)
- [ ] Day 11 added service.go. Is the total higher? Probably slightly. Now imagine adding 5 more domains (notes, projects, tags, ...) — does the layered version or the all-in-one version scale better?

This is a thinking task, not a coding one.

---

## Stretch — only if you're flying

- [ ] Sneak peek at Day 12: imagine writing `NewPostgresRepository(db *sql.DB) *PostgresRepository` that satisfies `Repository`. Sketch the `Create` method on paper — it's `INSERT ... RETURNING *` then `Scan`. Notice you wouldn't change *anything* else in the project.
- [ ] Add a `Service.Logger` field (a `*slog.Logger`) — but don't import slog yet, declare a tiny `Logger` interface inside the package. Pass it via constructor. Now both repo and service are testable without slog.
- [ ] Read the "Standard Go Project Layout" doc: <https://github.com/golang-standards/project-layout>. **Don't take it as gospel** — it's controversial — but skim the rationale for `internal/`.

