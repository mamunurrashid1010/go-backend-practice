// context basics: WithCancel, WithTimeout, WithDeadline, and the
// three sentinel values ctx.Err() can return.
//
// Run: go run ./cmd/basics
package main

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 1. WithCancel — someone calls cancel() explicitly ===")
	demoCancel()

	fmt.Println("\n=== 2. WithTimeout — automatic cancel after d ===")
	demoTimeout()

	fmt.Println("\n=== 3. WithDeadline — same, but at an absolute time ===")
	demoDeadline()

	fmt.Println("\n=== 4. errors.Is with ctx.Err() ===")
	demoErrorsIs()
}

func demoCancel() {
	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel() // <-- close ctx.Done(), set ctx.Err() = context.Canceled
	}()

	<-ctx.Done()
	fmt.Printf("  ctx.Err() = %v\n", ctx.Err())
	// Always safe to call cancel twice; second call is a no-op.
	cancel()
}

func demoTimeout() {
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel() // ALWAYS defer; releases the timer even if we return early

	start := time.Now()
	<-ctx.Done()
	fmt.Printf("  fired after ~%v; err = %v\n",
		time.Since(start).Round(time.Millisecond), ctx.Err())
}

func demoDeadline() {
	deadline := time.Now().Add(80 * time.Millisecond)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()

	<-ctx.Done()
	// The Deadline() method reports what we set:
	d, ok := ctx.Deadline()
	fmt.Printf("  deadline was set: ok=%v, at %s (%v ago)\n",
		ok, d.Format("15:04:05.000"), time.Since(d).Round(time.Millisecond))
	fmt.Printf("  err = %v\n", ctx.Err())
}

func demoErrorsIs() {
	// A function that returns ctx.Err() (wrapped or not).
	do := func(ctx context.Context) error {
		<-ctx.Done()
		return fmt.Errorf("do: %w", ctx.Err()) // wraps
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()

	err := do(ctx)
	fmt.Printf("  raw err: %v\n", err)
	fmt.Printf("  errors.Is(err, context.DeadlineExceeded) = %v\n",
		errors.Is(err, context.DeadlineExceeded))
	fmt.Printf("  errors.Is(err, context.Canceled)         = %v\n",
		errors.Is(err, context.Canceled))
	// Practical use: `if errors.Is(err, context.DeadlineExceeded) { retry() }`
}
