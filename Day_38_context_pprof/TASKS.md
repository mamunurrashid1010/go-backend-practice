# Day 38 — Practice Tasks

The demos are the setup. These tasks force the debugging skill: read a `pprof` listing, find the leak, fix it, prove it stopped.

> **Before you start:**
>
> ```powershell
> cd Day_38_context_pprof
> go mod init day38
> go mod tidy
> ```

Also install the interactive tool if you don't already have it — it ships with the Go toolchain, so this is usually a no-op:

```powershell
go tool pprof --help
```

If that prints the pprof help, you're set.

---

## Warm-up — run everything

- [ ] `go run ./cmd/basics` — feel `errors.Is(err, context.DeadlineExceeded)` return true.
- [ ] `go run ./cmd/propagation` — one parent, two children, one cancel; both fire.
- [ ] `go run ./cmd/values` — request id + user id on the same ctx.
- [ ] `go run ./cmd/leak` — see the goroutine count grow by exactly 100, then stay flat.
- [ ] `go run ./cmd/pprof_server` — leave it running; walk the pprof commands in the README.

---

## Task 1 — Fix `leakingFetch` without changing the channel type

In `cmd/leak`, `leakingFetch` uses `make(chan Result)`. The README's fix is to bump to `make(chan Result, 1)`. Do it *without* changing the channel:

- [ ] Change the goroutine to `select` on either "send" or "ctx.Done()", so if ctx fires it drops the value and returns.
- [ ] Rerun `cmd/leak` and confirm the count stays flat.

Both fixes are legitimate. The buffered-channel form is shorter; the select form is more explicit about the goroutine's cooperation with ctx.

---

## Task 2 — Read the actual pprof listing

Start the server:

```powershell
go run ./cmd/pprof_server
```

Baseline:

```powershell
curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | Select-String "total"
# goroutine profile: total 4
```

Now hit `/leak` 30 times:

```powershell
1..30 | ForEach-Object { curl.exe -s -o $null http://localhost:8080/leak }
```

- [ ] Fetch the profile again and confirm the total went up by ~30.
- [ ] Grep for `leakingHandler` — find the exact goroutine count stuck there.

Now do the same for `/fixed`:

```powershell
1..30 | ForEach-Object { curl.exe -s -o $null http://localhost:8080/fixed }
```

- [ ] Fetch the profile. The count didn't grow. The buffered channel lets the goroutine complete.

---

## Task 3 — Use `go tool pprof` interactively

```powershell
go tool pprof http://localhost:8080/debug/pprof/goroutine
```

At the `(pprof)` prompt:

- [ ] `top` — top functions by goroutine count. Expect `main.leakingHandler.func1` near the top after the leak.
- [ ] `list main.leakingHandler` — source listing with count-per-line annotations. See exactly which line has the blocked goroutines.
- [ ] `web` — opens an SVG call graph in your browser (needs `graphviz` installed; skip if you don't have it).
- [ ] `quit`.

This same triad works for CPU (`/debug/pprof/profile`) and heap (`/debug/pprof/heap`) profiles — Day 39.

---

## Task 4 — Prove `respectingHandler` doesn't leak either

The server has three handlers: `/leak`, `/fixed`, and `/respecting`. The third uses `ctx.Done()` inside the goroutine.

- [ ] Baseline. Hit `/respecting` 30 times. Check the profile.
- [ ] Notice the goroutines briefly go up (~30) while their `time.After(1s)` is pending, then settle back down as they complete.
- [ ] Set a low `ReadTimeout` on your curl (e.g. `--max-time 0.5`) and hit `/respecting`. The client hangs up early; the ctx fires; the goroutine sees `ctx.Done()` and exits without sending on `ch`. Verify no leak.

This is the pattern for handlers that spawn long-running work: pass `r.Context()` in, respect it.

---

## Task 5 — Two ways to break `errgroup`

You've been using `errgroup` since Day 37. Now break it two ways:

- [ ] **Leak on error.** In `cmd/errgroup` (from Day 37 — either open that repo or paste the code), change the "slow" fetch to `time.Sleep(5 * time.Second); return "OK slow", nil` — no ctx check. Rerun. Wall-clock is now 5s. The group returned "boom: bad" fast but the "slow" goroutine kept running.
- [ ] **Fix.** Restore the ctx-aware `select` from Day 37. Wall-clock drops back under 1s.

The lesson: **`errgroup` doesn't kill goroutines. It cancels the ctx.** Yesterday's Task 6 said the same thing; today you have `pprof` to prove it.

---

## Task 6 — A per-request cancellation propagation exercise

- [ ] Add a handler to `pprof_server` at `/tree`. It should:
  1. Take `r.Context()`.
  2. Launch three goroutines that each `<-ctx.Done()` and log which one saw the cancel first.
  3. Wait for one of them to finish, then return.
- [ ] Curl it, then curl it and Ctrl+C during the request.
- [ ] Confirm from your server logs that all three goroutines saw the cancel — the propagation was automatic.

This models a request that fans out to three downstream services. Cancelling the request cancels every downstream call for free.

---

## Task 7 — Sneak a memory leak in via context.Value

- [ ] In a new small program, put an ever-growing `*[]byte` into `ctx.Value`. Attach it to a base ctx that's shared across many requests.
- [ ] Watch RSS via `runtime.MemStats` grow request-by-request even though your handler returns nothing.
- [ ] Reflect: **why is stashing big things in `context.Value` a bad idea?** (Answer: contexts get retained by any goroutine that holds them; if you leak the goroutine you leak the ctx, and everything attached.)

Rule of thumb: only put **request-scoped identifiers** (ids, logger) in ctx.Value. Never mutable state, never large buffers.

---

## Stretch — only if you're flying

- [ ] **Continuous leak detector.** Add a background goroutine that samples `runtime.NumGoroutine()` every second and alerts on sustained growth (say, +100 in 60s). Wire it as a Prometheus counter (preview of Day 90).
- [ ] **`GODEBUG=gctrace=1`.** Run `pprof_server` with the env var set. Every GC prints one line. Hit the leaking endpoint 1000 times. Watch the heap grow. This is the same signal from a different angle.
- [ ] **`runtime.SetFinalizer`.** Add a finalizer on the value pushed onto the leaking channel that prints "GC'd me!". Confirm you never see the message from leaked goroutines — because the goroutine still holds a reference. Contrast with the fixed case where you *do* see it.
- [ ] **Chi middleware for pprof.** In the Day 35 notes API, mount `chi-middleware/profiler` under a gated route (`if cfg.IsDev()`). You now have goroutine profiling on the real service.

---

## What I learned (Day 38)

> 3 bullets in your own words.

-
-
-
