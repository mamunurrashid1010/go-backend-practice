// semaphore.Weighted — for jobs that aren't all the same size.
//
// The metaphor: think of the semaphore as a memory budget in MB.
// A 100MB job holds 100 tokens; a 1MB job holds 1. Small jobs pack
// together freely; a huge job runs mostly alone.
//
// Run: go run ./cmd/semaphore
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/semaphore"
)

type Job struct {
	Name string
	Cost int64 // "MB" for this demo — could be CPU, disk, whatever
	Work time.Duration
}

func main() {
	const capacity = 10 // total budget the semaphore hands out

	sem := semaphore.NewWeighted(capacity)

	jobs := []Job{
		{"small-a", 1, 200 * time.Millisecond},
		{"small-b", 1, 200 * time.Millisecond},
		{"small-c", 1, 200 * time.Millisecond},
		{"small-d", 1, 200 * time.Millisecond},
		{"huge-1", 8, 300 * time.Millisecond}, // needs 8 slots
		{"small-e", 1, 200 * time.Millisecond},
		{"small-f", 1, 200 * time.Millisecond},
		{"huge-2", 10, 200 * time.Millisecond}, // needs the WHOLE budget
	}

	var inflight int64
	var wg sync.WaitGroup
	start := time.Now()

	for _, j := range jobs {
		j := j
		// Acquire blocks until `Cost` tokens are free. It also respects
		// ctx (not shown here) — cancel the ctx and it returns ctx.Err().
		if err := sem.Acquire(context.Background(), j.Cost); err != nil {
			fmt.Printf("acquire failed: %v\n", err)
			return
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer sem.Release(j.Cost)

			n := atomic.AddInt64(&inflight, 1)
			defer atomic.AddInt64(&inflight, -1)

			fmt.Printf("  [+%3dms] START  %-8s cost=%2d  inflight_jobs=%d\n",
				time.Since(start).Milliseconds(), j.Name, j.Cost, n)
			time.Sleep(j.Work)
			fmt.Printf("  [+%3dms] DONE   %-8s\n", time.Since(start).Milliseconds(), j.Name)
		}()
	}
	wg.Wait()

	fmt.Printf("\nwall-clock: ~%v\n", time.Since(start).Round(10*time.Millisecond))
	fmt.Println("\nNote how huge-2 (cost=10) waits for the pool to fully drain,")
	fmt.Println("while huge-1 (cost=8) can share with at most 2 small jobs.")
}
