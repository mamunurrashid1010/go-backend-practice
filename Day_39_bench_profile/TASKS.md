# Day 39 — Practice Tasks

The demos give you shapes to run against. These tasks build the habits: read a benchmark output, compare two runs statistically, drive `pprof` interactively, and — the goal — turn a profile into a fix.

> **Before you start:**
>
> ```powershell
> cd Day_39_bench_profile
> go mod init day39
> go mod tidy
>
> # Optional but recommended:
> go install golang.org/x/perf/cmd/benchstat@latest
> ```

---

## Warm-up — run everything

- [ ] `go test -bench=. -benchmem ./strings` — see the 100x speed gap between `PlusEquals` and `StringsBuilder` on n=100.
- [ ] `go test -bench=BenchmarkPlusEquals_Scaling -benchmem ./strings` — watch `PlusEquals` blow up quadratically while `StringsBuilder` scales linearly.
- [ ] `go test ./race` — passes.
- [ ] `go test -race ./race` — **fails** with a DATA RACE report. Read the whole thing: goroutine ids, stacks, exact line.
- [ ] `go test -bench=. -cpuprofile=cpu.out ./cpu` then `go tool pprof cpu.out`.
- [ ] `go test -bench=. -benchmem -memprofile=mem.out ./heap` then `go tool pprof -alloc_space mem.out`.

---

## Task 1 — Read a real CPU profile

```powershell
go test -bench=BenchmarkSieve -cpuprofile=cpu.out ./cpu
go tool pprof cpu.out
```

At the prompt:

- [ ] `top` — top functions. `markMultiples` should be at the top by *flat* time (it's the tight inner loop).
- [ ] `top --cum` — top by *cum* time. `Sieve` climbs to the top because it calls everything.
- [ ] `list markMultiples` — see the per-line time attribution. The `composite[j] = true` line dominates.
- [ ] `list Sieve` — see time attributed to the `markMultiples` and `collectPrimes` call sites.
- [ ] `web` — SVG call graph (needs graphviz installed; skip otherwise).

Write in "What I learned": what's the difference between *flat* and *cum* time, and when do you look at each?

---

## Task 2 — Reduce the allocations in `StringConcat_Sprintf`

```powershell
go test -bench=BenchmarkStringConcat_Sprintf -benchmem -memprofile=mem.out ./heap
go tool pprof -alloc_space mem.out
(pprof) top
(pprof) list StringConcat_Sprintf
```

- [ ] The `fmt.Sprintf("%d", i)` line should be a huge share of the alloc bytes.
- [ ] Change ONLY that line to `strconv.Itoa(i)`. Rebench.
- [ ] Then change the `s +=` pattern to `strings.Builder`. Rebench.
- [ ] Compare with `benchstat`:
  ```powershell
  go test -bench=BenchmarkStringConcat_Sprintf -benchmem -count=10 ./heap > old.txt
  # apply your fixes...
  go test -bench=BenchmarkStringConcat_Sprintf -benchmem -count=10 ./heap > new.txt
  benchstat old.txt new.txt
  ```
- [ ] Expected: >90% reduction in both time and allocs. Note the `p=` value.

---

## Task 3 — When does `GrowNaive` catch up to `GrowPresized`?

- [ ] Add sub-benchmarks that vary `n` across 10, 100, 1000, 10000.
- [ ] For each, compute `ns/op / n` — the per-item cost. Does `GrowNaive` per-item cost grow with n? (Answer: it shouldn't, thanks to Go's amortised growth strategy — but the constant factor is 2–3x worse than `GrowPresized`.)
- [ ] For `n=1`, both should be ~identical. When does the gap open?

The teaching: **pre-sizing is free lunch**. `make([]T, 0, n)` when you know `n`.

---

## Task 4 — Turn the race detector on in your notes API

Go to any earlier day (e.g. Day 33 or 35) and:

- [ ] `go test -race ./...` on the whole notes API.
- [ ] Does anything fail? (Probably not — the codebase is careful.)
- [ ] Add this to your mental checklist for CI: `go test -race ./...` is the one line that catches concurrency bugs before prod does. Cost 5–10x; catch value: enormous.

---

## Task 5 — Live CPU profile on the notes API

Add the pprof endpoints to the Day 35 hardened notes API (if not already):

- [ ] Add `import _ "net/http/pprof"` in main.go.
- [ ] With chi, mount:
  ```go
  import "net/http/pprof"
  r.Get("/debug/pprof/", pprof.Index)
  r.Get("/debug/pprof/profile", pprof.Profile)
  r.Get("/debug/pprof/heap", pprof.Handler("heap").ServeHTTP)
  r.Get("/debug/pprof/goroutine", pprof.Handler("goroutine").ServeHTTP)
  ```
- [ ] Start the server. In one terminal, hammer it with load:
  ```powershell
  # crude ab-alike; Day 40 covers this properly with k6
  1..10000 | ForEach-Object {
      curl.exe -s -o $null -H "Authorization: Bearer $tok" http://localhost:8080/notes/1
  }
  ```
- [ ] While the load runs, in another terminal:
  ```powershell
  go tool pprof http://localhost:8080/debug/pprof/profile?seconds=15
  (pprof) top
  ```
- [ ] Expect: JSON encoding + Redis calls + Postgres driver near the top. The exact ordering tells you where to look for wins.

Behind a real prod gate: authenticate the pprof routes.

---

## Task 6 — Confirm `sync.Pool` from Day 36 actually helps

- [ ] Add a benchmark file to the Day 36 `cmd/pool` directory (or copy the two functions here):
  ```go
  func BenchmarkWithoutPool(b *testing.B) { ... }
  func BenchmarkWithPool(b *testing.B)    { ... }
  ```
- [ ] `go test -bench=. -benchmem ./...` from the Day 36 folder.
- [ ] Expected: `WithPool` has ~0 allocs/op after the pool warms up; `WithoutPool` has 1 alloc/op every iteration.
- [ ] `benchstat` the two runs. `sync.Pool` is one of the very few "always faster" wins on hot paths.

---

## Task 7 — Escape analysis one-liner

```powershell
go build -gcflags="-m" ./strings 2>&1 | Select-String "escapes to heap"
```

- [ ] See which locals in `strings/` escape to the heap. Every `escapes to heap` is one allocation per call.
- [ ] Try the same on `./heap`. The `s +=` chain forces every intermediate string onto the heap — that's why allocs explode.

`-gcflags="-m"` is the escape analysis diagnostic. It's noisy but readable, and it explains *why* a change reduced allocations without needing to guess.

---

## Stretch — only if you're flying

- [ ] **Flame graph via speedscope.** `go tool pprof -http=:6060 cpu.out` opens a browser UI. Try the flame graph view — same data, hugely more useful for wide call graphs.
- [ ] **`GODEBUG=allocfreetrace=1`.** Print every allocation as it happens. Only useful on tiny programs; noisy at scale. Try on a single `Sieve(1000)` call and see the alloc log.
- [ ] **`-blockprofile` and `-mutexprofile`.** Two more profile types that pprof supports: contention on channels/mutexes. Rarely needed but invaluable when you need them.
- [ ] **Continuous profiling.** Look up Parca / Pyroscope / Grafana Phlare. Same pprof format, sampled 24/7 in production, historical view. Day 90 (Prometheus) is the closest peer topic in the plan.

---

## What I learned (Day 39)

> 3 bullets in your own words.

-
-
-
