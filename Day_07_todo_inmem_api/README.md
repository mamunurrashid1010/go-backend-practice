# Day 7 — In-Memory To-Do REST API (Week 1 mini-project)

> **Goal:** assemble Days 1–6 into one cohesive REST API for managing to-dos. Same engine, real product surface.

This is not a *teaching* day — it's a **consolidation** day. Every concept from Week 1 plugs in here. If something doesn't click, that's the gap to study, not a new thing to learn.

---

## What it does

A minimal To-Do API. CRUD + listing with a filter + a health check.

| Method | Path                         | Purpose                                                   | Status codes |
| ------ | ---------------------------- | --------------------------------------------------------- | ------------ |
| GET    | `/healthz`                   | Liveness check                                            | 200          |
| GET    | `/todos`                     | List todos; optional `?done=true&search=...&limit=10`     | 200 |
| POST   | `/todos`                     | Create a todo                                             | 201, 400 |
| GET    | `/todos/{id}`                | Fetch one todo                                            | 200, 400, 404 |
| PUT    | `/todos/{id}`                | Full replace (`title`, `done` both required)              | 200, 400, 404 |
| PATCH  | `/todos/{id}`                | Partial update with pointer-field DTO                     | 200, 400, 404 |
| DELETE | `/todos/{id}`                | Delete                                                    | 204, 400, 404 |

Every error response uses the same envelope from Day 4:

```json
{ "error": { "code": "NOT_FOUND", "message": "todo not found" } }
```

Every request gets a `X-Request-ID` (forwarded if the client sent one). Every panic gets logged and returns a clean JSON 500.

---

## What it shows from Week 1

| Concept             | Where in this project                                                            |
| ------------------- | -------------------------------------------------------------------------------- |
| `net/http` server   | [main.go](main.go) — `http.Server` with explicit timeouts                        |
| JSON encode/decode  | [internal/todo/handler.go](internal/todo/handler.go) — strict-decode pipeline    |
| Struct tags + DTOs  | [internal/todo/todo.go](internal/todo/todo.go) — `CreateRequest`, `PatchRequest` |
| Error helpers       | [internal/respond](internal/respond/respond.go) — copied verbatim from Day 4     |
| chi router + groups | `Handler.Router()` in [handler.go](internal/todo/handler.go) — `r.Route`         |
| Middleware stack    | `Recover → RequestID → Logger` in [main.go](main.go)                             |
| `sync.Mutex` store  | [internal/todo/store.go](internal/todo/store.go) — concurrent-safe map           |
| Pointer-field PATCH | `PatchRequest` in [todo.go](internal/todo/todo.go)                               |

The codebase is now multi-package and starts to look like the shape Day 11 (handler/service/repository) will formalise — without forcing the layering yet.

---

## Run it

```powershell
go mod init day07
go get github.com/go-chi/chi/v5
go run .
```

```powershell
# liveness
curl.exe http://localhost:8080/healthz

# create
curl.exe -i -H "Content-Type: application/json" `
  -d '{"title":"learn Go"}' http://localhost:8080/todos

# list (open + filter)
curl.exe http://localhost:8080/todos
curl.exe "http://localhost:8080/todos?done=false&search=learn"

# fetch one
curl.exe http://localhost:8080/todos/1

# mark done with PATCH (pointer-field DTO)
curl.exe -i -X PATCH -H "Content-Type: application/json" `
  -d '{"done":true}' http://localhost:8080/todos/1

# replace whole todo with PUT
curl.exe -i -X PUT -H "Content-Type: application/json" `
  -d '{"title":"new title","done":false}' http://localhost:8080/todos/1

# delete (204)
curl.exe -i -X DELETE http://localhost:8080/todos/1
```

Watch your server terminal for a log line per request:

```
GET    /todos    200  1.4ms rid=4b8c91a2 size=128
POST   /todos    201  0.9ms rid=fe17a330 size=92
PATCH  /todos/1  200  0.6ms rid=20d3bb14 size=98
```

---

## Project layout

```
Day_07_todo_inmem_api/
├── README.md
├── TASKS.md
├── main.go                       ← wiring only (server, middleware, mount)
├── internal/
│   ├── middleware/middleware.go  ← Recover + RequestID + Logger (from Day 6)
│   ├── respond/respond.go         ← consistent JSON envelope (from Day 4)
│   └── todo/
│       ├── todo.go                ← types + DTOs
│       ├── store.go               ← in-memory sync.Mutex map
│       └── handler.go             ← HTTP handlers + Router()
```

The `internal/todo` package follows the simplest-thing-that-works split that already hints at:

- **types** (`todo.go`) — what a Todo is, what its DTOs look like
- **store** (`store.go`) — how it's persisted
- **handler** (`handler.go`) — how it's served over HTTP

Day 11 will introduce a third layer (**service**) between handler and store. Today we keep it two layers — already enough structure for the project to feel real.

---

## What to look for as a reader

Open files in this order:

1. [main.go](main.go) — see how short the wiring is. The router, the middleware chain, and `Handler.Router()` mounted at `/todos` are basically all there is.
2. [internal/todo/todo.go](internal/todo/todo.go) — domain model + request DTOs. Notice the **pointer fields** on `PatchRequest`.
3. [internal/todo/store.go](internal/todo/store.go) — every method takes the mutex with `defer Unlock`.
4. [internal/todo/handler.go](internal/todo/handler.go) — the longest file. Look at how every handler is 5–10 lines because the helpers do the heavy lifting.

If you find yourself thinking "this is mostly copy-paste from Day 6"… good. **That means Week 1 worked.**

---

## What's next

**Day 8** moves to Postgres — the in-memory `Store` becomes a real database, but the `Handler` and `Router` don't have to change. That's the lesson the layering will pay off for.

Stretch: [TASKS.md](TASKS.md) has ~8 extensions you can try before moving on — pagination, soft-delete, query-string validation, integration tests, the works.
