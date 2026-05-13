# Day 4 — Error Handling Patterns + `respondJSON` / `respondError`

> **Goal:** stop duplicating "set Content-Type, set status, encode JSON" in every handler. Build small, sharp helpers that make every response in the API look the same — successes *and* errors.

---

## 1. Why this day matters

In Day 3, every handler ended with the same incantation:

```go
w.Header().Set("Content-Type", "application/json; charset=utf-8")
w.WriteHeader(status)
json.NewEncoder(w).Encode(v)
```

…and error responses were a mix of `http.Error` (plain text), JSON envelopes, and one-off shapes. That's the seed of every messy API. A client integrating with you should be able to point at one type and parse *every* error your server can produce.

Today we fix that by:

1. Moving `respondJSON` / `respondError` into a reusable package: `internal/respond`.
2. Picking **one** consistent error envelope and sticking to it.
3. Adding convenience helpers (`respond.BadRequest`, `respond.NotFound`, …) so handlers read like English.
4. Setting up centralised 404/405 + a panic-recovery middleware so the API *can't* leak HTML or stack traces.

---

## 2. What an error response should look like

There's no single right answer, but the **rules of thumb** are non-negotiable:

- **Status code matches the body.** A 200 with `{"error": "..."}` is a smell — clients check the status first.
- **Same shape every time.** Mixing `{"error":"..."}` here and `{"message":"..."}` there is the most common API design crime.
- **No leaked internals.** Stack traces, SQL errors, panic messages → server log only. The body says "internal server error" and an opaque ID the user can quote to support.
- **Stable, machine-readable code.** Humans read `"message"`; clients branch on `"code"`. Translating messages is harder than translating codes.

Three common shapes you'll see in the wild:

| Shape | Example | Pros / cons |
| --- | --- | --- |
| **Simple** | `{"error": "user not found"}` | Easiest. No machine codes — clients have to string-match. Fine for personal projects. |
| **Structured (this day's pick)** | `{"error": {"code": "USER_NOT_FOUND", "message": "user not found"}}` | Sweet spot. Adds a code without much ceremony. |
| **RFC 7807 Problem Details** | `{"type": "...", "title": "...", "status": 404, "detail": "...", "instance": "..."}` | Standardised, verbose. Worth knowing; use it on public APIs. |

We'll use the **structured** shape today, and Task 5 has you migrate to RFC 7807 to feel the difference.

---

## 3. The `respond` package

```
Day_04_error_handling_helpers/
├── go.mod
├── main.go
└── internal/
    └── respond/
        └── respond.go
```

The package exposes a tiny surface:

```go
respond.JSON(w, http.StatusOK, user)               // success
respond.NoContent(w)                                // 204, no body
respond.BadRequest(w, "name is required")           // 400
respond.NotFound(w, "user not found")               // 404
respond.Conflict(w, "email already taken")          // 409
respond.Unauthorized(w, "missing token")            // 401
respond.Internal(w, err)                            // 500 — logs err, returns opaque body
respond.Error(w, http.StatusTeapot, "TEAPOT", "I'm a teapot")  // any custom case
```

Why this matters: every handler now reads like a sentence about the *domain*, not about HTTP plumbing.

```go
u, ok := store.get(id)
if !ok {
    respond.NotFound(w, "user not found")
    return
}
respond.JSON(w, http.StatusOK, u)
```

> **Why `internal/`?** A directory named `internal` is a Go convention — anything inside can only be imported by code in the same module. This makes "this is my package, not yours" enforced by the compiler. Use it for everything you don't want random external code depending on.

---

## 4. The "internal vs transport error" split

Day 3's `decodeErrorMessage` introduced this idea: take an internal error type and turn it into a friendly client message. Day 16 in the plan goes deep on this; today we plant the seed.

The mental model:

```
[ handler ]
    │ calls
    ▼
[ service / store ]  →  returns a domain error (e.g. ErrNotFound)
    │
    ▼
[ handler ]  ←  translates domain error → HTTP response via respond.*
```

The handler is the **only** place that knows about HTTP. The service knows about users, notes, orders. That separation is what makes services testable later (Day 22 mocks them with interfaces).

For today, we'll keep the translation inline; Day 16 introduces typed errors (`var ErrNotFound = errors.New("not found")`) and a dedicated mapper.

---

## 5. Centralised 404 and 405

`http.ServeMux` returns plain-text `404 page not found` by default. That breaks the "all errors are JSON" promise. Override it:

```go
mux := http.NewServeMux()

// ...routes...

// 404 catch-all: anything that didn't match any pattern.
mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
    respond.NotFound(w, "route not found: " + r.Method + " " + r.URL.Path)
})
```

Go 1.22's pattern-aware mux returns a JSON-friendly 405 for you when you use `"GET /users"` etc. But the generic `404` is yours to write.

---

## 6. Panic recovery — the safety net

Every long-lived server needs one piece of middleware that catches panics. Without it, an unhandled `nil` dereference in one handler kills the *entire* server. Go's HTTP server actually catches goroutine panics and aborts just that request, but the client gets a torn connection with no body — not what you want.

```go
func recoverPanic(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        defer func() {
            if rec := recover(); rec != nil {
                log.Printf("panic in handler: %v", rec)
                respond.Internal(w, fmt.Errorf("panic: %v", rec))
            }
        }()
        next.ServeHTTP(w, r)
    })
}
```

We wrap the mux in `recoverPanic`. Day 6 builds a small middleware stack; today's the first piece.

---

## 7. Don't leak. Ever.

The most common Go API security mistake:

```go
http.Error(w, err.Error(), http.StatusInternalServerError)
// → "sql: connection refused: dial tcp 10.0.0.5:5432: connect: ..."
```

That tells an attacker your database IP. Always:

1. **Log the real error** server-side (with a request ID; correlation comes Day 25).
2. **Respond with a generic message** + a status code.

`respond.Internal(w, err)` in our package does both.

---

## 8. How to run

```powershell
go mod init day04
go run .
```

```powershell
curl.exe -i http://localhost:8080/users/1            # JSON success
curl.exe -i http://localhost:8080/users/999          # JSON 404
curl.exe -i http://localhost:8080/users/abc          # JSON 400
curl.exe -i http://localhost:8080/banana             # JSON 404 (catch-all)
curl.exe -i -X PUT http://localhost:8080/users/1     # JSON 405 (method check)
curl.exe -i http://localhost:8080/panic              # demonstrates recovery middleware
```

Notice: **every** response is JSON, **every** error has the same shape, **nothing** leaks internals.

---

## 9. What's next

**Day 5** introduces `chi` — a real router with `Get`/`Post`/`Put` per route, route groups, and middleware as a first-class feature. Today's `respond` package and the recovery middleware come along unchanged.
