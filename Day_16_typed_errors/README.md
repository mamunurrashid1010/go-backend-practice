# Day 16 — Domain vs Transport Errors

> **Goal:** make errors a first-class part of the design. Sentinel errors for identity, typed errors for errors that carry data, `%w` wrapping for context, `errors.Is` / `errors.As` to inspect, and **one** function that maps domain errors → HTTP codes. Plus: translate Postgres driver errors into domain errors so SQL never leaks.

---

## 1. The three layers of an error

The same idea as Day 12, now made explicit:

```
┌─────────────────────────────────────────────────────────────┐
│  DRIVER errors      sql.ErrNoRows, *pgconn.PgError (23505)    │  ← never leave the repository
├─────────────────────────────────────────────────────────────┤
│  DOMAIN errors      ErrNotFound, *ConflictError               │  ← what service + handler speak
├─────────────────────────────────────────────────────────────┤
│  TRANSPORT          HTTP 404, 409, 422, 500                   │  ← what the wire speaks
└─────────────────────────────────────────────────────────────┘
```

- The **repository** catches driver errors and returns domain errors.
- The **service** passes domain errors up (and adds its own).
- The **handler** maps domain errors to HTTP status codes — in exactly one place.

A client should never see `sql: no rows in result set` or `ERROR: duplicate key value violates unique constraint`. Those are implementation details. They get a clean `404` or `409`.

---

## 2. Sentinel errors vs typed errors

Two kinds of error, two jobs.

### Sentinel error — identity only

```go
var ErrNotFound = errors.New("todo not found")
```

A package-level `var`. It carries **no data** — just "this specific thing happened". You check it with `errors.Is`:

```go
if errors.Is(err, ErrNotFound) { ... }
```

Use a sentinel when the *fact* of the error is all the caller needs. "Not found" is just "not found" — there's nothing extra to know.

### Typed error — carries data

```go
type ConflictError struct {
    Field string
    Value string
}

func (e *ConflictError) Error() string {
    return fmt.Sprintf("%s %q already exists", e.Field, e.Value)
}
```

A struct that implements `error`. It carries **fields** the handler can read. You check + extract it with `errors.As`:

```go
var conflict *ConflictError
if errors.As(err, &conflict) {
    // conflict.Field == "title", conflict.Value == "buy milk"
}
```

Use a typed error when the caller needs more than "it happened" — *which* field conflicted, *what* value, *what* limit was exceeded.

| | Sentinel | Typed |
| --- | --- | --- |
| Declared as | `var Err... = errors.New(...)` | `type ...Error struct{...}` |
| Carries data | no | yes |
| Checked with | `errors.Is` | `errors.As` |
| Example | `ErrNotFound` | `ConflictError{Field, Value}` |

---

## 3. Error wrapping with `%w`

When a low-level error bubbles up, you want to **add context without losing the original**. That's `%w`:

```go
// in the repository
return Todo{}, fmt.Errorf("get id=%d: %w", id, ErrNotFound)
```

This creates a new error whose message is `get id=42: todo not found`, but which **still satisfies `errors.Is(err, ErrNotFound)`**. The `%w` verb "wraps" the inner error so the chain is preserved.

```
fmt.Errorf("get id=42: %w", ErrNotFound)
        │
        ├─ message: "get id=42: todo not found"
        └─ wraps:  ErrNotFound   ← errors.Is can still find it
```

`%v` would format the error into the string but **break the chain** — `errors.Is` wouldn't find `ErrNotFound` anymore. **Use `%w` when you want callers to still be able to inspect the cause.** Use `%v` only when you're deliberately flattening (e.g. logging).

You can wrap multiple levels deep; `errors.Is` / `errors.As` walk the whole chain.

---

## 4. `errors.Is` vs `errors.As` — the mechanics

Both walk the wrapped chain. They answer different questions:

```go
// errors.Is — "is this (or anything it wraps) THIS sentinel?"
errors.Is(err, ErrNotFound)        // bool

// errors.As — "is there a *ConflictError anywhere in the chain?
//              if so, point my variable at it"
var conflict *ConflictError
errors.As(err, &conflict)          // bool; conflict now usable if true
```

- **`Is`** compares against a *value* (a sentinel). For "did X happen?"
- **`As`** matches by *type* and extracts. For "did an error of type T happen, and what's inside it?"

Rule of thumb: **sentinel → `Is`, typed → `As`.**

> Never compare errors with `==` once wrapping is in play: `err == ErrNotFound` fails if `err` is wrapped. Always `errors.Is`.

---

## 5. Translating driver errors (the repository's job)

Today we add a `UNIQUE` constraint on `todos.title` (artificial — real todos can repeat — but it's the cleanest way to demonstrate conflicts before we have users on Day 21).

When you insert a duplicate, Postgres returns a `*pgconn.PgError` with SQLSTATE `23505` (unique violation). The repository catches it and returns a domain `*ConflictError`:

```go
import "github.com/jackc/pgx/v5/pgconn"

_, err := r.db.QueryRowContext(ctx, q, ...).Scan(...)
if err != nil {
    var pgErr *pgconn.PgError
    if errors.As(err, &pgErr) && pgErr.Code == "23505" {
        return Todo{}, &ConflictError{Field: "title", Value: in.Title}
    }
    return Todo{}, fmt.Errorf("create: %w", err)   // anything else: wrapped, becomes 500
}
```

Now:
- The handler never imports `pgconn`.
- The client gets `409 CONFLICT`, not a raw Postgres error.
- The exact constraint code (`23505`) is knowledge that lives in **one place** — the repository.

The common SQLSTATE codes worth knowing:

| Code | Meaning |
| --- | --- |
| `23505` | unique violation |
| `23503` | foreign-key violation |
| `23502` | not-null violation |
| `23514` | check-constraint violation |

---

## 6. The one mapping function

The handler's `writeServiceErr` is the **single** place that turns domain errors into HTTP responses. Day 16 grows it to use both `Is` and `As`:

```go
func writeServiceErr(w http.ResponseWriter, err error) {
    var conflict *ConflictError
    switch {
    case errors.Is(err, ErrNotFound):
        respond.NotFound(w, err.Error())
    case errors.Is(err, ErrNothingToUpdate):
        respond.BadRequest(w, err.Error())
    case errors.As(err, &conflict):
        respond.Conflict(w, conflict.Error())     // 409, message from the typed error
    default:
        respond.Internal(w, err)                  // 500, logged, opaque to client
    }
}
```

Add a new error type? Add one `case`. Everything routes through here.

The `default` case is the safety net: any error you didn't explicitly handle becomes a logged-internally, opaque `500`. You never accidentally leak an unmapped error's message to the client.

---

## 7. Log the cause, return the clean version

Wrapping pays off in logs. The repository wraps with context (`get id=42: ...`), so when `respond.Internal` logs the error, you see the full chain:

```
internal error: create: ERROR: duplicate key ... (SQLSTATE 23505)
```

…while the **client** gets:

```json
{"error":{"code":"INTERNAL","message":"internal server error"}}
```

Rich for you, opaque for them. That's the whole point of the domain/transport split.

---

## 8. What changed from Day 15

| File | Change |
| --- | --- |
| `internal/todo/errors.go` | added `ConflictError` typed error |
| `internal/todo/postgres_repository.go` | translate `23505` → `*ConflictError` in Create + Update |
| `internal/todo/repository.go` | in-memory repo detects duplicate titles → `*ConflictError` (parity) |
| `internal/todo/handler.go` | `writeServiceErr` gained an `errors.As(&conflict)` case → 409 |
| `migrations/000002_unique_todo_title.*` | the `UNIQUE` constraint that makes conflicts possible |

Service, config, middleware, respond, validate — unchanged.

---

## 9. Run it

```powershell
cd Day_16_typed_errors
docker compose up -d
go mod init day16
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get github.com/jackc/pgx/v5/pgconn
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go run .
```

Demonstrate the conflict path:

```powershell
curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"unique me\"}" http://localhost:8080/todos
# 201 Created

curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"unique me\"}" http://localhost:8080/todos
# 409 CONFLICT  {"error":{"code":"CONFLICT","message":"title \"unique me\" already exists"}}
```

Check your server log on the 409 — you'll see the wrapped chain. The client only sees the clean message.

---

## 10. What's next

**Day 17** — bcrypt + `POST /auth/register` and `POST /auth/login`. The `ConflictError` you built today is exactly what "email already registered" returns. The `RegisterRequest` DTO you sketched in Day 15 Task 3 plugs in. Then JWT (Day 18), auth middleware (Day 19), refresh tokens (Day 20), and the Week 3 Notes API.
