package valuelab

// The mathbits workload is the tightest integer loop in the bench suite: on every
// iteration it runs Math.imul, two Math.clz32, and a Math.fround, all of which are
// 32-bit operations. The lowerer emits each of those as a float64 that is coerced
// to int32, operated on, and widened back, because a JavaScript number is a
// float64 and the value runtime keeps that model. These benchmarks isolate the
// cost of that round trip from the rest of the program so an optimization can be
// measured on the kernel alone, far faster than the end-to-end harness.
//
// benchMathbitsLowered mirrors, statement for statement, the Go the lowerer emits
// today for the mathbits inner loop (dump the workload with pkg/build.Compile to
// confirm). benchMathbitsNative is the same loop with every value already held as
// an int32, which is what integer type specialization in the lowerer produces. The
// gap between the two is the headroom the specialization is chasing. Both must
// return the same checksum, 1653160564, the value node, bun, deno, and bento all
// agree on for this workload, so the fast path is only valid while it keeps that.

import (
	"math/bits"
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

// mathbitsChecksum is the result every runtime agrees on for the mathbits
// workload. A lowering that changes it is wrong no matter how fast it runs.
const mathbitsChecksum = 1653160564

// benchMathbitsLowered runs the mathbits inner loop the way the lowerer emits it
// today: acc and i are float64, every bitwise step coerces through value.ToInt32
// or value.ToUint32, and the |0 idiom lowers to an OR with value.ToInt32(0).
func benchMathbitsLowered() int32 {
	var acc float64 = 0
	for range 100 {
		for i := float64(1); i < 5000; i++ {
			acc = value.Imul(float64(value.ToInt32(acc)^value.ToInt32(i)), 2654435761)
			acc = float64(value.ToInt32(acc+value.Clz32(i)) | value.ToInt32(0))
			f := value.Fround(i * 1.1)
			acc = float64(value.ToInt32(acc+value.Clz32(f)) | value.ToInt32(0))
		}
	}
	return value.ToInt32(acc)
}

// benchMathbitsNative runs the same loop with acc and i as int32, the shape
// integer type specialization produces: the bitwise operators are Go operators on
// the integers, imul is a wrapping int32 multiply, clz32 is bits.LeadingZeros32,
// and the |0 that only exists to coerce back to int32 disappears entirely.
func benchMathbitsNative() int32 {
	var acc int32 = 0
	for range 100 {
		for i := int32(1); i < 5000; i++ {
			acc = int32(uint32(acc^i) * 2654435761)
			acc = acc + int32(bits.LeadingZeros32(uint32(i)))
			f := float64(float32(float64(i) * 1.1))
			acc = acc + int32(bits.LeadingZeros32(value.ToUint32(f)))
		}
	}
	return acc
}

func TestMathbitsKernelsAgree(t *testing.T) {
	if got := benchMathbitsLowered(); got != mathbitsChecksum {
		t.Fatalf("lowered kernel checksum = %d, want %d", got, mathbitsChecksum)
	}
	if got := benchMathbitsNative(); got != mathbitsChecksum {
		t.Fatalf("native kernel checksum = %d, want %d", got, mathbitsChecksum)
	}
}

func BenchmarkMathbitsLowered(b *testing.B) {
	for b.Loop() {
		sinkI32 = benchMathbitsLowered()
	}
}

func BenchmarkMathbitsNative(b *testing.B) {
	for b.Loop() {
		sinkI32 = benchMathbitsNative()
	}
}
