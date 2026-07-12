package lower

import (
	"strings"
	"testing"
)

// TestUint8ArrayLoweringShape pins the Go a Uint8Array program lowers to: a
// construction from a length is value.NewUint8Array, a construction from a number
// list is value.Uint8ArrayOf, an indexed read is At, an indexed write is SetAt,
// .length is Len, and a local that holds a buffer is typed *value.Uint8Array. These
// are the pieces the end-to-end equivalence cases exercise against a real engine;
// this test reads the emitted code directly so a change to the shape is visible in
// review without running the toolchain.
func TestUint8ArrayLoweringShape(t *testing.T) {
	const src = `const buf = new Uint8Array(3);
buf[0] = 65;
buf[1] = buf[0] + 1;
const lit = new Uint8Array([9, 8, 7]);
console.log(buf.length);
console.log(buf[1]);
console.log(lit.length);
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"buf := value.NewUint8Array(",
		"lit := value.Uint8ArrayOf(",
		"buf.SetAt(",
		"buf.At(",
		"buf.Len()",
		"lit.Len()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("Uint8Array lowering missing %q:\n%s", want, source)
		}
	}
}

// TestUint8ArrayConstructionForms pins the two covered constructor forms in
// isolation: a length allocates a zeroed buffer, a number list fills one. The list
// form passes each element straight through, so a value outside the byte range
// rides to the runtime's ToUint8 rather than being folded here.
func TestUint8ArrayConstructionForms(t *testing.T) {
	cases := map[string]string{
		`const b = new Uint8Array(8); console.log(b.length);`:         "value.NewUint8Array(",
		`const b = new Uint8Array([1, 2, 3]); console.log(b.length);`: "value.Uint8ArrayOf(",
		`const b = new Uint8Array([256, -1]); console.log(b.length);`: "value.Uint8ArrayOf(",
	}
	for src, want := range cases {
		source := renderProgram(t, src+"\n")
		if !strings.Contains(source, want) {
			t.Errorf("%q did not lower to %q:\n%s", src, want, source)
		}
	}
}

// TestUint8ArrayHandsBackUnsupportedForms proves the byte-buffer lowering claims
// only the subset it can emit soundly and hands the rest back to the engine. A
// compound element write reads and writes the element, which the plain SetAt store
// does not model, so it must not be mistaken for a simple write. A method that has
// no lowering yet (subarray builds a view, fill writes a run) is a later slice, so it
// routes to the interpreter rather than emitting wrong or partial Go. The
// view-over-a-buffer constructor overload does lower, so it is covered by
// TestTypedArrayViewOverBufferLowering rather than asserted here.
func TestUint8ArrayHandsBackUnsupportedForms(t *testing.T) {
	handsBack(t, "const b = new Uint8Array(3); b[0] += 1; console.log(b[0]);\n")
	handsBack(t, "const b = new Uint8Array(3); const c = b.subarray(1); console.log(c.length);\n")
	handsBack(t, "const b = new Uint8Array(3); b.fill(0); console.log(b[0]);\n")
}
