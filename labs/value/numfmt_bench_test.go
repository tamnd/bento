package valuelab

// String(x) on a double is the whole of the numfmt workload and a large part of
// templates and coercion. These benchmarks time NumberToString on the shapes the
// formatter takes different paths for: the integer fast path, an ordinary
// fraction, a value in exponential notation, and a sub-unit magnitude. The first
// benchmark replays numfmt's own mixed distribution so a change shows up against
// the same spread the workload sees; the shape-specific ones isolate a single
// branch so a regression that only one path touches still surfaces. Each loop
// reads the counter so the call cannot be folded away and writes a package sink.

import (
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

var sinkLen int

// BenchmarkNumberToStringNumfmt replays the numfmt workload's distribution: an
// ordinary fraction, a large value that can cross into exponential, a small one
// that can cross the other way, and a negative. It is the micro-level mirror of
// the compute/numfmt benchmark.
func BenchmarkNumberToStringNumfmt(b *testing.B) {
	xs := make([]float64, 0, 3000*4)
	for i := 1; i < 3000; i++ {
		x := float64(i) * 1.000001
		xs = append(xs, x, x*1e18, x/1e12, -x)
	}
	var acc int
	i := 0
	for b.Loop() {
		acc += int(value.NumberToString(xs[i]).Length())
		i++
		if i == len(xs) {
			i = 0
		}
	}
	sinkLen = acc
}

// BenchmarkNumberToStringInteger times the whole-number fast path, the branch
// that skips the shortest-digit search and formats an int64 directly.
func BenchmarkNumberToStringInteger(b *testing.B) {
	var acc int
	i := 0
	for b.Loop() {
		acc += int(value.NumberToString(float64(i & 0xffff)).Length())
		i++
	}
	sinkLen = acc
}

// BenchmarkNumberToStringFraction times an ordinary non-integer that formats in
// plain decimal notation with a point inside the digits.
func BenchmarkNumberToStringFraction(b *testing.B) {
	var acc int
	i := 1
	for b.Loop() {
		acc += int(value.NumberToString(float64(i) * 1.000001).Length())
		i++
	}
	sinkLen = acc
}

// BenchmarkNumberToStringExponential times a magnitude past 1e21, the branch
// that writes exponential notation with an explicit-sign, unpadded exponent.
func BenchmarkNumberToStringExponential(b *testing.B) {
	var acc int
	i := 1
	for b.Loop() {
		acc += int(value.NumberToString(float64(i) * 1.000001e21).Length())
		i++
	}
	sinkLen = acc
}

// BenchmarkNumberToStringSmall times a magnitude below 1e-6, the branch that
// writes "0." then leading zeros then the digits.
func BenchmarkNumberToStringSmall(b *testing.B) {
	var acc int
	i := 1
	for b.Loop() {
		acc += int(value.NumberToString(float64(i) * 1.000001e-9).Length())
		i++
	}
	sinkLen = acc
}
