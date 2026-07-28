// errgroup — "WaitGroup that carries an error" + first-error-cancels-siblings.
//
// The canonical modern Go pattern for "run N things in parallel; fail
// fast if any one of them fails."
//
// Run: go run ./cmd/errgroup
package main

import (
	"context"
	"errors"
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"
)

// fetch simulates a network call. url == "bad" fails after 100ms.
// url == "slow" takes 1s but always succeeds — unless the ctx cancels
// first, which our errgroup will do the moment "bad" fires.
func fetch(ctx context.Context, url string) (string, error) {
	switch url {
	case "bad":
		select {
		case <-time.After(100 * time.Millisecond):
			return "", errors.New("boom: " + url)
		case <-ctx.Done():
			return "", ctx.Err()
		}
	case "slow":
		select {
		case <-time.After(1 * time.Second):
			return "OK " + url, nil
		case <-ctx.Done():
			return "", ctx.Err() // cooperative cancellation
		}
	default:
		select {
		case <-time.After(50 * time.Millisecond):
			return "OK " + url, nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}

func main() {
	fmt.Println("=== errgroup.WithContext — one bad url cancels the others ===")
	start := time.Now()

	g, ctx := errgroup.WithContext(context.Background())

	for _, url := range []string{"good1", "slow", "bad", "good2"} {
		url := url // still write the shadow — bulletproof on old Go too
		g.Go(func() error {
			s, err := fetch(ctx, url)
			if err != nil {
				fmt.Printf("  %-6s -> ERROR: %v\n", url, err)
				return err
			}
			fmt.Printf("  %-6s -> %s\n", url, s)
			return nil
		})
	}

	// Wait returns the FIRST non-nil error. Later errors from siblings
	// (typically ctx.Err() because the group cancelled them) are lost.
	err := g.Wait()
	fmt.Printf("\ng.Wait() -> %v\n", err)
	fmt.Printf("wall-clock: ~%v (would be ~1s if 'slow' wasn't cancelled)\n",
		time.Since(start).Round(10*time.Millisecond))

	// Note: the returned err is the *original* "boom: bad", not a
	// context.Canceled — errgroup remembers the winning error.
}
