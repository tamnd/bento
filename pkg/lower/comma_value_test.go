package lower

import (
	"strings"
	"testing"
)

// TestCommaSideEffectingLeftRuns pins that a comma expression whose left side
// mutates a variable runs the left for its effect and yields the right. Before
// this slice the comma operator handed back; now the left runs into the blank
// identifier and the right is the value.
func TestCommaSideEffectingLeftRuns(t *testing.T) {
	const src = `let a = 0;
let b = ((a = a + 1), 5);
console.log(a);
console.log(b);
`
	got := runProgramGo(t, src)
	if got != "1\n5\n" {
		t.Errorf("comma expression ran wrong\n got: %q\nwant: %q", got, "1\n5\n")
	}
}

// TestCommaPureLeftLowers pins that a comma whose left side has no side effect,
// the shape the checker flags 2695, still lowers once the front door admits the
// diagnostic. The strict compile helper would reject the code before the renderer
// ran, so this goes through the tolerant front door the AOT build uses; the
// end-to-end run of a pure-left comma is proven by the conformance fixture.
func TestCommaPureLeftLowers(t *testing.T) {
	const src = `let x = (1, 2, 3);
console.log(x);
`
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("pure-left comma handed back: %v", err)
	}
	if !strings.Contains(p.Source, "_ =") {
		t.Errorf("pure-left comma did not evaluate its left into the blank identifier:\n%s", p.Source)
	}
}

// TestCommaEvaluatesLeftToRight pins the whole chain runs left to right: each
// comma left mutates the counter before the final operand reads it.
func TestCommaEvaluatesLeftToRight(t *testing.T) {
	const src = `let n = 0;
let last = ((n = n + 10), (n = n + 1), n);
console.log(last);
`
	got := runProgramGo(t, src)
	if got != "11\n" {
		t.Errorf("comma chain ran wrong\n got: %q\nwant: %q", got, "11\n")
	}
}

// TestCommaLowersToBlankAssign pins the wrapper shape: the left evaluates into
// the blank identifier and the closure returns the right, the form a value
// position needs from an expression Go has no comma operator for.
func TestCommaLowersToBlankAssign(t *testing.T) {
	const src = `let a = 0;
let b = ((a = a + 1), 5);
console.log(b);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "_ =") {
		t.Errorf("comma left was not evaluated into the blank identifier:\n%s", source)
	}
}
