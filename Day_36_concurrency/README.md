# Day 36 — Go Concurrency

> **Goal:** load the concurrency primitives *cold*. Goroutines, channels, `select`, the two flavors of deadlock, `sync.Mutex` / `RWMutex` and when to reach for them vs channels, `sync.Once`, `sync.Pool`. No HTTP surface today — small runnable demos, each in its own `cmd/`.
>
> **The one-line frame:** concurrency is a *design* — many things at once. Parallelism is an *execution* — many things at literally the same instant. Go gives you concurrency primitives; the runtime figures out the parallelism.

Everything here has one goal: when you read `go func() { ch <- x }()` in Week 9's webhook retries or Week 10's Kafka consumer, none of the ceremony is new. This is the payment.

---

## 1. Goroutines

```go
go func() {
    fmt.Println("hi")
}()
```

A goroutine is a function on a **separately scheduled stack**. Cheap — 2 KiB initial stack (Linux threads: 8 MiB). The scheduler multiplexes goroutines onto OS threads (`GOMAXPROCS` of them, typically = CPU count).

Three things people get wrong on day one:

**1. `go` doesn't return anything.** No return value, no error, no way to observe when it's done. You need a channel or a `WaitGroup` for that.

**2. The loop-variable capture bug (pre Go 1.22).**

```go
// Bug in Go 1.21 and earlier: all goroutines print the same value.
for i := 0; i < 5; i++ {
    go func() { fmt.Println(i) }()
}
```

Fixed in Go 1.22+ — each iteration gets its own `i`. But you'll see legacy code that copies into a local:

```go
for i := 0; i < 5; i++ {
    i := i                   // <-- shadow the loop var
    go func() { fmt.Println(i) }()
}
```

Or the explicit parameter form (still correct on any Go version):

```go
for i := 0; i < 5; i++ {
    go func(i int) { fmt.Println(i) }(i)
}
```

**3. Goroutine leaks.** A goroutine that blocks forever on a channel that no one else touches is a leak. Nothing frees it. Under load, you leak memory + goroutines forever until OOM. Day 38 covers finding these with `pprof`.

Demo: [cmd/goroutines/main.go](cmd/goroutines/main.go). Also covers `sync.WaitGroup` — the standard "wait for N goroutines to finish" primitive.

---

## 2. Channels

A channel is a **typed pipe** with copy-on-send semantics. Values go in, values come out, in the order they were sent, one receiver per value.

```go
ch := make(chan int)         // unbuffered
ch := make(chan int, 4)      // buffered, capacity 4
ch <- 42                     // send
v := <-ch                    // receive
v, ok := <-ch                // receive; ok=false if closed AND drained
close(ch)                    // no more sends allowed
for v := range ch { ... }    // receive until closed AND drained
```

### Unbuffered — the meeting point

An unbuffered channel is a **synchronization primitive**, not a buffer. Send blocks until a receiver appears; receive blocks until a sender does. Two goroutines *hand off* the value at exactly one moment.

Use case: when the sender needs to know the receiver got it.

### Buffered — the queue

A buffered channel is a fixed-size queue. Send doesn't block until it's full. Receive doesn't block until it's empty.

Use case: smoothing a bursty producer, decoupling producer/consumer rates.

**Rule of thumb.** Start unbuffered. Only add buffer when you have a specific reason — otherwise buffer sizes get pulled from the air and every code review argues about the number.

### Direction

Any function that takes a channel should declare the direction it uses:

```go
func send(ch chan<- int)     // this function only sends
func recv(ch <-chan int)     // this function only receives
```

Callers pass an unrestricted `chan int` — the compiler downgrades the type. This turns "who is responsible for closing this" from a comment into a type error.

### The close rule

**Only the sender closes.** Receiving on a closed channel gives the zero value + `ok == false`. Sending on a closed channel *panics*. So closing is a signal: "the sender says no more values are coming."

If multiple goroutines send on the same channel, you need coordination (`WaitGroup` deciding when they're all done, then one caller closes). Never close from a receiver.

Demo: [cmd/channels/main.go](cmd/channels/main.go).

---

## 3. `select`

```go
select {
case v := <-ch1:      // receive from ch1
    use(v)
case ch2 <- x:        // send to ch2
    // sent
case <-time.After(1 * time.Second):
    // timeout
case <-ctx.Done():
    // cancelled
default:
    // NON-BLOCKING: taken only if none of the above are ready
}
```

`select` is *the* concurrency control-flow statement. Every real concurrent Go program uses it.

Things worth internalizing:

- **All cases are evaluated simultaneously**; if multiple are ready, Go picks one at pseudorandom (this is a fairness feature — don't rely on order).
- **`default` makes `select` non-blocking** — great for "try to send but drop if full."
- **`case <-time.After(d)`** is the canonical per-select timeout. Cheap in a select but the timer leaks if you don't drain — use `time.NewTimer` + `Stop` in hot loops.
- **`case <-ctx.Done()`** is how you propagate cancellation into a goroutine.

Demo: [cmd/select/main.go](cmd/select/main.go).

---

## 4. Deadlocks

Go's runtime detects one specific deadlock: **all goroutines are asleep**. It panics:

```
fatal error: all goroutines are asleep - deadlock!
```

The most common causes:

- **Sending to an unbuffered channel with no receiver ever.**
- **Receiving from a channel that will never see a send.**
- **Nested Mutex acquisition in different orders across goroutines** (classic ABBA).

The runtime *can't* detect partial deadlocks — if goroutines A and B are stuck but C is spinning, no panic. Day 38 shows how to find those with `pprof goroutine`.

Demo: [cmd/deadlocks/main.go](cmd/deadlocks/main.go) — a deliberate deadlock (commented out; uncomment to see the panic) and the fixed version.

---

## 5. `sync.Mutex` and `sync.RWMutex`

```go
var mu sync.Mutex
mu.Lock()
defer mu.Unlock()
// critical section
```

A `Mutex` gives *one* goroutine access to a critical section at a time. Simple; hard to misuse if you remember `defer Unlock` after every `Lock`.

`sync.RWMutex` distinguishes read-locks from write-locks:

- `RLock` / `RUnlock` — many holders concurrently.
- `Lock` / `Unlock` — exclusive; blocks new readers AND writers.

Reach for `RWMutex` **only** when you can prove read-locks are dramatically more common than writes AND the critical section is non-trivial. For a `map[string]int` guarded by short accesses, a plain `Mutex` is faster (the RWMutex bookkeeping isn't free).

### The classic Rob Pike rule

> Do not communicate by sharing memory; instead, share memory by communicating.

Translation: **before you reach for a Mutex, ask if a channel would do.**

- **Shared counter, hot path?** Mutex. Or `sync/atomic` if it's a single number.
- **A pipeline of stages?** Channels.
- **Ownership handoff between goroutines?** Channels — the value moves.
- **Producer / consumer with variable rates?** Buffered channel.
- **Cache with a map inside?** Mutex.

The heuristic: **Mutex for state, channels for flow.** Most real Go programs use both.

Demos: [cmd/mutex/main.go](cmd/mutex/main.go) shows the race + fix (run with `-race`); [cmd/pipeline/main.go](cmd/pipeline/main.go) shows a channels-based pipeline where no Mutex appears.

---

## 6. `sync.Once`

```go
var once sync.Once
var singleton *T

func Get() *T {
    once.Do(func() { singleton = &T{...} })
    return singleton
}
```

Guarantees `func` runs *exactly once*, even under concurrent calls, and every caller waits for that one run to finish before proceeding.

Naive version:

```go
if singleton == nil {          // race!
    singleton = &T{...}         // and race here!
}
```

Two goroutines both see nil, both allocate, one wins the assignment. You now have two `T`s and threw one away. `sync.Once` fixes this cheaply.

Demo: [cmd/once/main.go](cmd/once/main.go).

---

## 7. `sync.Pool`

A `sync.Pool` is a **GC-friendly reuse cache** for short-lived objects.

```go
var bufPool = sync.Pool{
    New: func() any { return new(bytes.Buffer) },
}

buf := bufPool.Get().(*bytes.Buffer)
defer func() { buf.Reset(); bufPool.Put(buf) }()
```

Use it when:

- Objects are expensive to allocate but easy to reset.
- You're allocating them on a hot path.
- Their lifetimes are short.

Don't use it as a global registry — the pool is **not** a cache with retention guarantees. The GC can drop everything in a pool between calls. It's a hint, not a store.

Demo: [cmd/pool/main.go](cmd/pool/main.go) — with allocation counts before/after.

---

## 8. What changed from Day 35

Nothing carries. Day 36 is a foundations day; no HTTP, no Postgres, no Redis. Just the runnable demos. Day 37 goes back to building on top: `errgroup`, `semaphore`, worker pools.

---

## 9. Run it

```powershell
cd Day_36_concurrency
go mod init day36
go mod tidy

# Each concept is its own runnable target:
go run ./cmd/goroutines
go run ./cmd/channels
go run ./cmd/select
go run ./cmd/deadlocks
go run ./cmd/mutex
go run ./cmd/pipeline
go run ./cmd/once
go run ./cmd/pool

# The race detector — always run it against concurrent code you're
# not 100% sure about:
go run -race ./cmd/mutex race
```

Every `main.go` is 40–100 lines. Read one, run it, tweak it, read the next.

---

## 10. What's next

**Day 37 — `errgroup`, `semaphore`, worker pools, fan-out / fan-in.** Same primitives, plus the coordination libraries from `golang.org/x/sync` that make them usable for real work. Then Day 38 does `context` and goroutine-leak hunting; Day 39, benchmarks + `-race` + `pprof`; Day 40, load testing; Day 41, `golangci-lint`; Day 42, GitHub Actions CI as the Week 6 closer.
