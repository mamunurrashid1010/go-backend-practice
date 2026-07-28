# Day 37 — `errgroup`, `semaphore`, Bounded Concurrency

> **Goal:** the coordination layer on top of Day 36. `errgroup.Group` for "N goroutines that all need to succeed," `errgroup.SetLimit` for bounded fan-out, `semaphore.Weighted` for jobs that aren't all the same size, and the two topologies you keep meeting in production Go code: **worker pools** and **fan-out / fan-in**.
>
> **The frame:** Day 36 gave you channels + `WaitGroup` + `Mutex`. Day 37 gives you the two `golang.org/x/sync` libraries that make those primitives *ergonomic*. Every real Go backend I've worked in uses `errgroup` on almost every parallel operation.

---

## 1. `errgroup.Group` — a WaitGroup that also carries an error

The three things `WaitGroup` doesn't give you and you always end up hand-rolling:

1. **The first error from any goroutine.** With `WaitGroup` you invent a `chan error` and a mutex-guarded `err`.
2. **Cancel the others when one fails.** With `WaitGroup` you thread a `context.CancelFunc` through by hand.
3. **A single `Wait()` that returns that first error.**

`errgroup.Group` bundles all three:

```go
g, ctx := errgroup.WithContext(parentCtx)

for _, url := range urls {
    url := url // (safe on Go 1.22+; keep the shadow for older Go)
    g.Go(func() error {
        return fetch(ctx, url)   // cancelled if ANY sibling returns err
    })
}

if err := g.Wait(); err != nil {
    // first error to fire; everyone else is cancelling
    return err
}
```

Two things worth internalising:

- **`ctx` is derived from `parentCtx` and cancelled the moment any `g.Go(fn)` returns non-nil**. Long-running siblings must respect `ctx.Done()`; otherwise they keep running until they finish naturally. That's why the fn takes `ctx` — cancellation is cooperative.
- **`g.Wait()` returns exactly one error.** Not a slice, not `errors.Join` — the first non-nil return. If you want all of them, either build your own bag or use `g.TryGo` + your own slice.

Demo: [cmd/errgroup/main.go](cmd/errgroup/main.go).

---

## 2. `errgroup.SetLimit(N)` — bounded fan-out inline

Before 2022 you built bounded pools by hand: N worker goroutines, `chan Job`, `WaitGroup`, `close(jobs)`, `wg.Wait()`. `SetLimit` compresses that into one line:

```go
g, ctx := errgroup.WithContext(ctx)
g.SetLimit(8) // at most 8 goroutines running at once

for _, item := range items {
    item := item
    g.Go(func() error { return handle(ctx, item) })
}
return g.Wait()
```

`g.Go` blocks when the pool is full. So the `for` loop naturally applies backpressure — you can iterate over a million items and only 8 will ever be in-flight. `SetLimit(0)` means unlimited (the default).

Two edge cases:

- **`g.TryGo`** attempts to launch without blocking; returns `false` if the pool is full. Useful for "do this if there's slack, drop otherwise."
- **`SetLimit(-1)`** means "unlimited" like `0`, but is explicit. Style choice.

Demo: [cmd/errgroup_limit/main.go](cmd/errgroup_limit/main.go).

---

## 3. `semaphore.Weighted` — for jobs that aren't equal

`errgroup.SetLimit` counts every goroutine as weight 1. Real jobs aren't equal: a small task and a huge memory hog both count as "one job," but the huge one should push you to `N-1` slots free while it runs.

`semaphore.Weighted` fixes that:

```go
sem := semaphore.NewWeighted(int64(runtime.NumCPU() * 100)) // total capacity

for _, job := range jobs {
    cost := int64(job.MemMB)
    if err := sem.Acquire(ctx, cost); err != nil { // blocks if not enough capacity
        return err
    }
    go func(job Job) {
        defer sem.Release(cost)
        run(job)
    }(job)
}
```

Two ways to think about the capacity:

- **CPU slots.** `NumCPU()`; weights are "how many cores." A 4-way parallel task acquires 4.
- **Memory budget.** Total is MB; weights are per-job MB. Small jobs pack, huge jobs run alone.

`Acquire` respects `ctx` — cancel the ctx and it unblocks with `ctx.Err()`. There's also `TryAcquire` for non-blocking.

Demo: [cmd/semaphore/main.go](cmd/semaphore/main.go).

**When to reach for `semaphore` over `errgroup.SetLimit`:** only when weights matter. If every job costs the same, `SetLimit(N)` is simpler.

---

## 4. Worker pool — the classic channel-based pattern

Before either `errgroup` or `semaphore` you built this:

```
   ┌── worker 1 ──┐
producer ─▶│ jobs chan │──▶ ├── worker 2 ──┤ ──▶ (nothing; workers write side-effects)
   └── worker 3 ──┘
```

- N worker goroutines each `range` over `jobs`.
- Producer sends onto `jobs` and closes when done.
- `sync.WaitGroup` counts the workers; caller `Wait()`s.

You still write this by hand when:

- Workers are **long-lived** across many batches. A pool that lives for the process's lifetime.
- You want workers to be **stateful** (their own buffers, DB connections, etc.).
- You need a **specific topology** (e.g. workers with per-worker output channels).

For a fire-and-forget "run N items in parallel," `errgroup.SetLimit(N)` is now the right tool. But the classic form still shows up. See [cmd/workerpool/main.go](cmd/workerpool/main.go).

---

## 5. Fan-out / fan-in — a topology, not a library

**Fan-out.** One producer, N workers, each doing the same work in parallel.

**Fan-in.** N producers, one consumer, values from all producers merged onto one output channel.

They combine into the classic pattern:

```
                        ┌── worker ──┐
input ──▶  producer ──▶ │ jobs chan │ ── worker ── ▶ ┐ results chan
                        └── worker ──┘                ┘
                                                     ▶ collector
```

A `Merge(chs ...<-chan T) <-chan T` helper is the standard fan-in primitive: one goroutine per input channel copies onto a shared output; a `WaitGroup` closes the output when all inputs are drained.

Demo: [cmd/fanout_fanin/main.go](cmd/fanout_fanin/main.go).

The pattern is behind: image-thumbnail pipelines, HTTP client batchers, log processors, most map-reduce shapes. It's also what any streaming database driver looks like inside.

---

## 6. Cheatsheet — when to reach for what

| Situation | Reach for |
| --- | --- |
| "Run these N things in parallel, fail fast on any error." | `errgroup.WithContext` |
| Same, but "no more than K in flight." | `errgroup.Group{}` + `SetLimit(K)` |
| K in flight, but jobs have different costs / weights. | `semaphore.Weighted` |
| Long-lived pool of stateful workers (DB conns, buffers). | Hand-built worker pool with `chan Job` |
| Pipeline of stages with independent lifecycles. | Chain of channels; `Merge` for fan-in |
| Rate-limit calls across a whole cluster of replicas. | Day 33 Redis limiter — different problem |

**Everything above assumes cooperative cancellation via `ctx`.** A worker that ignores `ctx.Done()` in a hot loop will happily keep running for another 30 minutes after `g.Wait()` returned an error. Every long-lived goroutine you write today should have a `select { case <-ctx.Done(): return }` in its main loop.

---

## 7. What changed from Day 36

Nothing carries. Day 37 continues the fundamentals arc; no HTTP surface. Day 38 goes deeper on `context` and pulls out `pprof goroutine` to hunt the leaks these patterns can create if used sloppily.

---

## 8. Run it

```powershell
cd Day_37_bounded_concurrency
go mod init day37
go get golang.org/x/sync/errgroup
go get golang.org/x/sync/semaphore
go mod tidy

# Each concept is its own runnable:
go run ./cmd/errgroup           # basic errgroup with cancel-on-first-error
go run ./cmd/errgroup_limit     # SetLimit(N) as the modern bounded pool
go run ./cmd/semaphore          # Weighted semaphore for varied-cost jobs
go run ./cmd/workerpool         # classic hand-built channel-based pool
go run ./cmd/fanout_fanin       # 1 producer, N workers, Merge collector
```

---

## 9. What's next

**Day 38 — `context.Context` deep dive + goroutine-leak hunt with `pprof`.** The `ctx` we've been threading through everything has more corners than most Go tutorials show, and every concurrency pattern here leaks goroutines if you use it wrong. Day 38 shows how to *see* those leaks in a running process.
