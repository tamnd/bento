package valuelab

// String.fromCharCode building a short string is the whole of the fromcharcode
// workload. The construction allocates, and this benchmark measures how much: the
// variadic argument slice, the code-unit slice, and any defensive copy the
// constructor makes. The workload then reads the result with charCodeAt and
// length, so the second benchmark builds and reads the way the workload does, to
// keep the two allocation costs visible together.

import (
	"testing"

	"github.com/tamnd/bento/pkg/value"
)

// BenchmarkFromCharCodeBuild times String.fromCharCode of four code units, the
// arity and the mix the fromcharcode workload uses: two ASCII letters, a value
// that ToUint16 wraps, and a Greek letter that is one BMP unit but not ASCII.
func BenchmarkFromCharCodeBuild(b *testing.B) {
	var s value.BStr
	i := 0
	for b.Loop() {
		s = value.FromCharCode(
			float64(65+(i&25)),
			float64(97+(i%26)),
			float64(65536+(i&127)),
			float64(945+(i%24)),
		)
		i++
	}
	sinkStr = s
}

// BenchmarkFromCharCodeBuildRead mirrors the workload body: build the string,
// then fold charCodeAt(0), charCodeAt(2), and length into a checksum, so the
// cost of reading a freshly built string is measured next to building it.
func BenchmarkFromCharCodeBuildRead(b *testing.B) {
	acc := 0
	i := 0
	for b.Loop() {
		s := value.FromCharCode(
			float64(65+(i&25)),
			float64(97+(i%26)),
			float64(65536+(i&127)),
			float64(945+(i%24)),
		)
		acc = acc*31 + int(s.CharCodeAt(0))
		acc = acc*31 + int(s.CharCodeAt(2))
		acc = acc*31 + int(s.Length())
		i++
	}
	sinkLen = acc
}
