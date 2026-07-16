// Mutex / RWMutex — protecting shared state.
//
// Run:
//   go run ./cmd/mutex race    # the racy version — try with -race
//   go run ./cmd/mutex safe    # the same but with sync.Mutex
//   go run ./cmd/mutex rw      # sync.RWMutex: many readers, one writer
package main

import (
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: go run ./cmd/mutex race|safe|rw")
		return
	}
	switch os.Args[1] {
	case "race":
		racyCounter()
	case "safe":
		safeCounter()
	case "rw":
		rwmutexDemo()
	default:
		fmt.Println("unknown subcommand:", os.Args[1])
	}
}

// racyCounter — 1000 goroutines each increment a shared int 1000 times.
// The expected final value is 1_000_000; you'll see something lower
// because increments race. Run with `-race` to see the detector fire.
func racyCounter() {
	var counter int
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				counter++ // <-- data race: read + write not atomic
			}
		}()
	}
	wg.Wait()
	fmt.Printf("racy counter: %d (want 1000000)\n", counter)
}

// safeCounter — same shape, but the shared int is protected by a Mutex.
// Exactly 1000000 every run.
func safeCounter() {
	var counter int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				mu.Lock()
				counter++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	fmt.Printf("safe counter (Mutex): %d\n", counter)

	// Aside: for a single-int counter, sync/atomic is faster than a
	// Mutex and equally correct. Use it when the state is one number.
	var atomicCounter int64
	wg = sync.WaitGroup{}
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				atomic.AddInt64(&atomicCounter, 1)
			}
		}()
	}
	wg.Wait()
	fmt.Printf("safe counter (atomic): %d\n", atomicCounter)
}

// rwmutexDemo — many readers concurrent, writers exclusive.
// Not always faster than Mutex — the RWMutex has more bookkeeping, so
// only use it when the read/write imbalance is real AND the critical
// sections are non-trivial.
func rwmutexDemo() {
	type cache struct {
		mu   sync.RWMutex
		data map[string]int
	}
	c := &cache{data: map[string]int{"a": 1, "b": 2}}

	read := func(key string) int {
		c.mu.RLock() // many readers concurrent
		defer c.mu.RUnlock()
		time.Sleep(50 * time.Millisecond)
		return c.data[key]
	}
	write := func(key string, v int) {
		c.mu.Lock() // exclusive
		defer c.mu.Unlock()
		time.Sleep(50 * time.Millisecond)
		c.data[key] = v
	}

	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) { defer wg.Done(); fmt.Printf("  reader %d saw a=%d\n", i, read("a")) }(i)
	}
	wg.Wait()
	fmt.Printf("  5 concurrent readers in ~%v (sequential would be ~250ms)\n", time.Since(start).Round(time.Millisecond))

	start = time.Now()
	write("a", 42)
	fmt.Printf("  1 writer took ~%v; a=%d\n", time.Since(start).Round(time.Millisecond), read("a"))
}
