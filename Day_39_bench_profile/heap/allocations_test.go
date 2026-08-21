package heap

import "testing"

// Benchmarks — pair every "sloppy" with a "fixed" so benchstat can
// compare them directly.
//
// Run:
//   go test -bench=. -benchmem ./heap
//
// Typical output (relative shape, not absolute times):
//   BenchmarkGrowNaive-8         30000    41000 ns/op   40952 B/op   19 allocs/op
//   BenchmarkGrowPresized-8      80000    12000 ns/op    8192 B/op    1 allocs/op
//   BenchmarkStringConcat_Sprintf-8   3000  400000 ns/op  200000 B/op  400 allocs/op
//   BenchmarkStringConcat_Fast-8    100000    9500 ns/op    2048 B/op    2 allocs/op
//
// Then capture a memory profile and inspect the alloc sites:
//   go test -bench=BenchmarkStringConcat_Sprintf -benchmem -memprofile=mem.out ./heap
//   go tool pprof -alloc_space mem.out
//   (pprof) top
//   (pprof) list StringConcat_Sprintf
//   # you'll see fmt.Sprintf as the hot alloc line inside the loop

func BenchmarkGrowNaive(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GrowNaive(1000)
	}
}

func BenchmarkGrowPresized(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GrowPresized(1000)
	}
}

func BenchmarkStringConcat_Sprintf(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringConcat_Sprintf(100)
	}
}

func BenchmarkStringConcat_Fast(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = StringConcat_Fast(100)
	}
}
