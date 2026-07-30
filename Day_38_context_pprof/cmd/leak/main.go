// The goroutine leak, in isolation, out of any HTTP context.
//
// leakingFetch demonstrates the classic "timeout leak" — an unbuffered
// channel + a goroutine that will block forever if ctx fires first.
// runtime.NumGoroutine before/after shows the accumulation.
//
// fixedFetch differs by ONE character: make(chan Result) becomes
// make(chan Result, 1).
//
// Run: go run ./cmd/leak
package main

import (
	"context"
	"fmt"
	"runtime"
	"time"
)

type Result struct{ V int }

// leakingFetch — an antipattern to fix in Task 1.
// If ctx wins the select race, the goroutine's `ch <- ...` blocks
// forever because no one is reading from ch anymore.
func leakingFetch(ctx context.Context) (Result, error) {
	ch := make(chan Result) // UNBUFFERED — the bug

	go func() {
		time.Sleep(50 * time.Millisecond) // "the real work"
		ch <- Result{V: 42}               // if ctx fired, blocks forever
	}()

	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		return Result{}, ctx.Err() // caller returns; goroutine leaks
	}
}

// fixedFetch — the one-character fix.
// A capacity-1 buffered channel means the goroutine can always send
// its result and exit, even if we already returned via ctx.
func fixedFetch(ctx context.Context) (Result, error) {
	ch := make(chan Result, 1) // BUFFERED — the fix

	go func() {
		time.Sleep(50 * time.Millisecond)
		ch <- Result{V: 42} // guaranteed non-blocking
	}()

	select {
	case v := <-ch:
		return v, nil
	case <-ctx.Done():
		return Result{}, ctx.Err()
	}
}

func main() {
	fmt.Printf("baseline goroutines: %d\n\n", runtime.NumGoroutine())

	fmt.Println("=== calling leakingFetch 100 times with an already-cancelled ctx ===")
	// An already-cancelled ctx → the select ALWAYS takes the ctx branch,
	// so the goroutine ALWAYS blocks on send.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for i := 0; i < 100; i++ {
		_, _ = leakingFetch(ctx)
	}

	// Give scheduler a moment to run any finalizers.
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("after 100 leakingFetch: %d goroutines (was 2..3)\n\n", runtime.NumGoroutine())

	fmt.Println("=== calling fixedFetch 100 times with an already-cancelled ctx ===")
	for i := 0; i < 100; i++ {
		_, _ = fixedFetch(ctx)
	}
	time.Sleep(200 * time.Millisecond)
	fmt.Printf("after 100 fixedFetch:   %d goroutines (flat)\n", runtime.NumGoroutine())

	fmt.Println("\nSame code shape; one character difference. Real production leaks look like this.")
}
