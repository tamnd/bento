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

// TestTypedNullUnionInternsTaggedSum pins that the bare null literal still boxes to
// the value.Null singleton, but a null inside a T | null union rides the tagged sum:
// a string | null interns as StrOrNull and a compare against null narrows to a tag
// check rather than handing back.
func TestTypedNullUnionInternsTaggedSum(t *testing.T) {
	src := "let s: string | null = null;\nconsole.log(s === null);\n"
	out := renderProgram(t, src)
	for _, want := range []string{
		"type StrOrNull struct {",
		"s := StrOrNullOfNull()",
		"s.tag == StrOrNullNull",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("string | null did not intern as a tagged sum, missing %q:\n%s", want, out)
		}
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
