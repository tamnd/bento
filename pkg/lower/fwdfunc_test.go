package lower

import (
	"strings"
	"testing"
)

// TestForwardFuncBindingHoists pins that a plain function binding a closure above
// it captures is declared once at the scope top and assigned at its own site. The
// arrow a reads b before b's line, which JavaScript allows because both consts are
// scoped to the whole module; Go needs b declared before the closure, so the
// binding pre-declares as a var and its site lowers to a plain assignment.
func TestForwardFuncBindingHoists(t *testing.T) {
	src := `const a = () => b()
const b = () => null
a()`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var b func()") {
		t.Fatalf("forward function binding did not pre-declare its var:\n%s", out)
	}
	if strings.Contains(out, "b := func()") {
		t.Fatalf("forward function binding kept a short declaration Go rejects:\n%s", out)
	}
}

// TestForwardFuncBindingRuns builds and runs the forward reference so the hoist is
// proven by the program result, not just the emitted shape: a calls b, defined
// after a, and the call runs after both are bound.
func TestForwardFuncBindingRuns(t *testing.T) {
	skipIfShort(t)
	src := `const a = () => b()
const b = () => 7
console.log(a())`
	got := runProgramGo(t, src)
	if got != "7\n" {
		t.Fatalf("forward function reference run mismatch:\n got %q\nwant %q", got, "7\n")
	}
}

// TestNonForwardFuncBindingStaysShort pins the boundary: a function binding no
// earlier statement captures keeps its ordinary short declaration, so the hoist
// touches only the forward-captured case and leaves the common shape untouched.
func TestNonForwardFuncBindingStaysShort(t *testing.T) {
	src := `const b = () => 7
const a = () => b()
a()`
	out := renderProgram(t, src)
	if strings.Contains(out, "var b func()") {
		t.Fatalf("a non-forward function binding was needlessly pre-declared:\n%s", out)
	}
}
