package race

import (
	"sync"
	"testing"
)

// TestCounter — passes under `go test` (usually prints a wrong number
// silently), FAILS under `go test -race` with a full data-race report
// showing exactly which goroutines conflicted at which line.
//
// The magic:
//   go test        ./race    # PASSES  (n is small, race may or may not corrupt visibly)
//   go test -race  ./race    # FAILS   ("WARNING: DATA RACE ... at counter.go:NN")
//
// This is why race-detector-enabled CI matters: tests that "pass"
// can still be shipping race bugs.
func TestCounter(t *testing.T) {
	c := &Counter{}

	const (
		workers   = 100
		incsEach  = 1000
		wantTotal = workers * incsEach
	)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < incsEach; j++ {
				c.Inc() // <-- data race: reads and writes c.n without a lock
			}
		}()
	}
	wg.Wait()

	// Without a race detector this "passes" — but c.N() is usually
	// less than wantTotal because increments are lost.
	if c.N() != wantTotal {
		t.Logf("counter drift: got %d, want %d — likely a race", c.N(), wantTotal)
		// Log, not Fail — the test's *purpose* is to be flagged by
		// -race, not by the assertion. Keeping it a log lets you see
		// the drift without failing bare `go test` runs.
	}
}
