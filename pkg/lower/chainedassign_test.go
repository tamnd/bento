package lower

import (
	"strings"
	"testing"
)

// A chained assignment a = b = 5 assigns the value once and settles every target,
// so it lowers to the innermost assignment followed by an outward copy per target,
// b = 5; a = b. The right side evaluates once, which matters when it is a call.

// TestChainedAssignFlattens proves a = b = 5 lowers to b = 5 then a = b, in that
// order.
func TestChainedAssignFlattens(t *testing.T) {
	const src = "let a = 0; let b = 0; a = b = 5;\n"
	source := renderProgram(t, src)
	ib := strings.Index(source, "b = 5")
	ia := strings.Index(source, "a = b")
	if ib < 0 || ia < 0 {
		t.Fatalf("chained assignment did not lower to inner then copy:\n%s", source)
	}
	if ib > ia {
		t.Errorf("chained assignment copied before the inner assignment ran:\n%s", source)
	}
}

// TestChainedAssignRuns builds and runs a two-link and a three-link chain so every
// target is proven to hold the assigned value.
func TestChainedAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let a = 0;
let b = 0;
let c = 0;
a = b = c = 5;
console.log(a + "," + b + "," + c);

let x = 0;
let y = 0;
x = y = 9;
console.log(x + "," + y);
`
	if got, want := runProgramGo(t, src), "5,5,5\n9,9\n"; got != want {
		t.Fatalf("chained assignment printed %q, want %q", got, want)
	}
}

// TestChainedAssignEvaluatesRightOnce proves the right side runs once, not once per
// target, by chaining a call that logs each invocation: a single "call" line means
// it ran once even though two targets take its value.
func TestChainedAssignEvaluatesRightOnce(t *testing.T) {
	skipIfShort(t)
	const src = `
function next(): number {
  console.log("call");
  return 7;
}
let a = 0;
let b = 0;
a = b = next();
console.log(a + "," + b);
`
	if got, want := runProgramGo(t, src), "call\n7,7\n"; got != want {
		t.Fatalf("chained assignment printed %q, want %q", got, want)
	}
}
