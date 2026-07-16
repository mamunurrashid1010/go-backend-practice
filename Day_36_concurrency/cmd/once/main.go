// sync.Once — run once, wait for it if you're second in line.
//
// Run: go run ./cmd/once
package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

var (
	once   sync.Once
	initCt int64 // how many times the init function actually ran
	value  string
)

func initExpensive() {
	atomic.AddInt64(&initCt, 1)
	time.Sleep(200 * time.Millisecond) // simulate work
	value = "singleton"
}

func Get() string {
	// Every caller enters here. Only ONE runs initExpensive. Late
	// callers block until the winner is done, then all see the same
	// initialized state.
	once.Do(initExpensive)
	return value
}

func main() {
	fmt.Println("=== 20 goroutines call Get() at once ===")
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			v := Get()
			fmt.Printf("  goroutine %2d got %q\n", i, v)
		}(i)
	}
	wg.Wait()
	fmt.Printf("\ninitExpensive ran %d time(s)\n", atomic.LoadInt64(&initCt))
	fmt.Printf("wall-clock: ~%v (would be ~4s serialized if Once didn't share)\n",
		time.Since(start).Round(time.Millisecond))
}
