package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestConcatNonPrimitiveEmits pins that a string concatenation with a non-primitive
// literal operand coerces it through value.ToString: the object or array literal
// boxes into a live value and the value model runs ToPrimitive then ToString, the
// same protocol the + operator uses on an object.
func TestConcatNonPrimitiveEmits(t *testing.T) {
	const src = "const s = \"x\" + { a: 1 };\nconsole.log(s);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ToString(") {
		t.Errorf("non-primitive concat did not route through value.ToString:\n%s", source)
	}
}

// TestConcatNonPrimitiveRuns builds and runs a concatenation with an object literal
// and an array literal on each side and matches Node: an object stringifies to the
// [object Object] tag and an array joins its elements with commas.
func TestConcatNonPrimitiveRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log("x" + { a: 1 });
console.log("v=" + [1, 2, 3]);
console.log([10, 20] + "!");
`
	got := runProgramGo(t, src)
	want := "x[object Object]\nv=1,2,3\n10,20!\n"
	if got != want {
		t.Fatalf("non-primitive concat program printed %q, want %q", got, want)
	}
}

// TestConcatStructVarHandsBack pins the remaining boundary: a non-primitive operand
// whose only form is a Go struct (an object-typed variable, not a literal) has no
// box constructor yet, so it hands back rather than emitting a coercion that does
// not exist.
func TestConcatStructVarHandsBack(t *testing.T) {
	const src = "function f(o: { a: number }): string { return \"x\" + o; }\nconsole.log(f({ a: 1 }));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "boxing this static type") {
		t.Errorf("hand-back reason = %q, want it to mention boxing a static type", nyl.Reason)
	}
}
