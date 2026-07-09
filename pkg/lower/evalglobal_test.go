package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestEvalGlobalHandsBack pins that a call to the ambient global eval hands back
// rather than lowering. bento has no eval, and the user-function path would emit a
// call to a capitalized Eval that was never declared, so the generated Go named an
// undefined symbol and failed to build. Handing back routes the unit to the
// interpreter instead. The test262 harness reaches eval inside assert.throws to
// probe a ReferenceError, so this kept a whole class of tests from building.
func TestEvalGlobalHandsBack(t *testing.T) {
	const src = "eval(\"var x = 1\");\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "ambient global eval") {
		t.Errorf("hand-back reason = %q, want it to mention the ambient global eval", nyl.Reason)
	}
}

// TestEvalAsValueHandsBack pins that eval read as a value, not called, also hands
// back. The indirect-eval shape var s = eval; s("...") never reaches the call
// path's guard: eval is a function symbol in the lib, so the function-used-as-a-
// value path would capitalize it to an undeclared Eval and the Go failed to build.
// The value path now hands back the same way the call path does.
func TestEvalAsValueHandsBack(t *testing.T) {
	const src = "var s = eval;\ns(\"var x = 1\");\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "ambient global eval") {
		t.Errorf("hand-back reason = %q, want it to mention the ambient global eval", nyl.Reason)
	}
}

// TestUserFunctionStillLowers pins that a user function shadowing nothing still
// lowers and calls through its exported Go name, so the ambient-global handback
// does not catch an ordinary call. A local declaration is not an ambient global,
// so the guard leaves it on the user-function path.
func TestUserFunctionStillLowers(t *testing.T) {
	src := `function twice(n: number): number { return n * 2; } console.log(String(twice(3)));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "Twice(") {
		t.Fatalf("user function call did not lower to its exported name:\n%s", out)
	}
}
