package lower

import (
	"strings"
	"testing"
)

// new Number(x), new String(x), and new Boolean(x) build primitive wrapper objects:
// typeof "object", always truthy, that coerce back to the wrapped primitive. The
// renderer lowers each to the value.NumberObject/StringObject/BooleanObject runtime
// constructor with the argument boxed. Before this slice new of any of these handed the
// unit back at the generic constructor reason.

// TestNewNumberWrapperLowersToRuntimeCtor pins the lowering: new Number(x) reaches
// value.NumberObject rather than handing back.
func TestNewNumberWrapperLowersToRuntimeCtor(t *testing.T) {
	src := "const n = new Number(5); console.log(typeof n);\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NumberObject(") {
		t.Fatalf("new Number did not lower to value.NumberObject:\n%s", out)
	}
}

// TestPrimWrapperTypeofRuns builds and runs the wrappers against the Node oracle: each
// wrapper is typeof "object" and a Boolean wrapper of false is still truthy.
func TestPrimWrapperTypeofRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const n = new Number(5);
const s = new String("hi");
const b = new Boolean(false);
console.log(typeof n);
console.log(typeof s);
console.log(typeof b);
console.log(b ? "truthy" : "falsy");
`
	got := runProgramGo(t, src)
	want := "object\nobject\nobject\ntruthy\n"
	if got != want {
		t.Fatalf("primitive wrapper typeof run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestPrimWrapperCoercionRuns pins the coercion short-circuit: a wrapper coerces back
// to the wrapped primitive through String, arithmetic, and parseFloat.
func TestPrimWrapperCoercionRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const n = new Number(1.5);
const s = new String("abc");
console.log(+n + 1);
console.log(parseFloat(new Number(3.14)));
console.log(s.length);
`
	got := runProgramGoTolerant(t, src)
	want := "2.5\n3.14\n3\n"
	if got != want {
		t.Fatalf("primitive wrapper coercion run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestPrimWrapperValueOfRuns pins the prototype methods: a wrapper's valueOf returns
// the wrapped primitive (not the object), so a strict equality against the primitive
// holds, and toString renders it. This is the shape the test262 wrapper cases assert
// through, new Boolean(1).valueOf() === true, that a wrong valueOf identity fails.
func TestPrimWrapperValueOfRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(new Boolean(1).valueOf() === true);
console.log(new Number(255).valueOf() === 255);
console.log(new Number(255).toString() === "255");
console.log(new String("hi").valueOf() === "hi");
`
	got := runProgramGoTolerant(t, src)
	want := "true\ntrue\ntrue\ntrue\n"
	if got != want {
		t.Fatalf("primitive wrapper valueOf run mismatch:\n got %q\nwant %q", got, want)
	}
}
