package valuelab

// The number-to-string radix conversion and the string-to-number parse are the
// primitives the coercion workload leans on: String(n), n.toString(16), and
// Number(s). These benchmarks time each one in isolation so a change to the radix
// buffer, the parse fast path, or the whitespace trim shows up here before it
// reaches the whole workload. Each loop reads the counter so the call cannot be
// folded away, and each writes a package-level sink so the result is observed.

import (
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

var (
	sinkStr   value.BStr
	sinkFloat float64
)

// BenchmarkNumberToStringRadixSmall times toString(16) over the small integers the
// coercion workload feeds it, the case that dominated the radix path when its
// buffer was heap-allocated on every call.
func BenchmarkNumberToStringRadixSmall(b *testing.B) {
	var s value.BStr
	i := 0
	for b.Loop() {
		s = value.NumberToStringRadix(float64(i&0xff), 16)
		i++
	}
	sinkStr = s
}

// BenchmarkNumberToStringRadixFraction times toString(16) on a value with a
// fractional part, which exercises the fraction loop the small-integer case skips,
// so a buffer regression that only the fraction path touches still shows up.
func BenchmarkNumberToStringRadixFraction(b *testing.B) {
	var s value.BStr
	i := 1
	for b.Loop() {
		s = value.NumberToStringRadix(float64(i)+0.5, 16)
		i++
	}
	sinkStr = s
}

// BenchmarkStringToNumberDecimal times Number(s) on a plain decimal string, the
// grammar-check then strconv.ParseFloat path, with no whitespace to trim so the
// trim shortcut is on its fast branch.
func BenchmarkStringToNumberDecimal(b *testing.B) {
	inputs := make([]value.BStr, 256)
	for i := range inputs {
		inputs[i] = value.NumberToString(float64(i)*1.5 - 3)
	}
	var acc float64
	i := 0
	for b.Loop() {
		acc += value.StringToNumber(inputs[i&255])
		i++
	}
	sinkFloat = acc
}

// BenchmarkStringToNumberHex times Number("0x..") over small hex values, the parse
// that ran a big.Int and a big.Float on every call before the uint64 fast path.
func BenchmarkStringToNumberHex(b *testing.B) {
	prefix := value.FromGoString("0x")
	inputs := make([]value.BStr, 256)
	for i := range inputs {
		inputs[i] = value.Concat(prefix, value.NumberToStringRadix(float64(i), 16))
	}
	var acc float64
	i := 0
	for b.Loop() {
		acc += value.StringToNumber(inputs[i&255])
		i++
	}
	sinkFloat = acc
}

// BenchmarkStringToNumberPadded times Number(s) on a value wrapped in whitespace,
// so the trim path is measured rather than skipped. The receiver is valid UTF-8, so
// this is the Go-string trim that replaced the code-unit materialization.
func BenchmarkStringToNumberPadded(b *testing.B) {
	inputs := make([]value.BStr, 256)
	for i := range inputs {
		inputs[i] = value.FromGoString("  " + value.NumberToString(float64(i)).ToGoString() + "  ")
	}
	var acc float64
	i := 0
	for b.Loop() {
		acc += value.StringToNumber(inputs[i&255])
		i++
	}
	sinkFloat = acc
}
