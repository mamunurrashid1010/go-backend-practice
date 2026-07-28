// Fan-out / fan-in — 1 producer → N workers → 1 collector.
//
//	                  ┌── square ──┐
//	generator ──▶ jobs│── square ──│─▶ merge ──▶ main
//	                  └── square ──┘
//
// Every arrow is a channel. Workers do the same work in parallel
// (fan-out); merge combines their outputs into one stream (fan-in).
//
// Run: go run ./cmd/fanout_fanin
package main

import (
	"fmt"
	"sync"
)

// generator emits ints 1..n, then closes.
func generator(n int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for i := 1; i <= n; i++ {
			out <- i
		}
	}()
	return out
}

// square is the fan-out worker. Many instances share one input channel,
// each writes squares to its OWN output channel.
func square(id int, in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			out <- v * v
			// (in a real workload each worker would be doing something
			// more interesting; imagine an HTTP call or a DB lookup)
			_ = id
		}
	}()
	return out
}

// Merge is the fan-in primitive. Reads from every input concurrently
// and forwards onto one output. Closes output when ALL inputs close.
func Merge[T any](chans ...<-chan T) <-chan T {
	out := make(chan T)
	var wg sync.WaitGroup
	wg.Add(len(chans))

	// One relay goroutine per input.
	for _, ch := range chans {
		ch := ch
		go func() {
			defer wg.Done()
			for v := range ch {
				out <- v
			}
		}()
	}

	// One coordinator goroutine that closes the output when every
	// relay has finished draining its input.
	go func() {
		wg.Wait()
		close(out)
	}()

	return out
}

func main() {
	const (
		items   = 20
		workers = 4
	)

	nums := generator(items)

	// Fan-out: 4 workers all range over the same `nums` channel.
	// Go's channel semantics guarantee each value is received by
	// exactly one worker — no coordination code needed.
	outs := make([]<-chan int, workers)
	for i := 0; i < workers; i++ {
		outs[i] = square(i, nums)
	}

	// Fan-in: one stream of results.
	results := Merge(outs...)

	sum := 0
	for v := range results {
		sum += v
	}
	fmt.Printf("sum of squares 1..%d = %d\n", items, sum)
	// (Sanity: sum_{i=1..20} i² = 2870)

	// Key point: no shared state, no Mutex. Ownership of each int
	// moves along the channels. When generator's `nums` closes, the
	// workers' ranges terminate; when workers close their outputs,
	// Merge's relays terminate; when relays are done, Merge closes
	// `results` and main's range exits. Clean shutdown falls out for
	// free.
}
