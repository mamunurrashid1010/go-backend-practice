# Day 25 — Structured Logging with `log/slog`

> **Goal:** replace `log.Printf` everywhere with `log/slog`, build a **request-scoped logger** on `context.Context`, and enrich it with `rid`, `method`, `path`, and (after auth) `user_id`. The output is JSON your log aggregator can actually search.

`log/slog` is stdlib (Go 1.21+). No dependency. No magic.

---

## 1. Why structured logging

`log.Printf("user 42 created note 7 in 1.2ms")` is a sentence. Useful to a human reading one line. Useless when you have a million lines and need to ask "show me every event for user 42 last hour."

Structured logging emits **key-value pairs**:

```json
{
  "time": "2026-06-09T12:00:00Z",
  "level": "INFO",
  "msg": "note created",
  "rid": "8f3a91c2",
  "user_id": 42,
  "note_id": 7,
  "duration_ms": 1.2
}
```

Now:
- Filter by `user_id = 42` in Elastic/Loki/Datadog with a real query.
- Aggregate `duration_ms` across millions of requests.
- Correlate every line in a request via `rid`.
- Alert on `level = ERROR` count > N per minute.

Same information, infinitely more useful. **You're not logging for yourself — you're logging for the next on-call engineer at 2am.**

---

## 2. `log/slog` in 60 seconds

```go
slog.Info("server started", slog.String("addr", ":8080"), slog.Int("pid", os.Getpid()))
slog.Warn("retry exhausted", slog.Int("attempts", 3), slog.Any("err", err))
slog.Error("db query failed", slog.String("query", q), slog.Any("err", err))
```

| Function | When |
| --- | --- |
| `slog.Debug` | Verbose — off in prod |
| `slog.Info`  | Normal operations |
| `slog.Warn`  | Recoverable problem |
| `slog.Error` | Something is broken |

Each attribute is **typed**:
- `slog.String("name", "x")`
- `slog.Int("count", 7)`
- `slog.Int64("id", 42)`
- `slog.Duration("dur", time.Since(start))`
- `slog.Any("err", err)` (last resort — uses reflection)
- `slog.Bool`, `slog.Float64`, `slog.Time`

The typed forms avoid the reflection cost of `slog.Any`. On a hot path, prefer the typed ones.

---

## 3. The two built-in handlers

`slog` separates **what to log** (the call site) from **how to format it** (the handler).

```go
opts := &slog.HandlerOptions{Level: slog.LevelInfo}

dev  := slog.NewTextHandler(os.Stdout, opts)   // pretty, human-readable
prod := slog.NewJSONHandler(os.Stdout, opts)   // one-line JSON, machine-readable

logger := slog.New(prod)
```

Pick by environment:

| Env | Handler | Why |
| --- | --- | --- |
| `development` | `TextHandler` | Coloured-ish, single-line text in your terminal |
| `staging` / `production` | `JSONHandler` | Stdout straight into Loki/Datadog/Elastic |

[internal/logging/logging.go](internal/logging/logging.go) builds the right one from config.

---

## 4. The request-scoped logger pattern

The big idea: **enrich the logger as more is known about the request, then propagate via context.**

```
1. main.go              creates the BASE logger (service-wide fields)
2. middleware.Logger    wraps it with rid, method, path
3. middleware.RequireAuth wraps it again with user_id (once token verified)
4. handlers + services  call logging.From(r.Context()).Info(...) — every line
                          automatically carries rid + method + path + user_id
```

The mechanism is `logger.With(...)`. It returns a **new** logger that has all the parent's fields plus the new ones:

```go
reqLog := baseLog.With(
    slog.String("rid", rid),
    slog.String("method", r.Method),
    slog.String("path", r.URL.Path),
)
ctx := logging.With(r.Context(), reqLog)
next.ServeHTTP(w, r.WithContext(ctx))
```

Now downstream code does:

```go
logging.From(r.Context()).Info("note created", slog.Int64("note_id", n.ID))
```

…and the emitted line is:

```json
{"time":"...","level":"INFO","msg":"note created","rid":"8f3a","method":"POST","path":"/notes","user_id":42,"note_id":7}
```

You didn't write any of those fields *here*. The middleware did, once.

---

## 5. The `logging` package — three small functions

```go
// internal/logging/logging.go

func New(level string, asJSON bool) *slog.Logger { ... }

// With attaches a logger to the context.
func With(ctx context.Context, l *slog.Logger) context.Context { ... }

// From retrieves the logger, falling back to slog.Default if missing.
func From(ctx context.Context) *slog.Logger { ... }
```

`With` + `From` use an unexported struct as the key (same trick as Day 6's `RequestID` and Day 19's `userID`).

---

## 6. `respond.Internal` now takes a `context.Context`

To log an internal error with the *request's* logger (not a global one), we change the signature:

```go
// before (Day 24):
func Internal(w http.ResponseWriter, err error) { log.Printf("internal error: %v", err); ... }

// after (Day 25):
func Internal(ctx context.Context, w http.ResponseWriter, err error) {
    logging.From(ctx).ErrorContext(ctx, "internal error", slog.Any("err", err))
    Error(w, http.StatusInternalServerError, "INTERNAL", "internal server error")
}
```

Every caller now passes `r.Context()`:

```go
respond.Internal(r.Context(), w, err)
```

The 500 the client gets is unchanged. The log line is now structured + correlated.

---

## 7. Configurable log level

Adding to `config.AuthConfig` would be wrong (it's not auth). New top-level field:

```go
type Config struct {
    Env      string `env:"APP_ENV" envDefault:"development"`
    LogLevel string `env:"LOG_LEVEL" envDefault:"info"`   // debug | info | warn | error
    // ...
}
```

`validate()` rejects anything else. In dev you might set `LOG_LEVEL=debug` to see the chatty stuff; prod stays at `info`.

---

## 8. What this looks like in your terminal

Boot the server in dev:

```
time=2026-06-09T12:00:00.000+00:00 level=INFO msg="config loaded" env=development addr=:8080 log_level=info
time=2026-06-09T12:00:00.020+00:00 level=INFO msg="connected to postgres"
time=2026-06-09T12:00:00.030+00:00 level=INFO msg="listening" addr=:8080
```

Now make a request to a protected route:

```powershell
curl.exe -H "Authorization: Bearer ..." http://localhost:8080/notes/7
```

Server log:

```
time=2026-06-09T12:00:05.123+00:00 level=INFO msg="http_request" rid=8f3a91c2 method=GET path=/notes/7 user_id=42 status=200 duration=1.2ms size=126
```

Set `APP_ENV=production` and the same line is JSON:

```json
{"time":"2026-06-09T12:00:05.123Z","level":"INFO","msg":"http_request","rid":"8f3a91c2","method":"GET","path":"/notes/7","user_id":42,"status":200,"duration":1200000,"size":126}
```

That's a real line a real aggregator can index.

---

## 9. What changed from Day 24

| File | Change |
| --- | --- |
| `internal/logging/logging.go` | **NEW** — `New`, `With`, `From` |
| `internal/config/config.go` | added `LOG_LEVEL` |
| `internal/middleware/middleware.go` | `Logger` enriches a request logger and stashes it on ctx; `Recover` logs with slog |
| `internal/respond/respond.go` | `Internal(ctx, w, err)` — pulls the logger from ctx |
| `internal/auth/middleware.go` | `RequireAuth` enriches the logger with `user_id` after verifying the token |
| `internal/auth/handler.go`, `internal/notes/handler.go` | callers updated to pass `r.Context()` to `respond.Internal` |
| `main.go` | builds the slog logger from config; passes it into `mw.Logger(base)` |

Service, repository, validate, the test files — unchanged.

---

## 10. Run it

```powershell
cd Day_25_slog
docker compose up -d
go mod init day25
# ... usual go gets
go run .
```

Hit a few endpoints, watch the structured log lines stream past. Set `LOG_LEVEL=debug` and `APP_ENV=production` to see the JSON format.

```powershell
# Fast tests still pass — same handler contract
go test ./internal/notes/

# Integration tests still pass
go test -tags integration ./internal/notes/
```

---

## 11. What's next

**Day 26** — pagination beyond `?limit=N`. Cursor pagination (`?after=<id>`) for stable, scalable lists. Then **Day 27** — rate limiting + CORS, and **Day 28** — the Week 4 polish (README badges, clean repo).

After today, every log line is queryable. After Day 27, your API is hardened against abuse. After Day 28, the project is interview-ready.
