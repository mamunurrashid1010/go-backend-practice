# Day 6 — Middleware: Logging, Recovery, Request ID

> **Goal:** understand what middleware is, write your own logging + recovery + request-ID middleware, then swap them for `chi/middleware`'s versions and see why both choices are valid.

---

## 1. What is middleware?

Middleware is **a function that wraps an `http.Handler` and returns a new `http.Handler`**. That's the whole concept.

```go
func mw(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // ... do something BEFORE the next handler runs ...
        next.ServeHTTP(w, r)
        // ... do something AFTER the next handler runs ...
    })
}
```

Picture it as an onion: each request peels through every middleware on the way in, hits the handler at the centre, and unwinds back out.

```
client → [logger → requestID → recover → handler] → client
              ↓        ↓          ↓         ↓
            before   before    before    work
            after    after     after
```

Two things make this idea so powerful:

1. **Composability.** You can stack arbitrarily many. `mw1(mw2(mw3(handler)))` is a valid handler.
2. **Cross-cutting concerns.** Logging, auth, tracing, panic recovery, timeouts, CORS — none of these belong inside the business handler, and middleware lets you write them once.

You already wrote one in Day 4 (`recoverPanic`) and Day 5 (the API-key check). Today we make it deliberate.

---

## 2. The signature you'll see everywhere

```go
type Middleware func(http.Handler) http.Handler
```

If a library says "middleware" and the function doesn't fit this shape, it's probably a framework with a custom signature (Gin, Echo) — fine, just different. Stdlib + chi stick to `func(http.Handler) http.Handler`.

---

## 3. Order matters

`r.Use(a, b, c)` in chi means a request flows **a → b → c → handler → c → b → a**. Some implications:

- **Recovery must be near the outside** so it can catch panics from anything below it (including other middleware).
- **Request ID must be early** so logging, tracing, and error responses can see it.
- **Logging is usually after Request ID** so the log line carries the ID.
- **Auth goes after logging** so unauthorised attempts still show up in the log.
- **CORS is usually outermost** because it has to handle `OPTIONS` preflight before anything else.

A common stack:

```
Recover  →  RequestID  →  Logger  →  CORS  →  Auth  →  handler
```

---

## 4. The three you'll build today

### 4a. RequestID

Assigns every request a short unique ID. Reads `X-Request-ID` from the client if present (so you can trace cross-service), otherwise generates one. Stashes it on the request `context.Context` so handlers and other middleware can read it, and echoes it back in `X-Request-ID`.

```go
ctx := context.WithValue(r.Context(), requestIDKey{}, id)
next.ServeHTTP(w, r.WithContext(ctx))
```

A correlation ID is the single highest-ROI thing you can add to an API. When a user reports "your API broke at 3:14pm", you find their exact request in milliseconds.

### 4b. Logger

Logs `method path status duration` on every request. Looks simple — but **capturing the status code is the hard part**, because `http.ResponseWriter` doesn't expose it. The trick: wrap the writer so we can record `WriteHeader`'s argument.

```go
type loggingRW struct {
    http.ResponseWriter
    status int
}
func (l *loggingRW) WriteHeader(code int) {
    l.status = code
    l.ResponseWriter.WriteHeader(code)
}
```

You'll see this exact wrapper pattern in every Go middleware library. It's also how you'd capture response size, latency at first byte, etc.

### 4c. Recover

You wrote this in Day 4. Today we move it into the middleware package alongside the others. Bonus: include the **stack trace** in the log via `runtime/debug.Stack()` — invaluable for prod incidents.

---

## 5. `context.Context` — read this if you haven't yet

We use `context.Context` to carry the request ID across middleware and into handlers. The pattern:

```go
type requestIDKey struct{}                                    // unexported key type avoids collisions

ctx := context.WithValue(r.Context(), requestIDKey{}, id)     // store
r = r.WithContext(ctx)                                        // attach to request

// later, in a handler:
id, _ := r.Context().Value(requestIDKey{}).(string)           // read
```

**Why an empty struct as the key?** Two unrelated middlewares using `"requestID"` as a string key would collide. An unexported struct type is unique by definition.

**Don't put business data in context.** It's an escape hatch for request-scoped values that are awkward to pass as parameters. Use it for: cancellation, deadlines, request ID, auth-derived `userID`. Don't use it for: form data, query params, anything that should be an argument.

---

## 6. `chi/middleware` — the off-the-shelf version

After you write your own, you'll appreciate the chi versions. Install:

```powershell
go get github.com/go-chi/chi/v5/middleware
```

Equivalents:

| Yours                | `chi/middleware`                       | Notes |
| -------------------- | -------------------------------------- | ----- |
| `RequestID`          | `middleware.RequestID`                 | Identical concept, key is `middleware.RequestIDKey`. |
| `Logger`             | `middleware.Logger`                    | Colourful CLI output, configurable. |
| `Recover`            | `middleware.Recoverer`                 | Prints the stack trace, returns 500. |
|                      | `middleware.RealIP`                    | Reads `X-Forwarded-For` / `X-Real-IP` behind proxies. |
|                      | `middleware.StripSlashes`              | Treats `/users/` like `/users`. |
|                      | `middleware.Timeout(10 * time.Second)` | Cancels request context after N seconds. |
|                      | `middleware.Compress(5)`               | gzip responses. |
|                      | `middleware.CleanPath`                 | Normalises `//` and `..`. |

**You don't have to use them.** Two reasons people do:

1. They've been battle-tested by thousands of teams.
2. You stop maintaining boring code.

**Two reasons people roll their own:**

1. The default output isn't JSON (matters when your logs go to Elastic/Datadog).
2. They want zero non-stdlib deps in this layer.

Today you'll do both: ship your own first, then `r.Use(middleware.Logger, middleware.Recoverer)` as a comparison.

---

## 7. Where middleware lives in this project

```
Day_06_middleware/
├── go.mod
├── main.go
└── internal/
    ├── middleware/
    │   └── middleware.go         ← your hand-written ones
    └── respond/
        └── respond.go             ← copied from Day 5
```

`main.go` mounts the chain on the chi router:

```go
r := chi.NewRouter()
r.Use(mw.Recover)        // outermost
r.Use(mw.RequestID)
r.Use(mw.Logger)
// ... routes ...
```

---

## 8. Run it

```powershell
go mod init day06
go get github.com/go-chi/chi/v5
go get github.com/go-chi/chi/v5/middleware
go run .
```

Hit any route and watch the terminal:

```
GET  /users          200  1.2ms   rid=8f3a91c2
POST /users          201  4.8ms   rid=7b2c449e
GET  /users/999      404  0.4ms   rid=12a89ef0
GET  /panic          500  0.2ms   rid=a44feb70
```

The `rid` is also returned in the `X-Request-ID` response header. If the client passes one in `X-Request-ID`, you reuse it. That's how distributed tracing starts.

---

## 9. What's next

**Day 7** is the Week 1 mini-project: a real in-memory To-Do REST API with chi + your middleware. Everything from Days 1–6 plugs in.
