package cpu

import "testing"

// BenchmarkSieve — the target for CPU profiling.
//
// Run:
//   go test -bench=. -cpuprofile=cpu.out ./cpu
//   go tool pprof cpu.out
//
// Then at the (pprof) prompt:
//   top          — top functions by CPU (both flat and cum)
//   list Sieve   — source of Sieve with per-line time attribution
//   list markMultiples  — should be the flat-time winner
//   web          — SVG call graph in browser (needs graphviz)
//
// You'll see markMultiples dominating flat time (the tight inner loop
// that flips bytes) and Sieve dominating cum time (it calls all the
// others). This flat/cum contrast is the whole reading skill.

func BenchmarkSieve(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = Sieve(1_000_000)
	}
}

// Two smaller variants so a sub-benchmark run shows the scaling curve:
//   go test -bench=BenchmarkSieve_ ./cpu
func BenchmarkSieve_100k(b *testing.B)   { for i := 0; i < b.N; i++ { _ = Sieve(100_000) } }
func BenchmarkSieve_1M(b *testing.B)     { for i := 0; i < b.N; i++ { _ = Sieve(1_000_000) } }
func BenchmarkSieve_10M(b *testing.B)    { for i := 0; i < b.N; i++ { _ = Sieve(10_000_000) } }
