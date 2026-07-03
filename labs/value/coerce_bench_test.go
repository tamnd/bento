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

// BenchmarkCoercionBody runs the whole coercion inner-loop body one iteration per
// b.Loop() the way the workload's golden does: format n, parse it back, build and
// parse the "0x" hex, and fold the two truthiness tests. It is the aggregate the
// per-primitive benchmarks above decompose, so a profile of this benchmark shows
// where the workload's compute time actually lands.
func BenchmarkCoercionBody(b *testing.B) {
	prefix := value.FromGoString("0x")
	var acc, truthy float64
	i := 0
	pass := 0
	for b.Loop() {
		n := float64(i)*1.5 - float64(pass)
		s := value.NumberToString(n)
		acc += value.StringToNumber(s)
		acc += value.StringToNumber(value.Concat(prefix, value.NumberToStringRadix(float64(i&0xff), 16)))
		if value.NumberToBool(n) && value.StringToBool(s) {
			truthy++
		}
		i++
		if i == 2000 {
			i = 0
			pass++
		}
	}
	sinkFloat = acc + truthy
}
