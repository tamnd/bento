package lower

import (
	"strings"
	"testing"
)

// TestSharedArrayBufferLoweringShape pins the Go a program that builds a
// SharedArrayBuffer and reads its byte length lowers to: a construction from a length
// is value.NewSharedArrayBuffer, a SharedArrayBuffer parameter is typed
// *value.SharedArrayBuffer, and a .byteLength read is a ByteLength() call, the float64
// the checker gives the property. It reads the emitted code directly so a change to the
// shape is visible in review without running the toolchain.
func TestSharedArrayBufferLoweringShape(t *testing.T) {
	const src = `function size(buf: SharedArrayBuffer): number {
  return buf.byteLength;
}
const b = new SharedArrayBuffer(8);
console.log(size(b));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewSharedArrayBuffer(",
		"*value.SharedArrayBuffer",
		"buf.ByteLength()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("SharedArrayBuffer lowering missing %q:\n%s", want, source)
		}
	}
}

// TestSharedArrayBufferGrowableLoweringShape pins the Go the growable constructor form
// lowers to: new SharedArrayBuffer(n, { maxByteLength: m }) is
// value.NewGrowableSharedArrayBuffer with the two lengths, grow is a Grow call, and the
// maxByteLength and growable reads are MaxByteLength and Growable calls on the buffer.
func TestSharedArrayBufferGrowableLoweringShape(t *testing.T) {
	const src = `const b = new SharedArrayBuffer(8, { maxByteLength: 16 });
b.grow(12);
console.log(b.maxByteLength);
console.log(b.growable);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewGrowableSharedArrayBuffer(8, 16)",
		"b.Grow(12)",
		"b.MaxByteLength()",
		"b.Growable()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("growable SharedArrayBuffer lowering missing %q:\n%s", want, source)
		}
	}
}

// TestSharedArrayBufferSliceLoweringShape pins the Go slice lowers to: a Slice call on
// the buffer carrying the lowered bounds, the fresh shared buffer the method returns.
func TestSharedArrayBufferSliceLoweringShape(t *testing.T) {
	const src = `const b = new SharedArrayBuffer(8);
const c = b.slice(2, 6);
console.log(c.byteLength);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"b.Slice(2, 6)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("SharedArrayBuffer slice lowering missing %q:\n%s", want, source)
		}
	}
}

// TestSharedArrayBufferHandsBackUnsupportedForms proves the SharedArrayBuffer lowering
// claims the forms it covers and hands the rest back. A byte length that is not a
// number, and a grow length that is not a number, both hand back rather than emitting
// wrong Go.
func TestSharedArrayBufferHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const n: any = 8; const b = new SharedArrayBuffer(n); console.log(b.byteLength);\n")
	handsBack(t, "const n: any = 4; const b = new SharedArrayBuffer(8, { maxByteLength: 16 }); b.grow(n); console.log(b.byteLength);\n")
}
