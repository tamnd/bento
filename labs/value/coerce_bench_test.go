package valuelab

// The integer coercions and the three bit-exact Math methods are the primitives
// every integer workload leans on. These benchmarks time them one call at a time
// so a change to ToInt32, ToUint32, Imul, or Clz32 shows up here before it reaches
// a whole workload. Each loop reads the loop counter so the compiler cannot fold
// the call away, and each writes a package-level sink so the result is observed.

import (
	"math/bits"
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

// sinkI32, sinkU32, and sinkF64 keep benchmark results live so the compiler does
// not delete the work being measured.
var (
	sinkI32 int32
	sinkU32 uint32
	sinkF64 float64
)

func BenchmarkToInt32(b *testing.B) {
	var acc int32
	n := 0
	for b.Loop() {
		acc += value.ToInt32(float64(n) * 1.5)
		n++
	}
	sinkI32 = acc
}

func BenchmarkToUint32(b *testing.B) {
	var acc uint32
	n := 0
	for b.Loop() {
		acc += value.ToUint32(float64(n) * 1.5)
		n++
	}
	sinkU32 = acc
}

func BenchmarkImul(b *testing.B) {
	var acc float64
	n := 0
	for b.Loop() {
		acc = value.Imul(acc, float64(n|1))
		n++
	}
	sinkF64 = acc
}

func BenchmarkClz32(b *testing.B) {
	var acc float64
	n := 0
	for b.Loop() {
		acc += value.Clz32(float64(n | 1))
		n++
	}
	sinkF64 = acc
}

// BenchmarkClz32Native is the native counterpart to BenchmarkClz32: it shows the
// floor cost of the leading-zero count once the ToUint32 coercion and the float64
// widen are gone, which is what the specialized lowering reaches.
func BenchmarkClz32Native(b *testing.B) {
	var acc int32
	n := 0
	for b.Loop() {
		acc += int32(bits.LeadingZeros32(uint32(n) | 1))
		n++
	}
	sinkI32 = acc
}
