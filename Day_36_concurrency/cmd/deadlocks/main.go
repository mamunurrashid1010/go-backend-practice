// Deadlocks — the runtime detects "all goroutines are asleep" and panics.
//
// Run: go run ./cmd/deadlocks
package main

import "fmt"

func main() {
	// Uncomment ONE of the demos to see the panic:
	//   fatal error: all goroutines are asleep - deadlock!
	//
	// Only one at a time — the first uncommented one crashes and no
	// later code runs.

	// deadlockUnbufferedNoReceiver()
	// deadlockBufferFull()

	fmt.Println("=== fixed version — a receiver appears ===")
	fixed()
}

// deadlockUnbufferedNoReceiver — the classic "send with no receiver
// on an unbuffered channel." The main goroutine blocks on send forever.
// The runtime notices nothing else is runnable and panics.
func deadlockUnbufferedNoReceiver() { //nolint:unused
	ch := make(chan int)
	ch <- 1 // BLOCKS: no receiver anywhere. Panic.
	_ = ch
}

// deadlockBufferFull — buffered channel, but the buffer is full and
// no one is reading. Third send blocks. Same panic.
func deadlockBufferFull() { //nolint:unused
	ch := make(chan int, 2)
	ch <- 1
	ch <- 2
	ch <- 3 // BLOCKS: buffer at capacity, no receiver. Panic.
	_ = ch
}

// fixed — a receiver goroutine is running, so the send hands off and
// life goes on. No panic.
func fixed() {
	ch := make(chan int)
	go func() {
		v := <-ch
		fmt.Printf("  receiver got %d\n", v)
	}()
	ch <- 42
	fmt.Println("  sender sent; done.")
}
