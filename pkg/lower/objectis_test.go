package lower

import (
	"strings"
	"testing"
)

// TestObjectIsNumberEmitsSameValue pins that Object.is over two numbers lowers to
// value.NumberSameValue.
func TestObjectIsNumberEmitsSameValue(t *testing.T) {
	src := "const a = 1;\nconst b = 1;\nconsole.log(Object.is(a, b));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberSameValue(a, b)") {
		t.Errorf("Object.is over numbers did not lower to NumberSameValue:\n%s", source)
	}
}

// TestObjectIsStringEmitsEqual pins that Object.is over two strings lowers to the
// BStr.Equal comparison strict equality already uses.
func TestObjectIsStringEmitsEqual(t *testing.T) {
	src := "const a = \"x\";\nconst b = \"y\";\nconsole.log(Object.is(a, b));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Equal(b)") {
		t.Errorf("Object.is over strings did not lower to Equal:\n%s", source)
	}
}

// TestObjectIsMixedHandsBack pins that Object.is over operands of different types,
// which folds to a constant false and would drop both reads, is a later slice.
func TestObjectIsMixedHandsBack(t *testing.T) {
	src := "const a = 1;\nconst b = \"x\";\nconsole.log(Object.is(a, b));\n"
	renderProgramHandBack(t, src)
}

// TestObjectIsRuns builds and runs Object.is against the Node oracle, covering
// the NaN and signed-zero cases where SameValue parts from strict equality. The
// NaN comes from Math.sqrt(-1) and the negative zero from negating a runtime
// zero, since a bare NaN global does not lower and a -0 literal would fold to +0
// as a Go constant, both of which would hide the very cases under test.
func TestObjectIsRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const nan = Math.sqrt(-1);
let z = 0;
const nz = -z;
console.log(Object.is(1, 1));
console.log(Object.is(1, 2));
console.log(Object.is(nan, nan));
console.log(Object.is(z, nz));
console.log(Object.is(nz, nz));
console.log(Object.is("hi", "hi"));
console.log(Object.is("hi", "ho"));
console.log(Object.is(true, true));
console.log(Object.is(true, false));
`
	got := runProgramGo(t, src)
	want := "true\nfalse\ntrue\nfalse\ntrue\ntrue\nfalse\ntrue\nfalse\n"
	if got != want {
		t.Fatalf("Object.is program printed %q, want %q", got, want)
	}
}
