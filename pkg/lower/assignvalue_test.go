package lower

import (
	"strings"
	"testing"
)

// An assignment read for its value, not run as a statement on its own line, has no
// direct Go form: Go's assignment is a statement and yields nothing. The value form
// rides an immediately-called closure that assigns the target and returns it, since
// the value of x = e in JavaScript is the value assigned.

// TestAssignmentValueLowersToAssignThenReturn proves const r = (x = 5) lowers to a
// closure that assigns the target and returns it.
func TestAssignmentValueLowersToAssignThenReturn(t *testing.T) {
	const src = "let x = 0; const r = (x = 5); console.log(r, x);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x = 5\n\t\treturn x") {
		t.Errorf("assignment in value position did not lower to an assign-then-return closure:\n%s", source)
	}
}

// TestAssignmentValueFormsRun builds and runs the value form in a binding, a while
// condition, and an element-assignment right side, so the assigned value is proven
// against the JavaScript result rather than just the emitted shape.
func TestAssignmentValueFormsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
let x = 0;
const r = (x = 5);
console.log(r, x);

let i = 0;
const n = 3;
while ((i = i + 1) < n) {
  console.log(i);
}
console.log("done", i);

const a = [0, 0];
let y = 0;
a[0] = (y = 7);
console.log(a[0], y);
`
	if got, want := runProgramGo(t, src), "5 5\n1\n2\ndone 3\n7 7\n"; got != want {
		t.Fatalf("assignment value form printed %q, want %q", got, want)
	}
}

// TestAssignmentValueOnNonIdentifierHandsBack proves the value form stays scoped to
// a plain local: an assignment into a property, read for its value, has no
// capturing closure yet and hands back rather than emit wrong Go.
func TestAssignmentValueOnNonIdentifierHandsBack(t *testing.T) {
	const src = "const o = { n: 0 }; const r = (o.n = 5); console.log(r);\n"
	renderProgramHandBack(t, src)
}
