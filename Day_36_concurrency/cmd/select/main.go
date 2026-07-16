// select — the concurrency control-flow statement.
//
// Run: go run ./cmd/select
package main

import (
	"context"
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 1. multi-case — whichever is ready first ===")
	// Two producers on two channels; select picks whichever value
	// arrives first. Order of cases doesn't matter.
	a, b := make(chan string, 1), make(chan string, 1)
	go func() { time.Sleep(50 * time.Millisecond); a <- "from A" }()
	go func() { time.Sleep(100 * time.Millisecond); b <- "from B" }()
	select {
	case v := <-a:
		fmt.Printf("  got: %s\n", v)
	case v := <-b:
		fmt.Printf("  got: %s\n", v)
	}
	// The other channel still has a value queued; someone should drain
	// it or the goroutine that WOULD have sent it later leaks.
	fmt.Printf("  runner-up: %s\n", <-b)

	fmt.Println("\n=== 2. default — non-blocking select ===")
	// Try to receive; if nothing's ready RIGHT NOW, fall through to
	// default. Great for "peek without blocking."
	ch := make(chan int)
	select {
	case v := <-ch:
		fmt.Printf("  got %d\n", v)
	default:
		fmt.Println("  nothing ready (default branch)")
	}

	fmt.Println("\n=== 3. time.After — the per-select timeout ===")
	slow := make(chan string)
	go func() { time.Sleep(300 * time.Millisecond); slow <- "eventually" }()
	select {
	case v := <-slow:
		fmt.Printf("  got %q\n", v)
	case <-time.After(100 * time.Millisecond):
		fmt.Println("  timed out waiting (100ms limit)")
	}
	// NOTE: time.After leaks a timer until it fires. In a tight loop,
	// prefer:
	//   t := time.NewTimer(d); defer t.Stop(); ...
	//   case <-t.C: ...

	fmt.Println("\n=== 4. ctx.Done() — cancellation propagation ===")
	// The idiomatic way a goroutine listens for "stop what you're doing."
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	work(ctx)
}

func work(ctx context.Context) {
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			fmt.Printf("  cancelled: %v\n", ctx.Err())
			return
		case t := <-tick.C:
			fmt.Printf("  tick @ %s\n", t.Format("15:04:05.000"))
		}
	}
}
