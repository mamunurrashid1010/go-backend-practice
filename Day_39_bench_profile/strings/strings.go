// Three ways to build the same string. The BenchmarkX_Y functions in
// strings_test.go time all of them so you can see the difference.
package strings

import (
	"bytes"
	strs "strings"
)

// PlusEquals is the naive way. Every +=  allocates a NEW string —
// the old one is garbage. For N concatenations that's O(N²) work.
// Try it on N=1000 and watch the numbers explode.
func PlusEquals(parts []string) string {
	var s string
	for _, p := range parts {
		s += p
	}
	return s
}

// StringsBuilder is the idiomatic modern answer. Uses an internal
// []byte buffer that grows amortised O(1), then a single conversion
// to string at the end. Zero-copy at the last step.
func StringsBuilder(parts []string) string {
	var b strs.Builder
	// Pre-sizing helps a lot for known-size inputs — one allocation
	// instead of log(N) growth steps.
	total := 0
	for _, p := range parts {
		total += len(p)
	}
	b.Grow(total)

	for _, p := range parts {
		b.WriteString(p)
	}
	return b.String()
}

// BytesBuffer — older / more general. Very similar performance to
// strings.Builder but with a []byte -> string conversion at the end
// that DOES copy. Only reach for it when you also need io.Writer.
func BytesBuffer(parts []string) string {
	var buf bytes.Buffer
	for _, p := range parts {
		buf.WriteString(p)
	}
	return buf.String()
}
