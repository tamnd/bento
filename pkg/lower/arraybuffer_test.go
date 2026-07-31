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

// TestArrayBufferResizableLoweringShape pins the Go the resizable constructor form
// lowers to: new ArrayBuffer(n, { maxByteLength: m }) is value.NewResizableArrayBuffer
// with the two lengths, and resize is a Resize call.
func TestArrayBufferResizableLoweringShape(t *testing.T) {
	const src = `const b = new ArrayBuffer(8, { maxByteLength: 16 });
b.resize(12);
console.log(b.byteLength);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NewResizableArrayBuffer(8, 16)",
		"b.Resize(12)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("resizable ArrayBuffer lowering missing %q:\n%s", want, source)
		}
	}
}

// TestArrayBufferResizableGetterShape pins the Go the resizable-geometry reads lower
// to: maxByteLength is a MaxByteLength call and resizable is a Resizable call, both on
// the buffer, the same accessor-to-method shape byteLength and detached take.
func TestArrayBufferResizableGetterShape(t *testing.T) {
	const src = `const b = new ArrayBuffer(8, { maxByteLength: 16 });
console.log(b.maxByteLength);
console.log(b.resizable);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"b.MaxByteLength()",
		"b.Resizable()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("resizable ArrayBuffer getter lowering missing %q:\n%s", want, source)
		}
	}
}

// TestArrayBufferHandsBackUnsupportedForms proves the ArrayBuffer lowering claims the
// forms it covers and hands the rest back. A byte length that is not a number, and a
// second argument that is not an object literal carrying maxByteLength, both hand back
// rather than emitting wrong Go.
func TestArrayBufferHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const n: any = 8; const b = new ArrayBuffer(n); console.log(b.byteLength);\n")
	handsBack(t, "const opts: any = { maxByteLength: 16 }; const b = new ArrayBuffer(8, opts); console.log(b.byteLength);\n")
}

// TestArrayBufferSliceLoweringShape pins the Go slice lowers to: a Slice call on the
// buffer carrying whichever bounds the call gave, since the runtime method reads its
// variadic bounds by length and an omitted one has to stay omitted rather than arrive
// as a NaN.
func TestArrayBufferSliceLoweringShape(t *testing.T) {
	const src = `const b = new ArrayBuffer(8);
console.log(b.slice(2, 6).byteLength);
console.log(b.slice(2).byteLength);
console.log(b.slice().byteLength);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"b.Slice(2, 6)",
		"b.Slice(2)",
		"b.Slice()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("ArrayBuffer slice lowering missing %q:\n%s", want, source)
		}
	}
}

// TestArrayBufferSliceHandsBackUnsupportedForms proves the slice lowering claims the
// forms it covers and hands the rest back. A bound that is not a number is a later
// slice, since it would need the ToNumber coercion the runtime method does not take.
func TestArrayBufferSliceHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const n: any = 2; const b = new ArrayBuffer(8); console.log(b.slice(n).byteLength);\n")
}
