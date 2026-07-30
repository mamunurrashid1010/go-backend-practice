# Day 38 — `context.Context` + Goroutine Leaks via `pprof`

> **Goal:** the corner-heavy parts of `context.Context` (cancel / timeout / deadline / value; propagation rules; the four sentinel errors), then a live HTTP server that leaks goroutines on purpose so you can *see* them with `net/http/pprof`. The single most important debugging skill for a production Go service.
>
> **The frame:** Day 36 gave you primitives (goroutines, channels, mutex). Day 37 gave you coordination (`errgroup`, `semaphore`). Day 38 is about **lifecycles**: when goroutines start, when they stop, and — the whole point — when they *don't* stop.

---

## 1. `context.Context` in one page

```go
type Context interface {
    Deadline() (deadline time.Time, ok bool) // "when should this be done by?"
    Done() <-chan struct{}                    // closed when cancelled or deadline hit
    Err() error                               // nil while alive; context.Canceled or DeadlineExceeded once done
    Value(key any) any                        // request-scoped values (use sparingly)
}
```

Five things create a `Context`; all five compose:

```go
ctx := context.Background()                          // the root; use in main, tests, init
ctx := context.TODO()                                // "I don't know yet" — placeholder during a refactor
ctx, cancel := context.WithCancel(parent)            // manual cancel
ctx, cancel := context.WithTimeout(parent, 5 * time.Second)  // auto-cancel after d
ctx, cancel := context.WithDeadline(parent, t)       // auto-cancel at t
ctx := context.WithValue(parent, key, value)         // attach a value to the ctx tree
```

Every `WithCancel` / `WithTimeout` / `WithDeadline` returns a **`cancel` function you must call**. Failing to call it leaks the timer + a small amount of ctx tree memory:

```go
ctx, cancel := context.WithTimeout(parent, 5*time.Second)
defer cancel()                                       // always
```

`defer cancel()` is idempotent and cheap. Do it even when the ctx will fire on its own — it releases the timer immediately.

---

## 2. The tree

Contexts form a **tree**: every derived context has a parent. When a parent is cancelled, **all descendants are cancelled** (their `Done()` closes, their `Err()` returns the parent's err).

```
Background
    │
    └─ WithCancel(bg)        [parent request scope]
           │
           ├─ WithTimeout(parent, 200ms)   [outbound DB call]
           └─ WithValue(parent, "rid", "abc123")
                  │
                  └─ WithCancel(v)          [background worker for this request]
```

Cancel the top `WithCancel` and *every* child down the tree is cancelled simultaneously. Cancel a leaf and it doesn't affect anyone else.

**The "shortest deadline wins" rule.** If a child's deadline is later than its parent's, the parent still wins:

```go
parent, _ := context.WithTimeout(background, 100*time.Millisecond)
child, _  := context.WithTimeout(parent, 1*time.Second) // pointless; parent fires first
```

Demo: [cmd/basics/main.go](cmd/basics/main.go) and [cmd/propagation/main.go](cmd/propagation/main.go).

---

## 3. The two cancel errors

`ctx.Err()` returns exactly one of three values:

- **`nil`** — still alive.
- **`context.Canceled`** — someone called `cancel()`.
- **`context.DeadlineExceeded`** — the deadline elapsed.

Check them with `errors.Is`:

```go
select {
case <-ctx.Done():
    if errors.Is(ctx.Err(), context.DeadlineExceeded) {
        // treat as timeout — probably retryable
    }
    if errors.Is(ctx.Err(), context.Canceled) {
        // treat as caller-abandoned — probably not retryable
    }
    return ctx.Err()
}
```

Any function that wraps `ctx.Err()` (via `%w`) preserves the check under `errors.Is`. This shows up in `errgroup.Wait` returning the original error, `sql.DB.QueryContext` on timeout, `http.Client.Do` on request cancel — all of them either propagate or wrap.

---

## 4. `context.Value` — controversial for a reason

`ctx.Value(key)` attaches key/value pairs to the ctx tree. The community convention is:

- **Yes:** request-scoped values that would be a nightmare to plumb through every function — request ID, user ID, tenant ID, trace ID, request-scoped logger.
- **No:** dependencies — anything the function needs to *work*. Pass those as ordinary parameters. If you find yourself doing `db := ctx.Value(dbKey).(*sql.DB)`, undo it.

The type-key pattern prevents key collisions across packages:

```go
type userIDKey struct{}   // unexported empty struct — no other package can construct this key

func WithUserID(ctx context.Context, id int64) context.Context {
    return context.WithValue(ctx, userIDKey{}, id)
}
func GetUserID(ctx context.Context) (int64, bool) {
    id, ok := ctx.Value(userIDKey{}).(int64)
    return id, ok
}
```

This is exactly the pattern the notes-API codebase uses since Day 6 for `X-Request-ID`, since Day 19 for `user_id`, and since Day 25 for the request-scoped logger. If you've read those days, you've used it.

Demo: [cmd/values/main.go](cmd/values/main.go).

---

## 5. Cancellation is cooperative — how leaks happen

Nothing in Go *kills* a goroutine. `cancel()` closes a channel. A goroutine only stops if it **checks** for cancellation and returns.

The three antipatterns that leak:

**A. Goroutine blocks on a channel that no one will ever send to / receive from.**

```go
ch := make(chan int)
go func() { <-ch }() // waits forever
// nothing ever sends to ch
// goroutine leaks. Every time this function runs, one more leaks.
```

**B. Goroutine sends on an unbuffered channel that no one drains.**

```go
func Fetch(ctx context.Context) (Result, error) {
    ch := make(chan Result)         // unbuffered
    go func() {
        ch <- expensiveWork()       // BLOCKS if nobody reads
    }()

    select {
    case v := <-ch:
        return v, nil
    case <-ctx.Done():
        return Result{}, ctx.Err()  // <-- caller returns; ch has no reader anymore
    }
}
// If ctx wins the race, the goroutine's `ch <- ...` blocks forever.
// Fix: make(chan Result, 1) — buffered channel of size 1.
```

This is the classic "timeout leak." Fix with a **capacity-1 buffered channel** so the goroutine can always send and exit.

**C. Goroutine runs an infinite loop that doesn't check `ctx.Done()`.**

```go
go func() {
    for {
        doWork()   // never returns; never checks ctx
    }
}()
```

Fix: `select` on `<-ctx.Done()` in the loop, or make `doWork` accept `ctx` and check it.

Demo: [cmd/leak/main.go](cmd/leak/main.go) — runs the leak, prints `runtime.NumGoroutine()` before and after.

---

## 6. `pprof goroutine` — see the leak

Go's runtime keeps a live inventory of every goroutine, including its full stack trace. `net/http/pprof` exposes it over HTTP.

```go
import _ "net/http/pprof"    // registers /debug/pprof/* on http.DefaultServeMux

// If you're not using DefaultServeMux (chi, gin, echo), mount explicitly:
r.Mount("/debug/pprof", middleware.Profiler())  // chi-middleware helper
// or just:
r.Get("/debug/pprof/goroutine", pprof.Index)
r.Get("/debug/pprof/goroutine/", pprof.Handler("goroutine").ServeHTTP)
```

Two ways to inspect:

### The human-readable listing

```
curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | head -50
```

Prints one entry per unique stack, with a count:

```
goroutine profile: total 47
40 @ 0x1044b8c 0x1044c58 0x104489c 0x1090abc 0x11bc1c8 ...
#   0x11bc1c7   main.leakingHandler.func1+0x67   /path/leak/main.go:23
```

**40 goroutines are all in `leakingHandler.func1`.** That's the leak — that function should be exiting, not accumulating.

### The `go tool pprof` interactive REPL

```
go tool pprof http://localhost:8080/debug/pprof/goroutine

(pprof) top              # top functions by goroutine count
(pprof) list main.leakingHandler   # source listing with counts per line
(pprof) web              # opens an SVG call graph in your browser
```

The `top` / `list` / `web` triad is how you diagnose CPU + heap profiles too (Day 39). Learning it here on goroutines is the easiest entry.

Demo: [cmd/pprof_server/main.go](cmd/pprof_server/main.go) — a real HTTP server with the leak wired to `/leak` and pprof at `/debug/pprof/*`.

---

## 7. Three habits that prevent leaks

1. **Every long-lived goroutine takes `ctx` and selects on `<-ctx.Done()` in its main loop.** No exceptions.
2. **Every `chan T` used for "send result to caller" from an untrusted-duration goroutine is `make(chan T, 1)`.** The buffer is the goroutine's escape hatch.
3. **`defer cancel()` after every `WithCancel` / `WithTimeout` / `WithDeadline`.** Idempotent; costs nothing; catches "we returned early" cases.

Do all three and the antipatterns above go away by construction.

---

## 8. What changed from Day 37

Nothing carries. Third and final foundations day of Week 6. Day 39 goes back to writing code with these primitives on hot paths — benchmarks, the race detector, and CPU/heap pprof.

---

## 9. Run it

```powershell
cd Day_38_context_pprof
go mod init day38
go mod tidy

go run ./cmd/basics           # cancel / timeout / deadline
go run ./cmd/propagation      # parent cancel cancels the whole tree
go run ./cmd/values           # the type-key pattern; unrelated packages don't collide
go run ./cmd/leak             # runtime.NumGoroutine before/after — the leak in isolation
go run ./cmd/pprof_server     # HTTP server; instructions below
```

The `pprof_server` walkthrough:

```powershell
# Terminal 1:
go run ./cmd/pprof_server
# listening on http://localhost:8080

# Terminal 2:
curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | Select-String "total"
# total 4 (baseline)

# hit the leaking endpoint 20 times
1..20 | ForEach-Object {
    curl.exe -s -o $null http://localhost:8080/leak
}

curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | Select-String "total"
# total 24 (leaked 20)

# see who's leaking:
curl "http://localhost:8080/debug/pprof/goroutine?debug=1"
# scroll: 20 @ ... main.leakingHandler.func1  <-- there it is

# vs the fixed endpoint:
1..20 | ForEach-Object { curl.exe -s -o $null http://localhost:8080/fixed }
curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | Select-String "total"
# still 24 — no growth. Fixed handler cleans up.
```

---

## 10. What's next

**Day 39 — benchmarks, `-race`, `pprof` for CPU + heap.** Same `pprof` machinery as today, aimed at "why is this slow?" and "why is my heap ballooning?". Then Day 40 loads the API up with `k6` / `vegeta` and finds a real bottleneck.
