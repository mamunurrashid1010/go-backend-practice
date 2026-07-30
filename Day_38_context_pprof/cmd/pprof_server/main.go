// HTTP server with a leaking endpoint and net/http/pprof mounted.
// Use it to feel the leak from the outside — hit /leak N times, then
// read /debug/pprof/goroutine?debug=1 and watch the count grow.
//
// Run: go run ./cmd/pprof_server
//
// Then in a second shell:
//
//   curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | head -1
//   # "goroutine profile: total 4"   <-- baseline
//
//   for i in {1..20}; do curl -s -o /dev/null http://localhost:8080/leak; done
//
//   curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | head -20
//   # "goroutine profile: total 24"
//   # 20 @ ...
//   # #  0x...  main.leakingHandler.func1+0x...   .../pprof_server/main.go:NN
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	_ "net/http/pprof" // side-effect: registers /debug/pprof/* on http.DefaultServeMux
	"runtime"
	"time"
)

// leakingHandler — the antipattern from cmd/leak, wrapped in HTTP.
// Every request spawns a goroutine that blocks forever on an unbuffered
// send; the request itself returns immediately because we don't wait
// for the result.
func leakingHandler(w http.ResponseWriter, r *http.Request) {
	ch := make(chan int) // unbuffered
	go func() {
		time.Sleep(1 * time.Second)
		ch <- 42 // <-- nobody is reading; blocks forever
	}()

	// Return immediately. The goroutine outlives the request.
	fmt.Fprintf(w, "leaked one goroutine. Total: %d\n", runtime.NumGoroutine())
}

// fixedHandler — same shape, but the channel is buffered so the
// background goroutine can always send-and-exit.
func fixedHandler(w http.ResponseWriter, r *http.Request) {
	ch := make(chan int, 1) // buffered — the fix
	go func() {
		time.Sleep(1 * time.Second)
		ch <- 42 // guaranteed non-blocking
	}()
	fmt.Fprintf(w, "no leak. Total: %d\n", runtime.NumGoroutine())
}

// respectingHandler — an alternative fix using ctx. The goroutine
// participates in cancellation and stops early if the request is done.
func respectingHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	ch := make(chan int, 1)
	go func() {
		select {
		case <-time.After(1 * time.Second):
			ch <- 42
		case <-ctx.Done():
			// Bail out early — leave the buffered ch alone
			return
		}
	}()

	select {
	case v := <-ch:
		fmt.Fprintf(w, "got %d. Total goroutines: %d\n", v, runtime.NumGoroutine())
	case <-ctx.Done():
		fmt.Fprintf(w, "cancelled: %v\n", ctx.Err())
	}
}

func main() {
	mux := http.DefaultServeMux // pprof registered onto this

	mux.HandleFunc("/leak", leakingHandler)
	mux.HandleFunc("/fixed", fixedHandler)
	mux.HandleFunc("/respecting", respectingHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, `Day 38 pprof playground.

Endpoints:
  GET /leak         — spawns a goroutine that blocks forever
  GET /fixed        — same shape, no leak (buffered chan)
  GET /respecting   — same shape, respects ctx cancellation
  GET /debug/pprof/ — pprof index (from net/http/pprof)

Try:
  curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | head -5
  for i in {1..20}; do curl -s -o /dev/null http://localhost:8080/leak; done
  curl "http://localhost:8080/debug/pprof/goroutine?debug=1" | head -20

Runtime goroutines right now: %d
`, runtime.NumGoroutine())
	})

	log.Println("listening on http://localhost:8080")
	log.Println("pprof index at http://localhost:8080/debug/pprof/")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal(err)
	}
}
