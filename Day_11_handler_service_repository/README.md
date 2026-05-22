# Day 11 — Handler / Service / Repository Layering

> **Goal:** take Day 7's To-Do API and split it into three layers with clean boundaries. **Same routes, same JSON, same behaviour** — only the internal structure changes. Done well, Day 12 (swap in Postgres) becomes a 30-minute task that touches one file.

---

## 1. Why three layers?

In Day 7, [internal/todo/handler.go](../Day_07_todo_inmem_api/internal/todo/handler.go) did three different jobs in one file:

1. **HTTP transport** — decode JSON, set status codes, write responses.
2. **Business rules** — "title is required", "nothing to update".
3. **Data access** — talk to the in-memory map.

Mixing them works for 200 lines. It rots fast. The classic split:

| Layer | Knows about | Doesn't know about |
| --- | --- | --- |
| **Handler** | HTTP, JSON, status codes | the DB, business invariants |
| **Service** | business rules, orchestration | HTTP, SQL |
| **Repository** | the data store (in-memory today, Postgres tomorrow) | HTTP, business rules |

```
HTTP request
   │
   ▼
┌──────────┐    decode JSON,         ┌─────────┐  validate inputs,  ┌─────────────┐
│ Handler  │ ───────────────────────▶│ Service │ ─────────────────▶│ Repository  │
│ (HTTP)   │     map domain          │ (rules) │   orchestrate     │ (storage)   │
│          │ ◀────────────────────── │         │ ◀──────────────── │             │
└──────────┘    errors → 4xx/5xx     └─────────┘                    └─────────────┘
   │
   ▼
HTTP response
```

Three concrete payoffs:

1. **Day 12 — swap the repository.** The handler/service don't know the data lives in Postgres. We change one file.
2. **Day 22/23 — unit tests.** The service is tested by mocking the repository. The handler is tested by using a fake service or the real service backed by an in-memory repo.
3. **Day 16 — typed domain errors.** The service returns `ErrNotFound`; the handler maps that to `404`. No SQL errors ever reach the HTTP layer.

---

## 2. The interface boundary

The trick that makes all of this work in Go is **dependency inversion via interfaces**:

```go
// in internal/todo/repository.go
type Repository interface {
    List(ctx context.Context, f ListFilter) ([]Todo, error)
    Get(ctx context.Context, id int64) (Todo, error)
    Create(ctx context.Context, in CreateRequest) (Todo, error)
    // ...
}
```

The `Service` depends on this **interface**, not on `InMemoryRepository` directly:

```go
type Service struct {
    repo Repository      // ← any type that satisfies the interface works
}
```

So you can hand it:
- `NewInMemoryRepository()` today
- `NewPostgresRepository(db)` tomorrow (Day 12)
- A test mock (Day 22)

The service code is identical in all three cases. That's the win.

> **Go's idiom:** *accept interfaces, return structs.* A consumer takes an interface so callers can swap implementations. A constructor returns the concrete type so callers see all its methods.

---

## 3. What goes where — the rules

A useful mental model when you write a new handler:

1. **Does the HTTP request have all the right fields?** → handler. Decode JSON, parse URL params.
2. **Is the request semantically valid?** → service. "Title can't be empty." "User can't delete someone else's note."
3. **How do I read/write this data?** → repository. SQL, mutex, file, network call.

Three rules of thumb:

- **The handler should never `import "database/sql"`.** If it does, leakage.
- **The service should never `import "net/http"`.** If it does, you can't reuse it from a CLI/cron/RPC.
- **The repository should never know about `http.ResponseWriter` or status codes.** It deals in domain types and domain errors.

Each layer talks to the next one via a small, named interface. Each layer's tests can stop one level deep.

---

## 4. Today's file layout

```
Day_11_handler_service_repository/
├── main.go                            ← wiring: build repo → service → handler → router
└── internal/
    ├── middleware/middleware.go        ← Recover + RequestID + Logger
    ├── respond/respond.go              ← consistent JSON envelope
    └── todo/
        ├── todo.go                     ← domain model + DTOs (Day 7 carry-over)
        ├── errors.go                   ← typed domain errors (preview of Day 16)
        ├── repository.go               ← Repository INTERFACE + InMemoryRepository struct
        ├── service.go                  ← Service struct, takes a Repository
        └── handler.go                  ← Handler struct, takes a *Service
```

Compare to Day 7:

| Day 7 | Day 11 |
| --- | --- |
| `todo.go` — types only | `todo.go` — types only (unchanged) |
| `store.go` — in-memory store | `repository.go` — interface + impl |
| `handler.go` — HTTP + validation + storage calls | `handler.go` — HTTP only |
|   | `service.go` — business rules (new) |
|   | `errors.go` — typed errors (new) |

The API behaviour, the routes, the JSON envelope — none of those change. **Hit the routes with the same curls and the responses are identical.** That's the proof the refactor is invisible from outside.

---

## 5. Typed errors — the glue that lets layers talk

When the repository says "no such todo", how does the handler know to return `404`?

**Wrong answer:** the repo returns an HTTP-friendly error like `"not found"` and the handler string-matches. Fragile.

**Right answer:** the package defines a sentinel error, and every layer agrees on its meaning.

```go
// internal/todo/errors.go
var (
    ErrNotFound        = errors.New("todo not found")
    ErrEmptyTitle      = errors.New("title is required")
    ErrNothingToUpdate = errors.New("nothing to update")
)
```

The repository wraps storage errors when relevant:

```go
return Todo{}, ErrNotFound       // direct return — caller uses errors.Is
// or
return Todo{}, fmt.Errorf("get id=%d: %w", id, ErrNotFound)  // wrapped for context
```

The handler maps to HTTP at the edge:

```go
switch {
case errors.Is(err, todo.ErrNotFound):
    respond.NotFound(w, err.Error())
case errors.Is(err, todo.ErrEmptyTitle), errors.Is(err, todo.ErrNothingToUpdate):
    respond.BadRequest(w, err.Error())
case err != nil:
    respond.Internal(w, err)        // anything else: 500, logged, opaque
}
```

This pattern scales: 20 endpoints, 5 error types, one mapping function. Day 16 makes it formal.

---

## 6. Dependency injection from `main.go`

`main.go` becomes the wiring file. It builds each layer from the inside out and hands the result down:

```go
func main() {
    repo := todo.NewInMemoryRepository()             // bottom layer
    svc  := todo.NewService(repo)                    // takes repo
    h    := &todo.Handler{Svc: svc}                  // takes service

    r := chi.NewRouter()
    r.Use(mw.Recover, mw.RequestID, mw.Logger)
    r.Mount("/todos", h.Router())
    // ...
}
```

If you ever read those four lines and think "this isn't very Go-magical" — that's the whole point. **No framework, no annotations, no DI container.** Just constructors. Easy to read, easy to test, easy to swap.

For Day 12, exactly one line changes:

```go
repo := todo.NewPostgresRepository(db)   // ← the only difference
```

That's the entire point of today's refactor.

---

## 7. Why not just one big "service"?

A common temptation: "why three layers? I'll put everything in one file."

Real reasons against:

- **Coupling.** Once `handler.go` knows about `database/sql`, you can't reuse its business logic from a CLI or a cron job.
- **Testing.** Mocking SQL in handler tests is painful. Mocking a `Repository` interface is one line.
- **Reasoning.** When a request 500s, you can read the layers like a stack trace: handler called service, service called repo, repo returned this error. Each file is small and obvious.
- **Reviewability.** A 5-line handler reviewed against a 50-line service is fast; a 200-line monolith is slow.

The cost is one new file and a tiny ceremony of constructors. The benefits compound for years.

---

## 8. Run it

```powershell
cd Day_11_handler_service_repository
go mod init day11
go get github.com/go-chi/chi/v5
go run .
```

```powershell
# Same endpoints, same responses as Day 7:
curl.exe -i http://localhost:8080/todos
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"title\":\"day 11\"}" http://localhost:8080/todos
curl.exe -i -X PATCH -H "Content-Type: application/json" `
  -d "{\"done\":true}" http://localhost:8080/todos/1
```

> If you swap any curl with Day 7's exact responses — body, status, headers — they match. That's the contract.

---

## 9. What's next

**Day 12** swaps the repository:

```go
- repo := todo.NewInMemoryRepository()
+ repo := todo.NewPostgresRepository(db)
```

That's the line. Day 11's discipline today is what makes Day 12 feel like a one-line change instead of a rewrite.

After that, Day 13 (config via env), Day 22 (unit tests with a mock repo), and Day 16 (typed errors formalized) all build on this exact skeleton.
