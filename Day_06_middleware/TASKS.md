# Day 6 — Practice Tasks

Each task adds, swaps, or studies one middleware. Type it; the muscle memory matters most here because you'll use this pattern every day.

> **Before you start:**
>
> ```powershell
> go mod init day06
> go get github.com/go-chi/chi/v5
> go run .
> ```
>
> Hit a few routes and watch the terminal. You should see one log line per request, with the `rid=` matching the `X-Request-ID` response header.

---

## Warm-up (observe what already exists)

- [ ] `curl.exe -i http://localhost:8080/users` — note the `X-Request-ID` header. Curl again and confirm it changes each call.
- [ ] `curl.exe -i -H "X-Request-ID: my-trace-1234" http://localhost:8080/users` — note the **same** ID comes back. That's how cross-service tracing starts.
- [ ] `curl.exe http://localhost:8080/panic` — clean JSON 500, server doesn't die. Server log shows the stack trace + request ID.
- [ ] Check your server terminal: every line should be `METHOD PATH STATUS DURATION rid=... size=...`.

---

## Task 1 — Log the request ID inside a handler

The RequestID middleware stores the ID on the context; your handlers don't read it yet.

- [ ] In `getUserHandler`, after `parseID`, log: `log.Printf("getUser id=%d rid=%s", id, mw.GetRequestID(r.Context()))`.
- [ ] Curl `GET /users/1` and confirm the rid in the handler log matches the rid in the middleware log line *and* the response header. **All three are the same ID.** That's the magic.

**Why:** this is the plumbing that Day 25's `slog` package will replace with structured logging. Today you see it manually.

---

## Task 2 — A "slow request" warning middleware

- [ ] Write a new middleware `SlowWarn(threshold time.Duration)` that wraps a handler and, *after* it runs, logs `WARN slow request: ...` if duration > threshold.
- [ ] Note this is a **higher-order** middleware — it takes a parameter and *returns* a middleware:
  ```go
  func SlowWarn(threshold time.Duration) func(http.Handler) http.Handler {
      return func(next http.Handler) http.Handler {
          return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
              // ...
          })
      }
  }
  ```
- [ ] `r.Use(mw.SlowWarn(50 * time.Millisecond))` after the Logger.
- [ ] Add a route `/slow` that does `time.Sleep(200ms)` and confirm the warn fires.

**Why:** higher-order middleware is the pattern for configurable middleware (timeouts, rate limits, CORS allow-lists, …). Practising it once is enough.

---

## Task 3 — A `Timeout(d time.Duration)` middleware

- [ ] Use `context.WithTimeout` to cancel the request context after `d`. Attach it with `r.WithContext(ctx)`.
- [ ] If a handler runs past `d`, the context is cancelled — most well-behaved handlers honour this and abort.
- [ ] Wrap your `/slow` route with `mw.Timeout(100 * time.Millisecond)` and verify it 504s (or 500s, depending on how you implement) when the sleep is longer.

**Why:** prod APIs always have a timeout middleware. A handler with a slow DB query shouldn't hold a goroutine forever.

---

## Task 4 — Add `chi/middleware` side-by-side

- [ ] `go get github.com/go-chi/chi/v5/middleware`
- [ ] Comment out your own `mw.RequestID`, `mw.Logger`, `mw.Recover`, and use chi's equivalents:
  ```go
  import chimw "github.com/go-chi/chi/v5/middleware"
  r.Use(chimw.RequestID, chimw.Logger, chimw.Recoverer)
  ```
- [ ] Run the same curls. Note differences: chi's `Logger` is coloured for terminals (not JSON), `Recoverer` prints a fuller stack, `RequestID` generates a longer ID.
- [ ] Decide which set you'd ship and write 2 lines in your "What I learned" section justifying it.

**Why:** rolling your own is a great learning exercise; using the well-tested library version is the right professional call most of the time. Knowing both is the win.

---

## Task 5 — A CORS middleware (hand-written)

- [ ] Add `func CORS(allowedOrigins []string) func(http.Handler) http.Handler`.
- [ ] On every request, set `Access-Control-Allow-Origin` (only if the request's `Origin` is in the allow-list).
- [ ] On `OPTIONS` preflight, also set `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, and respond `204 No Content` immediately — don't call `next`.
- [ ] `r.Use(mw.CORS([]string{"http://localhost:3000"}))` at the top of the chain (outermost).
- [ ] Test:
  ```powershell
  curl.exe -i -X OPTIONS -H "Origin: http://localhost:3000" `
    -H "Access-Control-Request-Method: POST" http://localhost:8080/users
  ```
  Expect 204 with the right headers.

**Why:** CORS issues kill prototype demos more often than auth does. Knowing how the headers actually work means you can debug them without "fix CORS" guesswork.

---

## Task 6 — Use a middleware ONLY inside a group

- [ ] Reuse your Day 5 Task 5 `requireAPIKey` middleware (or write one fresh).
- [ ] `r.Group(func(r chi.Router) { r.Use(mw.RequireAPIKey); ... })` around write methods only (POST, PUT, DELETE).
- [ ] Confirm `GET /users` works without an API key, but `POST /users` returns 401 without the key.
- [ ] Confirm the rid in the 401's `X-Request-ID` matches the server log line — proof that middleware ordering is right.

**Why:** real apps mix public + protected routes. Per-group middleware is the cleanest way to do it.

---

## Stretch — only if you're flying

- [ ] Make your `Logger` middleware emit **JSON** instead of one line of text:
  ```json
  {"ts":"...","method":"GET","path":"/users","status":200,"dur_ms":1.2,"rid":"..."}
  ```
  This is the format an aggregator (Datadog, Loki, Elastic) actually wants. Day 25's `slog` does this with stdlib alone.
- [ ] Read `chi/middleware`'s `Recoverer` source (it's <100 lines): <https://github.com/go-chi/chi/blob/master/middleware/recoverer.go>. Note how it handles `http.ErrAbortHandler` specially.
- [ ] Convince yourself middleware is just function composition by writing one manually without `r.Use`:
  ```go
  handler := mw.Recover(mw.RequestID(mw.Logger(myHandler)))
  ```

