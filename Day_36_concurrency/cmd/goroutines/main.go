// Goroutines + sync.WaitGroup — the two basics for "run N things in parallel."
//
// Run: go run ./cmd/goroutines
package main

import (
	"fmt"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== 1. bare goroutine ===")
	// This fires-and-forgets. The main goroutine might exit before
	// this ever prints — try commenting out the sleep to see it happen.
	go func() {
		fmt.Println("  hello from a goroutine")
	}()
	time.Sleep(100 * time.Millisecond)

	fmt.Println("\n=== 2. loop-variable capture (Go 1.22+ handles this) ===")
	// In Go 1.22+, each iteration gets its own i so these all print
	// distinct values. In Go 1.21 and earlier you'd see 5 fives.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fmt.Printf("  loopvar i=%d\n", i)
		}()
	}
	wg.Wait()

	fmt.Println("\n=== 3. explicit param — works on ANY Go version ===")
	// The bulletproof form: pass i as a parameter. Do this if you
	// need code that ports across old Go versions.
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			fmt.Printf("  param  i=%d\n", i)
		}(i)
	}
	wg.Wait()

	fmt.Println("\n=== 4. WaitGroup — wait for N to finish ===")
	// The canonical pattern: Add BEFORE go, Done via defer inside.
	// Never Add inside the goroutine — the parent may hit Wait first
	// and race the Add.
	start := time.Now()
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			time.Sleep(200 * time.Millisecond)
			fmt.Printf("  worker %d done\n", i)
		}(i)
	}
	wg.Wait()
	fmt.Printf("  all done in ~%v (would be 600ms sequential)\n", time.Since(start).Round(time.Millisecond))
}
