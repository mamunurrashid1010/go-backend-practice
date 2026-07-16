// Channels — unbuffered, buffered, close + range, direction restriction.
//
// Run: go run ./cmd/channels
package main

import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("=== 1. unbuffered — a synchronization point ===")
	// Send blocks until receive. Both goroutines meet at the exact
	// moment of the handoff.
	ch := make(chan string)
	go func() {
		fmt.Println("  sender: about to send")
		ch <- "hi"
		fmt.Println("  sender: sent (receiver has taken the value)")
	}()
	time.Sleep(50 * time.Millisecond) // let the sender print first
	v := <-ch
	fmt.Printf("  main: got %q\n", v)

	fmt.Println("\n=== 2. buffered — a fixed-size queue ===")
	// Capacity 2. First 2 sends don't block. Third would (no receiver
	// yet), so we don't do it.
	bch := make(chan int, 2)
	bch <- 1
	bch <- 2
	fmt.Println("  sent 1 and 2 without blocking (buffer has room)")
	fmt.Printf("  got %d, %d\n", <-bch, <-bch)

	fmt.Println("\n=== 3. close + range — drain until closed ===")
	nums := make(chan int)
	go func() {
		defer close(nums) // ONLY the sender closes. See README §2.
		for i := 1; i <= 4; i++ {
			nums <- i
		}
	}()
	// range terminates when the channel is closed AND drained.
	for n := range nums {
		fmt.Printf("  got %d\n", n)
	}

	fmt.Println("\n=== 4. the two-value receive form ===")
	// Detect "channel closed and drained" without a range loop.
	done := make(chan struct{})
	close(done)
	v2, ok := <-done
	fmt.Printf("  <-done -> value=%v ok=%v (ok=false means drained)\n", v2, ok)

	fmt.Println("\n=== 5. directional channels — enforced by the compiler ===")
	// The producer function CAN'T receive. The consumer CAN'T send.
	// Try changing one and rebuilding — the compiler stops you.
	c := make(chan int)
	go produce(c)
	consume(c)
}

func produce(out chan<- int) {
	defer close(out)
	for i := 0; i < 3; i++ {
		out <- i * i
	}
}

func consume(in <-chan int) {
	for v := range in {
		fmt.Printf("  consumed %d\n", v)
	}
}
