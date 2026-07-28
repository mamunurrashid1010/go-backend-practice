// Worker pool — the classic hand-built pattern.
//
// N long-lived workers pull from a shared job channel. Producer sends
// onto jobs and closes when done. WaitGroup counts the workers.
//
// You still hand-roll this when: workers are stateful (per-worker DB
// conn, buffer, etc.), workers live across many batches, or you need
// a specific topology. For a fire-and-forget batch, errgroup.SetLimit
// is now the right tool.
//
// Run: go run ./cmd/workerpool
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// Job — trivial payload; imagine anything.
type Job struct {
	ID int
}

// worker consumes jobs until the channel is closed, then returns.
// The stateful thing here is `id` — the worker's own name. In real
// code this might be a per-worker DB connection, HTTP client, etc.
func worker(ctx context.Context, id int, jobs <-chan Job, wg *sync.WaitGroup) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			// Cooperative cancel — bail out even if there are jobs
			// still queued. Without this the worker keeps running
			// until the channel closes.
			fmt.Printf("  worker %d: cancelled (%v)\n", id, ctx.Err())
			return
		case job, ok := <-jobs:
			if !ok {
				// Channel closed AND drained — normal exit path.
				fmt.Printf("  worker %d: exiting (no more jobs)\n", id)
				return
			}
			time.Sleep(80 * time.Millisecond)
			fmt.Printf("  worker %d: processed job %d\n", id, job.ID)
		}
	}
}

func main() {
	const (
		workers = 3
		jobsN   = 10
	)

	ctx := context.Background()
	jobs := make(chan Job)

	var wg sync.WaitGroup
	for i := 1; i <= workers; i++ {
		wg.Add(1)
		go worker(ctx, i, jobs, &wg)
	}

	// Producer. In real code this often lives elsewhere (an HTTP
	// handler, a message consumer). It's responsible for closing
	// the channel when it's done.
	for i := 0; i < jobsN; i++ {
		jobs <- Job{ID: i}
	}
	close(jobs)

	wg.Wait()
	fmt.Println("\nall workers stopped")

	// The worker pool is now dismantled. If the caller wants to
	// process another batch, they need a new `jobs` chan and to
	// restart the workers — that's why long-lived pools usually
	// keep the workers running and hand them a "shutdown" signal
	// (context) instead of closing the channel.
}
