# Day 39 — Benchmarks, `-race`, CPU + Heap Profiling

> **Goal:** write a benchmark, run it, read the numbers. Then run the same code under `-race`, `-cpuprofile`, and `-memprofile`, and use `go tool pprof` — the same triad you learned on Day 38's goroutine profile — to answer "why is this slow?" and "why is my heap growing?"
>
> **The frame:** Day 38 gave you `pprof` for goroutine leaks. Today it points at CPU and heap. The `top` / `list` / `web` skill transfers directly.

---

## 1. Writing a benchmark

Benchmarks live in `_test.go` files, alongside their tests. They're functions named `BenchmarkFoo(b *testing.B)`:

```go
func BenchmarkConcat(b *testing.B) {
    for i := 0; i < b.N; i++ {
        _ = concat("hello", "world")
    }
}
```

The Go tool decides `b.N` at runtime. It starts small (1), runs the benchmark, extrapolates how many iterations it can fit in the target duration (default 1s), then runs the *real* measurement with that `b.N`. You never pick `b.N` yourself.

If setup is expensive, run it **outside** the loop and reset the clock:

```go
func BenchmarkParse(b *testing.B) {
    data := generateBigInput()   // NOT measured
    b.ResetTimer()

    for i := 0; i < b.N; i++ {
        _ = Parse(data)
    }
}
```

For piecewise-timed benchmarks (setup between iterations), use `b.StopTimer()` / `b.StartTimer()`.

### Go 1.24's `b.Loop()`

Newer Go — same shape, no `b.N`:

```go
func BenchmarkConcat(b *testing.B) {
    for b.Loop() {
        _ = concat("hello", "world")
    }
}
```

Cleaner and prevents accidental optimisation (the compiler can't hoist work out of `b.Loop()`). If you're on Go 1.24+, use it. This repo's demos use the classic `b.N` form for portability.

---

## 2. Running benchmarks

```powershell
# All benchmarks in a package
go test -bench=. ./strings

# One benchmark by regex
go test -bench=BenchmarkPlus ./strings

# With allocation counts
go test -bench=. -benchmem ./strings

# Longer per-run duration (more stable numbers)
go test -bench=. -benchtime=3s ./strings

# Run each benchmark N times for statistical comparison
go test -bench=. -count=10 ./strings > new.txt
```

Output shape:

```
BenchmarkPlus-8       10000    123456 ns/op    98304 B/op    99 allocs/op
             │  │        │        │              │            │
             │  │        │        │              │            └─ per-op heap alloc COUNT
             │  │        │        │              └─ per-op heap alloc BYTES
             │  │        │        └─ nanoseconds per iteration
             │  │        └─ number of iterations Go decided to run
             │  └─ GOMAXPROCS at run time
             └─ benchmark name
```

The four numbers you care about, always in the same order: **iterations, ns/op, B/op, allocs/op.** The last two need `-benchmem`.

---

## 3. `benchstat` — compare two runs statistically

Two runs of the same benchmark aren't equal — CPU noise, GC pauses, thermal throttling. `benchstat` runs a t-test:

```powershell
go install golang.org/x/perf/cmd/benchstat@latest

# Baseline
go test -bench=. -count=10 -benchmem ./strings > old.txt
# Make your "improvement"...
go test -bench=. -count=10 -benchmem ./strings > new.txt

benchstat old.txt new.txt
```

Sample output:

```
             │   old.txt   │              new.txt              │
             │   sec/op    │   sec/op     vs base              │
Plus-8         123.5µ ± 2%   24.3µ ± 1%  -80.32% (p=0.000 n=10)
```

`p=0.000` means the difference is statistically significant. `n=10` is the sample count. If you're not showing a real improvement, `benchstat` will tell you `~` (no difference within noise).

**Never trust a single-run "5% speedup."** Run 10, use benchstat.

---

## 4. `-race` — the race detector

```powershell
go test -race ./race
```

Instruments every memory read/write to detect concurrent access without synchronisation. Cost: **5–10x slowdown**, ~2x memory. Never in prod, always in CI.

What it detects:

- Two goroutines reading + writing the same memory without a lock / channel handoff.
- Reports the exact goroutine IDs, stacks, and access sites.

What it *doesn't* detect:

- **Deadlocks** — Day 36's runtime panic covers the "all goroutines asleep" case; partial deadlocks show up in Day 38's `pprof goroutine`.
- **Logic bugs** — a race-free but wrong-answer function passes.
- **Rare races** the test didn't provoke — the detector reports only races that actually happened during the run.

That last one is why race-detected CI is worth its cost: your test suite is more likely to hit rare shapes than production traffic is.

Demo: [race/counter_test.go](race/counter_test.go).

---

## 5. CPU profiling — where is my time going?

```powershell
# Run a benchmark AND capture a CPU profile
go test -bench=BenchmarkExpensive -cpuprofile=cpu.out ./cpu

# Interactive
go tool pprof cpu.out
```

At the `(pprof)` prompt:

- **`top`** — top functions by CPU time. Two numbers per function:
  - **`flat`** — time spent *in this function itself*.
  - **`cum`** — time spent in this function *and everything it called*.

  A leaf function has flat ≈ cum. A high-level function has cum >> flat.

- **`list expensive`** — line-by-line source listing with time attributed to each. This is where you spot "wait, why is line 42 taking 60% of the CPU?"

- **`web`** — an SVG call graph in your browser (needs `graphviz`). Boxes sized by cum time; edges labelled with time flowing between callers and callees. Absurdly useful.

- **`top --cum`** — top by cumulative time. Shows the deep callers whose *children* are hot.

CPU profiling in Go uses **sampling** at ~100 Hz. It's cheap enough to leave on in production behind a route, and accurate enough for anything that's actually hot.

---

## 6. Heap profiling — where are my allocations?

```powershell
# Run a benchmark AND capture a heap profile
go test -bench=BenchmarkAllocs -memprofile=mem.out ./heap

# Two views — pick the right one:
go tool pprof -alloc_space mem.out    # TOTAL bytes allocated over the run
go tool pprof -alloc_objects mem.out  # TOTAL object counts
go tool pprof -inuse_space mem.out    # bytes currently held (memory leak detection)
go tool pprof -inuse_objects mem.out
```

Same `top` / `list` / `web` commands.

**`-alloc_space` vs `-inuse_space`.** Total bytes allocated tells you "where is GC pressure coming from?" — the hot allocation sites. Bytes currently held tells you "where is memory *stuck*?" — usually a cache or a leak.

Rule of thumb for hot paths: **allocations are more expensive than they look.** A single `append` that grows a slice is often three allocations (old buffer, new buffer, GC eventually collects). Reduce allocations, reduce GC pressure, latency-p99 falls dramatically.

Demo: [heap/allocations_test.go](heap/allocations_test.go).

---

## 7. Six things profiling almost always uncovers

Every Go service I've profiled has at least one of these in its top-10:

1. **String concatenation with `+` in a loop.** Every `+=` allocates a new string. Use `strings.Builder`. [strings/strings_test.go](strings/strings_test.go) benchmarks this — 100x speedup + 99% fewer allocations for concatenating 100 strings.
2. **JSON encoding without a buffer.** `json.Marshal` allocates. `json.NewEncoder(w).Encode(v)` writes straight to `w` — one syscall, no throwaway `[]byte`.
3. **`fmt.Sprintf("%d", n)`.** Six times slower than `strconv.Itoa(n)` in hot paths. Reflection isn't free.
4. **Interface boxing.** Passing an `int` into a `func(any)` heap-allocates the int. `any` in hot paths costs.
5. **Small allocations that could be pooled.** Buffers, encoders. `sync.Pool` (Day 36) reduces allocs to near zero.
6. **Reflection.** `encoding/json`, `reflect`, `text/template` all use it. Codegen (like sqlc from Day 31, or [ffjson](https://github.com/pquerna/ffjson) / [sonic](https://github.com/bytedance/sonic)) is the fix if it's on the hot path.

**But don't guess.** Profile first. The intuition wins from experience are the ones where you were *wrong* about the bottleneck.

---

## 8. Live CPU/heap on a running server

The same pprof endpoints from Day 38's `pprof_server` also serve CPU and heap:

```powershell
# 30-second CPU profile from a running server
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# Current heap snapshot
go tool pprof http://localhost:8080/debug/pprof/heap
```

`profile` blocks for `seconds` while the server samples. `heap` is a snapshot. Both drop you into the same interactive REPL. This is how you profile **production** — put pprof behind an auth-gated route in the Day 35 notes API and you're set.

---

## 9. What changed from Day 38

Nothing carries. Day 39 is the last pure-tooling day of Week 6. Day 40 turns these skills on the real notes API with `k6` load testing.

---

## 10. Run it

```powershell
cd Day_39_bench_profile
go mod init day39
go mod tidy

# Benchmarks — read the numbers, feel the shape
go test -bench=. -benchmem ./strings

# Race detector — passes without -race, fails with it
go test        ./race            # passes; prints a wrong number
go test -race  ./race            # FAILS: DATA RACE at counter.go:XX

# CPU profile
go test -bench=. -cpuprofile=cpu.out ./cpu
go tool pprof cpu.out
# (pprof) top
# (pprof) list Sieve
# (pprof) quit

# Heap profile
go test -bench=. -benchmem -memprofile=mem.out ./heap
go tool pprof -alloc_space mem.out
# (pprof) top
# (pprof) list Grow
# (pprof) quit
```

---

## 11. What's next

**Day 40 — load test with `k6` / `vegeta`.** The Day 35 hardened notes API under load. Find a real bottleneck by watching latency curves, then use today's `pprof` to explain *why* the bottleneck is where it is, then fix it, then re-benchmark to prove the fix worked.
