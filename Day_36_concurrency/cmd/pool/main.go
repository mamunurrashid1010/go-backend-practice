// sync.Pool — reuse short-lived objects to reduce GC pressure.
//
// Run:
//   go run ./cmd/pool                   # print alloc counts
//   go test -bench=. -benchmem ./cmd/pool ...
//     (see pool_bench_test.go for the real head-to-head)
package main

import (
	"bytes"
	"fmt"
	"runtime"
	"sync"
)

// bufPool — the New function is called only when Get finds the pool
// empty. Between Gets, freed buffers may be returned to the caller
// OR discarded by the GC — Pool is a hint, not a store.
var bufPool = sync.Pool{
	New: func() any { return new(bytes.Buffer) },
}

func withPool() {
	buf := bufPool.Get().(*bytes.Buffer)
	buf.Reset() // ALWAYS reset before use — the pool can return a dirty one.
	buf.WriteString("hello ")
	buf.WriteString("world")
	// Put it back so someone else can reuse it.
	bufPool.Put(buf)
}

func withoutPool() {
	buf := new(bytes.Buffer)
	buf.WriteString("hello ")
	buf.WriteString("world")
	_ = buf
}

func measure(label string, fn func(), iters int) {
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	for i := 0; i < iters; i++ {
		fn()
	}
	runtime.ReadMemStats(&m1)
	// TotalAlloc is monotonic; the delta is bytes allocated during
	// the run. Mallocs is the count of allocation events.
	fmt.Printf("%-16s bytes=%-10d mallocs=%d\n",
		label, m1.TotalAlloc-m0.TotalAlloc, m1.Mallocs-m0.Mallocs)
}

func main() {
	const iters = 100_000
	measure("no pool", withoutPool, iters)
	measure("with pool", withPool, iters)

	// The pooled version allocates dramatically less. Under high
	// concurrency the difference is even bigger — the pool has per-P
	// caches so contention is minimal.
}
