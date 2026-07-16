# Day 36 — Practice Tasks

The demos are the *before*. These tasks are the *after* — where the primitives get tested against realistic small problems.

> **Before you start:**
>
> ```powershell
> cd Day_36_concurrency
> go mod init day36
> go mod tidy
> ```

---

## Warm-up — run everything

- [ ] `go run ./cmd/goroutines` — three snippets on how to launch and wait for goroutines.
- [ ] `go run ./cmd/channels` — the five channel forms.
- [ ] `go run ./cmd/select` — timeout + cancel patterns.
- [ ] `go run ./cmd/deadlocks` — uncomment ONE of the deadlock demos, `go run` again, watch the runtime panic.
- [ ] `go run ./cmd/mutex race`, then `go run -race ./cmd/mutex race`, then `go run ./cmd/mutex safe` and `go run ./cmd/mutex rw`.
- [ ] `go run ./cmd/pipeline`
- [ ] `go run ./cmd/once`
- [ ] `go run ./cmd/pool` — read the alloc numbers.

---

## Task 1 — Race detector on ordinary code

The `-race` flag is not a debug tool — it's a QA tool you should run against every test suite in CI. Prove it works.

- [ ] Write a tiny test file `cmd/mutex/race_test.go`:
  ```go
  package main

  import "testing"

  func TestRacyCounter(t *testing.T) {
      racyCounter()
  }
  ```
- [ ] `go test ./cmd/mutex` — passes (probably prints a wrong number).
- [ ] `go test -race ./cmd/mutex` — the race detector reports the bug at the exact line.

The race detector adds ~5–10x overhead so it's not for prod, but it's essentially free in CI.

---

## Task 2 — Fix the loop-variable bug in older Go

- [ ] Check your Go version: `go version`. If 1.22+, `i := i` and the plain form work identically. If < 1.22, only the shadowed / parameter forms are safe.
- [ ] Temporarily change the goroutines demo to launch 10 goroutines that each print `i` — first without shadowing/param, then with. On Go 1.22+ both work; on older Go the naive form prints "10" ten times.

Even on Go 1.22, the parameter form is defensible — it makes the closure explicit and doesn't rely on a language-version quirk.

---

## Task 3 — A bounded worker pool (channels only)

Rewrite the WaitGroup demo as a bounded pool: N workers pull work from a `chan Job`, exit when the channel closes.

- [ ] Define `type Job func()`.
- [ ] `jobs := make(chan Job)` — unbuffered.
- [ ] Spawn 4 workers, each `range jobs` and calls each `Job`.
- [ ] Send 20 jobs — each sleeps 100ms and prints its id.
- [ ] `close(jobs)` when done, `wg.Wait()`.
- [ ] Total wall-clock should be ~500ms (20 jobs / 4 workers × 100ms), not 2s.

Day 37 formalises this pattern; today you're just building it by hand.

---

## Task 4 — Timeout ANY function call

- [ ] Write a helper:
  ```go
  func WithTimeout(ctx context.Context, d time.Duration, fn func() (string, error)) (string, error) { ... }
  ```
- [ ] It runs `fn` in a goroutine; returns whichever fires first:
  - the goroutine's result,
  - `ctx.Done()`,
  - `time.After(d)`.
- [ ] Test with a 1s sleep + a 500ms timeout — expect the timeout branch.

Note the *leak*: if `fn` outruns the timeout, its goroutine is still running when your helper returns. Sending the result on a **buffered channel** (capacity 1) lets the goroutine send-and-exit without blocking. Try both and see the leak in `go test -race` with a goroutine count check.

---

## Task 5 — Producer that closes the shared channel safely

Two producers, one shared channel. Each producer sends 5 values, then wants to signal "I'm done." Who closes?

- [ ] Wrong: either producer closes → the *other* one panics with "send on closed channel."
- [ ] Right: a `sync.WaitGroup` counts the producers; a **third** goroutine `wg.Wait()`s and then `close()`s.
- [ ] Implement this and run it 1000 times in a loop. If your implementation is wrong, one of the runs will crash.

---

## Task 6 — RWMutex vs Mutex under load

Benchmark the difference between `sync.Mutex` and `sync.RWMutex` for a read-heavy map.

- [ ] `map[string]int` guarded by each.
- [ ] `func Get(k string) int`, `func Set(k string, v int)`.
- [ ] Benchmark: 99% Get, 1% Set, hot on 8 goroutines. Measure ns/op for both.
- [ ] Now try 50/50. Does RWMutex still win?

For short critical sections and mixed workloads, a plain Mutex often beats RWMutex because the bookkeeping isn't free. Rule of thumb: only reach for RWMutex when reads are the overwhelming majority AND the critical section is long enough to matter.

---

## Stretch — only if you're flying

- [ ] **Fan-in.** Write `Merge(chans ...<-chan int) <-chan int` that reads from all input channels concurrently and emits values on one output. Close the output when all inputs close. (Hint: WaitGroup + one goroutine per input.)
- [ ] **Semaphore-lite.** Use a buffered channel of `chan struct{}` (capacity N) as a semaphore: `sem <- struct{}{}` acquires, `<-sem` releases. Bound concurrent HTTP fetches with it. Day 37 shows the `x/sync` package's real semaphore; this exercise is to feel that the primitives you already have suffice.
- [ ] **`sync.Pool` under real load.** Add a `pool_test.go` in `cmd/pool` with `Benchmark`s for both `withPool` and `withoutPool`, run with `-benchmem`, compare `allocs/op`.
- [ ] **The double-close panic.** In a small program, close the same channel twice. Confirm the panic. Reflect: how would you architect around it? (Answer: only one goroutine owns "close.")

---

## What I learned (Day 36)

> 3 bullets in your own words.

-
-
-
