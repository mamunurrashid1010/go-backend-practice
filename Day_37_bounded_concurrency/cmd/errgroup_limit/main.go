// errgroup.SetLimit — the modern "bounded worker pool" in one line.
//
// Run: go run ./cmd/errgroup_limit
package main

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
)

func main() {
	const (
		limit = 4
		items = 20
	)

	// inflight tracks concurrent goroutines. We check it during each
	// job to prove SetLimit really is capping in-flight work at 4.
	var inflight, peak int64

	g, _ := errgroup.WithContext(context.Background())
	g.SetLimit(limit) // at most 4 g.Go(...) callbacks running at once

	start := time.Now()

	for i := 0; i < items; i++ {
		i := i
		// g.Go BLOCKS until a slot is free. So this loop naturally
		// paces itself: pushing 20 items with a limit of 4 doesn't
		// spawn 20 goroutines — it dribbles them out.
		g.Go(func() error {
			n := atomic.AddInt64(&inflight, 1)
			for {
				old := atomic.LoadInt64(&peak)
				if n <= old || atomic.CompareAndSwapInt64(&peak, old, n) {
					break
				}
			}
			defer atomic.AddInt64(&inflight, -1)

			time.Sleep(100 * time.Millisecond)
			fmt.Printf("  job %2d done (inflight was %d)\n", i, n)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		fmt.Printf("error: %v\n", err)
	}

	fmt.Printf("\npeak in-flight: %d (limit was %d)\n", atomic.LoadInt64(&peak), limit)
	fmt.Printf("wall-clock: ~%v (20 items / %d workers × 100ms = ~%dms)\n",
		time.Since(start).Round(10*time.Millisecond), limit, items/limit*100)

	// Try changing limit to 1 (fully serial) or items (fully parallel)
	// and rerun — the wall-clock scales linearly.
}
