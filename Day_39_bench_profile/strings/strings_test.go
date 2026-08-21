package strings

import (
	"strings"
	"testing"
)

// A quick sanity test — every builder should produce the same string.
// Run with `go test ./strings` (no -bench needed for tests).
func TestBuildersAgree(t *testing.T) {
	parts := []string{"a", "b", "c", "d"}
	want := "abcd"
	for name, fn := range map[string]func([]string) string{
		"PlusEquals":     PlusEquals,
		"StringsBuilder": StringsBuilder,
		"BytesBuffer":    BytesBuffer,
	} {
		if got := fn(parts); got != want {
			t.Errorf("%s: want %q, got %q", name, want, got)
		}
	}
}

// makeParts — shared setup for every benchmark. We want the SAME
// input across runs so numbers are comparable.
func makeParts(n int) []string {
	parts := make([]string, n)
	for i := range parts {
		parts[i] = strings.Repeat("x", 8)
	}
	return parts
}

// The three benchmarks — same input, three concatenators.
// Run: go test -bench=. -benchmem ./strings
//
// You should see (typical):
//   BenchmarkPlusEquals-8         500     3400000 ns/op  200000 B/op  99 allocs/op
//   BenchmarkStringsBuilder-8   50000       25000 ns/op     896 B/op   1 allocs/op
//   BenchmarkBytesBuffer-8      45000       28000 ns/op    1808 B/op   4 allocs/op
//
// The Builder version is ~100x faster and does 99% fewer allocations
// on n=100. The gap widens dramatically as n grows because PlusEquals
// is O(n²) — try n=1000 and watch.

func BenchmarkPlusEquals(b *testing.B) {
	parts := makeParts(100)
	b.ResetTimer()          // don't count setup
	b.ReportAllocs()        // print B/op + allocs/op even without -benchmem
	for i := 0; i < b.N; i++ {
		_ = PlusEquals(parts)
	}
}

func BenchmarkStringsBuilder(b *testing.B) {
	parts := makeParts(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringsBuilder(parts)
	}
}

func BenchmarkBytesBuffer(b *testing.B) {
	parts := makeParts(100)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = BytesBuffer(parts)
	}
}

// Sub-benchmarks — same benchmark, different input sizes. Great for
// seeing how algorithms scale.
// Run: go test -bench=BenchmarkPlusEquals_Scaling -benchmem ./strings
func BenchmarkPlusEquals_Scaling(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		n := n
		b.Run(itoa(n), func(b *testing.B) {
			parts := makeParts(n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = PlusEquals(parts)
			}
		})
	}
}

// Same shape but for the Builder. Doubling n roughly doubles the time,
// vs PlusEquals which QUADRUPLES.
func BenchmarkStringsBuilder_Scaling(b *testing.B) {
	for _, n := range []int{10, 100, 1000} {
		n := n
		b.Run(itoa(n), func(b *testing.B) {
			parts := makeParts(n)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = StringsBuilder(parts)
			}
		})
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
