# Day 37 — Practice Tasks

The demos show the shape. These tasks give you the reps: replace hand-rolled patterns with `errgroup`, hit the corners where `SetLimit` isn't enough, feel the difference weighted work makes.

> **Before you start:**
>
> ```powershell
> cd Day_37_bounded_concurrency
> go mod init day37
> go get golang.org/x/sync/errgroup
> go get golang.org/x/sync/semaphore
> go mod tidy
> ```

---

## Warm-up — run everything

- [ ] `go run ./cmd/errgroup` — one bad URL cancels the others; wall-clock stays under a second.
- [ ] `go run ./cmd/errgroup_limit` — peak in-flight equals the limit; wall-clock scales with jobs / workers.
- [ ] `go run ./cmd/semaphore` — read the timestamps carefully. The `huge-2` job (cost 10) waits until the entire pool is idle.
- [ ] `go run ./cmd/workerpool` — 3 workers exit only when the jobs channel closes and drains.
- [ ] `go run ./cmd/fanout_fanin` — confirms `sum_{i=1..20} i² = 2870`.

---

## Task 1 — Rewrite Day 36 Task 3 with `errgroup.SetLimit`

Yesterday's Task 3 asked you to build a bounded worker pool by hand: `chan Job`, N workers, `WaitGroup`, `close`. Do the same with `errgroup.SetLimit`:

- [ ] Copy your Day 36 Task 3 code.
- [ ] Replace the whole pool with:
  ```go
  g, _ := errgroup.WithContext(ctx)
  g.SetLimit(4)
  for _, job := range jobs {
      job := job
      g.Go(func() error { job(); return nil })
  }
  return g.Wait()
  ```
- [ ] Count the lines. The hand-rolled version was ~30; this is 6.

Then reflect: what did you *lose*? (Answer: long-lived workers, per-worker state, explicit control over the topology. For a one-shot batch you lost nothing.)

---

## Task 2 — Fail fast under contention

Modify `cmd/errgroup_limit`:

- [ ] Change one of the jobs to return an error after sleeping 50ms.
- [ ] Confirm the group cancels *even though there are still 15+ jobs waiting to launch*.
- [ ] The ones still queued NEVER run — `g.Go` returns immediately without executing when the group's ctx is cancelled.

This is the killer feature vs a hand-rolled pool: automatic backpressure that turns into "abort" on first error.

---

## Task 3 — CPU-bound weighting with `semaphore.Weighted`

Rewrite `cmd/semaphore` to use CPU cores as the budget:

- [ ] `capacity := int64(runtime.NumCPU())`.
- [ ] Job cost is "how many cores this job wants to use." Small = 1, huge = capacity.
- [ ] Simulate work by spinning CPU (a tight loop counting up to a large N), not `time.Sleep`. Sleep frees the CPU; a spin actually contends.
- [ ] Notice that with 4 cores and small=1 / huge=4, you can either run 4 smalls OR 1 huge — never mix.

This is the real reason to reach for weighted semaphores: bounding an *actual* resource, not just goroutine count.

---

## Task 4 — Rate-limit a downstream call

You have a downstream API that allows 5 req/s per client. Your service needs to call it from ~50 concurrent requests without exceeding the limit.

- [ ] Sketch two solutions:
  1. `errgroup.SetLimit(5)` around the batch — but this bounds concurrency, not rate. If each call takes 200ms, you're at 25 req/s.
  2. A **rate limiter** (`golang.org/x/time/rate` — from Day 27) at 5/s, called before every request.
- [ ] Implement (2). One `rate.NewLimiter(5, 1)` at package level; every downstream call does `lim.Wait(ctx)` first.
- [ ] Notice: `errgroup.SetLimit` and the rate limiter are complementary, not competitors. Bound concurrency to protect memory; rate-limit to protect the downstream.

---

## Task 5 — Fan-out with per-worker HTTP clients

Modify `cmd/fanout_fanin`:

- [ ] Instead of `square`, have each worker call `httpbin.org/uuid` (or a local httptest server) N times and count how many uuids it received.
- [ ] Each worker holds its own `*http.Client` with `Transport.MaxIdleConnsPerHost = 1` — this is the "stateful worker" case where a hand-built pool wins.
- [ ] Merge collects the counts; main sums them.

Compare: could you have done this with `errgroup.SetLimit`? Yes — but you'd share one HTTP client across all goroutines, which is often what you want (connection pooling) but sometimes isn't (isolating slow clients, per-worker auth).

---

## Task 6 — Detect the "one-slow-goroutine-holds-the-group" bug

In `cmd/errgroup`, `slow` takes 1s and is cancellable. Now break the cancellation:

- [ ] In `fetch`, change the `"slow"` branch to `time.Sleep(1 * time.Second); return "OK slow", nil` — no `select`, no ctx respect.
- [ ] Rerun. The group still returns "boom: bad" quickly *but the wall-clock is now 1s*, because the slow goroutine finishes before g.Wait() returns.

`errgroup` doesn't kill goroutines. It cancels the ctx. If your goroutine ignores the ctx, it keeps running. This is *the* most common concurrency bug in Go services. Day 38 makes this visible with `pprof`.

---

## Stretch — only if you're flying

- [ ] **`errgroup.TryGo`.** Rewrite `cmd/errgroup_limit` so `g.TryGo` drops jobs when the pool is full instead of blocking. Log how many were dropped. (Use case: metrics collection where dropping is better than slowing down.)
- [ ] **Weighted memory budget.** `sem := semaphore.NewWeighted(int64(1024))` — 1024 MB. Jobs of random sizes 50–500 MB. Sort by size before submitting — does anything change? (Answer: yes, tail latency; small-first is worst.)
- [ ] **Merge with `context`.** Add a `Merge` variant that stops draining if `ctx` is cancelled. Fix the goroutine leak that a naive Merge has when the caller stops reading from the output.
- [ ] **Structured concurrency wishlist.** Sketch what `errgroup` would look like if Go had structured concurrency (the "parent scope must outlive all its children" rule). What would `g.Wait()` be replaced by?

---

## What I learned (Day 37)

> 3 bullets in your own words.

-
-
-
