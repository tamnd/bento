package lower

import (
	"strings"
	"testing"
)

// TestNullLiteralBoxes pins that the null literal in a dynamic slot lowers to the
// value.Null singleton, the box whose whole meaning is the null tag.
func TestNullLiteralBoxes(t *testing.T) {
	src := "let x: any = null;\nconsole.log(x === null);\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "x := value.Null") {
		t.Fatalf("null literal did not box to value.Null:\n%s", out)
	}
}

// TestUndefinedLiteralBoxes pins that the undefined global in a dynamic slot
// lowers to the value.Undefined singleton.
func TestUndefinedLiteralBoxes(t *testing.T) {
	src := "let y: any = undefined;\nconsole.log(y === undefined);\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "y := value.Undefined") {
		t.Fatalf("undefined literal did not box to value.Undefined:\n%s", out)
	}
}

// TestTypedNullUnionStillHandsBack pins that the boxing stays gated to the bare
// literal: a null inside a T | null union keeps its own representation, so a
// compare against it is not lowered by this slice and hands back.
func TestTypedNullUnionStillHandsBack(t *testing.T) {
	src := "let s: string | null = null;\nconsole.log(s === null);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "mixed or non-primitive operands") {
		t.Fatalf("expected the union compare to still hand back, got: %q", reason)
	}
}

// TestNullUndefinedBoxRuns builds and runs a program that stores the null and
// undefined literals in dynamic slots and reads their tags back, matching the
// JavaScript answers: null is null and not undefined, undefined is the mirror,
// and the two are loosely equal but not strictly.
func TestNullUndefinedBoxRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let a: any = null;
let b: any = undefined;
console.log(a === null);
console.log(a === undefined);
console.log(b === undefined);
console.log(b === null);
console.log(a == b);
console.log(a === b);
`
	got := runProgramGo(t, src)
	want := "true\nfalse\ntrue\nfalse\ntrue\nfalse\n"
	if got != want {
		t.Fatalf("null/undefined box run mismatch:\n got %q\nwant %q", got, want)
	}
}
