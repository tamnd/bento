package lower

import (
	"strings"
	"testing"
)

// TestArrayBufferLoweringShape pins the Go a program that builds an ArrayBuffer and
// reads its byte length lowers to: a construction from a length is
// value.NewArrayBuffer, an ArrayBuffer parameter is typed *value.ArrayBuffer, and a
// .byteLength read is a ByteLength() call, the float64 the checker gives the
// property. It reads the emitted code directly so a change to the shape is visible
// in review without running the toolchain.
func TestArrayBufferLoweringShape(t *testing.T) {
	const src = `function size(buf: ArrayBuffer): number {
  return buf.byteLength;
}
const b = new ArrayBuffer(8);
console.log(size(b));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewArrayBuffer(",
		"*value.ArrayBuffer",
		"buf.ByteLength()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("ArrayBuffer lowering missing %q:\n%s", want, source)
		}
	}
}

// TestArrayBufferHandsBackUnsupportedForms proves the ArrayBuffer lowering claims
// only the byte-length constructor and hands the rest back. The resizable form new
// ArrayBuffer(n, { maxByteLength }) is a later slice, and a byte length that is not
// a number is a later slice too, so both hand back rather than emitting wrong Go.
func TestArrayBufferHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const b = new ArrayBuffer(8, { maxByteLength: 16 }); console.log(b.byteLength);\n")
	handsBack(t, "const n: any = 8; const b = new ArrayBuffer(n); console.log(b.byteLength);\n")
}
