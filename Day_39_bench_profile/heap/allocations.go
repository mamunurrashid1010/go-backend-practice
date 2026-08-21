// Two versions of the same job — one allocates like a chimney, one
// doesn't. The benchmarks in allocations_test.go time both; a
// -memprofile lets you see the alloc sites in the sloppy version.
package heap

import (
	"fmt"
	"strconv"
)

// GrowNaive appends N ints to a slice starting from a nil zero slice.
// Go's slice-growth strategy means many reallocations as len crosses
// each capacity threshold (0 -> 1 -> 2 -> 4 -> 8 -> ... ~2N total bytes).
// Every growth event allocates a new backing array and copies the old
// one over.
func GrowNaive(n int) []int {
	var out []int
	for i := 0; i < n; i++ {
		out = append(out, i)
	}
	return out
}

// GrowPresized does the same with a known-size make(). ONE allocation
// for the underlying array, zero realloc churn. Same output.
func GrowPresized(n int) []int {
	out := make([]int, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, i)
	}
	return out
}

// StringConcat_Sprintf — a common perf trap. Every += allocates a
// new string, and fmt.Sprintf itself allocates via reflection. In a
// loop this is O(n²) allocations.
//
// This will show up in the heap profile as the top alloc site.
func StringConcat_Sprintf(n int) string {
	s := ""
	for i := 0; i < n; i++ {
		if i > 0 {
			s += "-"
		}
		s += fmt.Sprintf("%d", i) // <-- the hot alloc line
	}
	return s
}

// StringConcat_Fast — same output. strconv.AppendInt writes into an
// existing byte slice (no reflection). Pre-sized buffer avoids
// reallocation. One final string() conversion at the end.
func StringConcat_Fast(n int) string {
	buf := make([]byte, 0, n*4) // rough upper bound
	for i := 0; i < n; i++ {
		if i > 0 {
			buf = append(buf, '-')
		}
		buf = strconv.AppendInt(buf, int64(i), 10)
	}
	return string(buf)
}
