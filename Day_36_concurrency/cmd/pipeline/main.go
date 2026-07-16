// Pipeline — the classic channels-shine example.
//
// generator -> square -> print. Each stage is a goroutine; values
// flow between them via channels. No shared state, no Mutex, no
// "which goroutine owns this data" — the channel is the ownership.
//
// Run: go run ./cmd/pipeline
package main

import "fmt"

// generator emits ints 1..n on out, then closes out.
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

// square receives ints, sends their squares, closes out when in closes.
// Note the direction restrictions: it can only receive from `in` and
// only send to `out`.
func square(in <-chan int) <-chan int {
	out := make(chan int)
	go func() {
		defer close(out)
		for v := range in {
			out <- v * v
		}
	}()
	return out
}

func main() {
	numbers := generator(6)
	squared := square(numbers)
	for v := range squared {
		fmt.Println(v)
	}
	// Zero Mutexes. The `range` on each stage terminates the moment
	// its input channel closes — so the whole pipeline shuts down
	// cleanly with no goroutine leaked.
}
