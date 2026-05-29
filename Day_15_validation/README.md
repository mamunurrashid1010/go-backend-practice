# Day 15 — Input Validation with `go-playground/validator`

> **Goal:** replace hand-rolled `if in.Title == ""` checks with declarative struct-tag validation, and return **field-level** error details so clients can fix every problem at once.

This starts Week 3 (Validation, errors, auth). We build on the Day 14 To-Do API.

---

## 1. The problem with hand-rolled validation

Day 14's service did this:

```go
func (s *Service) Create(ctx context.Context, in CreateRequest) (Todo, error) {
    if in.Title == "" {
        return Todo{}, ErrEmptyTitle
    }
    return s.repo.Create(ctx, in)
}
```

Fine for one field. But real DTOs have ten fields with rules like "required", "max 200 chars", "valid email", "one of low/medium/high", "between 1 and 5". Writing each by hand is:

- **Verbose** — 10 fields = 30 lines of `if`.
- **Inconsistent** — every dev writes the message differently.
- **One-at-a-time** — you return on the first failure, so the client fixes a field, resubmits, hits the next error, repeat. Bad UX.

A validation library fixes all three.

---

## 2. `go-playground/validator` — the de-facto standard

```powershell
go get github.com/go-playground/validator/v10
```

You annotate struct fields with `validate:"..."` tags, call `validator.Struct(x)`, and get back a list of every field that failed.

```go
type CreateRequest struct {
    Title string `json:"title" validate:"required,max=200"`
    Done  bool   `json:"done"`
}
```

`required,max=200` means "must be present AND at most 200 characters". The library checks both and reports each failure.

---

## 3. The tags you'll use constantly

| Tag | Meaning | Example field |
| --- | --- | --- |
| `required` | not the zero value | `validate:"required"` |
| `min=N` / `max=N` | string length / number range / slice length | `validate:"min=1,max=200"` |
| `len=N` | exact length | `validate:"len=6"` |
| `email` | valid email format | `validate:"required,email"` |
| `oneof=a b c` | one of a fixed set | `validate:"oneof=low medium high"` |
| `gte=N` / `lte=N` | >= / <= (numbers) | `validate:"gte=1,lte=5"` |
| `url` | valid URL | `validate:"url"` |
| `uuid` | valid UUID | `validate:"uuid"` |
| `alphanum` | letters + digits only | `validate:"alphanum"` |
| `omitempty` | skip remaining rules if empty/nil | `validate:"omitempty,min=1"` |
| `eqfield=X` | equals another field (password confirm) | `validate:"eqfield=Password"` |

`omitempty` is the one to understand for `PATCH`: a pointer field that's `nil` is skipped; if present, the remaining rules run on the dereferenced value.

```go
type PatchRequest struct {
    Title *string `json:"title,omitempty" validate:"omitempty,min=1,max=200"`
    Done  *bool   `json:"done,omitempty"`
}
```

"If `title` was sent, it must be 1–200 chars. If it wasn't sent, no rule applies."

---

## 4. The `internal/validate` package

[internal/validate/validate.go](internal/validate/validate.go) wraps the library so the rest of the app gets a clean API:

```go
fields := validate.Struct(in)   // nil if valid, []FieldError if not
```

Two things it does beyond the raw library:

**a) Uses the JSON field name, not the Go name.**

By default validator reports the Go field name (`Title`). Clients see JSON (`title`). We register a tag-name func so errors use the JSON name:

```go
v.RegisterTagNameFunc(func(fld reflect.StructField) string {
    name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
    if name == "-" {
        return ""
    }
    return name
})
```

**b) Translates validator's tags into human messages.**

Raw validator gives you `Tag() == "max"` and `Param() == "200"`. We map that to `"must be at most 200 characters"`. One `switch`, every endpoint benefits.

---

## 5. The field-level error response

The error envelope grows a `details` array (this was Day 4 Task 3 — now it's real):

```json
{
  "error": {
    "code": "VALIDATION",
    "message": "request validation failed",
    "details": [
      { "field": "title", "issue": "is required" },
      { "field": "priority", "issue": "must be one of: low medium high" }
    ]
  }
}
```

Status code: **`422 Unprocessable Entity`** — the request was well-formed JSON (that'd be `400`), but the *values* didn't pass validation. Some teams use `400` for both; `422` is the more precise choice and what we'll use.

[internal/respond/respond.go](internal/respond/respond.go) gains a `ValidationFailed(w, fields)` helper that emits exactly that shape.

---

## 6. Where validation lives — the layering question

A real question: should validation run in the **handler** or the **service**?

| | Handler | Service |
| --- | --- | --- |
| Catches bad input from | HTTP clients | HTTP + CLI + cron + tests |
| Knows about | HTTP / JSON | domain only |
| Returns | 422 with field details | domain errors |

**Today's choice:** the handler decodes JSON, then calls `validate.Struct(dto)` and returns `422` on failure — *before* the service is even called. The service keeps only genuine **business rules** that validator can't express (e.g. "nothing to update", and later "email already taken" which needs a DB lookup).

The key insight: `validate.Struct` is **not HTTP-coupled**. It works on any struct. If you add a CLI tomorrow, the CLI calls the same `validate.Struct` before invoking the service. Validation is "at the boundary", and the handler is just *today's* boundary.

> Format validation (shape, length, pattern) → `validator` at the boundary.
> Business validation (uniqueness, ownership, state) → service, often needs the DB.
> Day 16 makes the service's errors richer (typed, with fields).

---

## 7. What changed from Day 14

| File | Change |
| --- | --- |
| `internal/validate/validate.go` | **NEW** — wraps go-playground/validator |
| `internal/respond/respond.go` | added `ValidationFailed` + `details` in the envelope |
| `internal/todo/todo.go` | DTOs gained `validate:"..."` tags |
| `internal/todo/errors.go` | dropped `ErrEmptyTitle` (validator owns "required" now) |
| `internal/todo/service.go` | dropped the `Title == ""` checks; kept `ErrNothingToUpdate` |
| `internal/todo/handler.go` | each write handler calls `validate.Struct` after decode |

Everything else carries over unchanged.

---

## 8. Run it

```powershell
cd Day_15_validation
docker compose up -d
go mod init day15
go get github.com/go-chi/chi/v5
go get github.com/jackc/pgx/v5/stdlib
go get -tags 'postgres' github.com/golang-migrate/migrate/v4
go get github.com/golang-migrate/migrate/v4/database/postgres
go get github.com/golang-migrate/migrate/v4/source/file
go get github.com/joho/godotenv
go get github.com/caarlos0/env/v11
go get github.com/go-playground/validator/v10
go run .
```

Trigger validation errors:

```powershell
# empty title → 422 with field detail
curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"\"}" http://localhost:8080/todos

# title too long → 422 "must be at most 200 characters"
curl.exe -i -H "Content-Type: application/json" `
  -d "{\"title\":\"$('a' * 201)\"}" http://localhost:8080/todos

# valid → 201
curl.exe -i -H "Content-Type: application/json" -d "{\"title\":\"learn validation\"}" http://localhost:8080/todos
```

The 422 body:

```json
{"error":{"code":"VALIDATION","message":"request validation failed","details":[{"field":"title","issue":"is required"}]}}
```

---

## 9. What's next

**Day 16** — domain vs transport errors. Typed errors (`ErrNotFound`, `ErrConflict`) with extra fields, a single mapping function, error wrapping. The validator's field errors and the service's domain errors converge into one clean error story.

Then bcrypt (Day 17), JWT (Day 18), auth middleware (Day 19) — and the Week 3 Notes API where each user sees only their own notes.
