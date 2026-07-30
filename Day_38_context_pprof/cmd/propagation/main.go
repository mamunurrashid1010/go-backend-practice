// Propagation: cancelling a parent context cancels every descendant.
// A child's shorter deadline wins; a child's longer deadline is
// silently overridden by the parent.
//
// Run: go run ./cmd/propagation
package main

import (
	"context"
	"fmt"
	"sync"
	"time"
)

func main() {
	fmt.Println("=== 1. cancel parent → all children fire ===")
	demoParentCancelsChildren()

	fmt.Println("\n=== 2. child cancel does NOT affect parent ===")
	demoChildCancelIsolated()

	fmt.Println("\n=== 3. the shorter deadline wins ===")
	demoShorterDeadlineWins()
}

func demoParentCancelsChildren() {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Two nested children, each waiting on their own ctx.
	childA, _ := context.WithCancel(parent)
	childB, _ := context.WithTimeout(parent, 10*time.Second) // deliberately long

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-childA.Done()
		fmt.Printf("  childA fired: err=%v\n", childA.Err())
	}()
	go func() {
		defer wg.Done()
		<-childB.Done()
		fmt.Printf("  childB fired: err=%v\n", childB.Err())
	}()

	time.Sleep(50 * time.Millisecond)
	fmt.Println("  cancelling parent...")
	cancel() // → both childA.Done and childB.Done close, both err() = context.Canceled

	wg.Wait()
}

func demoChildCancelIsolated() {
	parent, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	child, childCancel := context.WithCancel(parent)

	childCancel() // cancel only the child

	fmt.Printf("  child.Err()  = %v (cancelled)\n", child.Err())
	fmt.Printf("  parent.Err() = %v (still nil)\n", parent.Err())
}

func demoShorterDeadlineWins() {
	parent, cancel1 := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel1()

	// Ask for a longer deadline on the child. Doesn't matter — parent
	// cancels first.
	child, cancel2 := context.WithTimeout(parent, 5*time.Second)
	defer cancel2()

	start := time.Now()
	<-child.Done()
	fmt.Printf("  child fired after ~%v (asked for 5s; got parent's 50ms)\n",
		time.Since(start).Round(time.Millisecond))
	fmt.Printf("  child.Err() = %v\n", child.Err())
}
