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

// TestForwardFuncBindingNotDoubleDeclaredWithVarHoist pins the fix for a
// redeclaration: a `var` function-valued binding read later is hoisted once by the
// var hoist, but an earlier closure holding its OWN local of the same name makes
// the forward-capture scan (which matches by name) think the module binding is
// captured too, so it would declare `var result` a second time and Go rejects it.
// The test262 asi/S7.9_A5.5_T2 case hit this: the assert prelude has a local
// `result` inside compareArray, and the test does `var result = function ...`.
// The var hoist owns the binding, so the fwd hoist must skip it: exactly one
// `var result` declaration, and the earlier closure keeps its own short local.
func TestForwardFuncBindingNotDoubleDeclaredWithVarHoist(t *testing.T) {
	src := `var obj: any = {};
obj.compare = function (): boolean { var result = helper(); if (result) { return true; } return false; };
function helper(): number { return 0; }
var result = function f(o: any) { o.x = 1; return o; };
if (typeof result !== "function") { throw new Error("bad"); }
`
	out := renderProgram(t, src)
	if n := strings.Count(out, "var result func("); n != 1 {
		t.Fatalf("module `var result` was declared %d times, want exactly 1:\n%s", n, out)
	}
	if !strings.Contains(out, "result = func(o value.Value)") {
		t.Fatalf("module `var result` site did not lower to an assignment:\n%s", out)
	}
	if !strings.Contains(out, "result := ") {
		t.Fatalf("the earlier closure lost its own short local named result:\n%s", out)
	}
}

// TestForwardFuncBindingNotDoubleDeclaredRuns builds and runs the same shape so
// the fix is proven by a clean compile-and-run, not just the emitted shape: the
// program must build (no `result redeclared in this block`) and reach the end.
func TestForwardFuncBindingNotDoubleDeclaredRuns(t *testing.T) {
	skipIfShort(t)
	src := `var obj: any = {};
obj.compare = function (): boolean { var result = helper(); if (result) { return true; } return false; };
function helper(): number { return 0; }
var result = function f(o: any) { o.x = 1; return o; };
if (typeof result !== "function") { throw new Error("bad"); }
console.log("ok");
`
	got := runProgramGo(t, src)
	if got != "ok\n" {
		t.Fatalf("run mismatch:\n got %q\nwant %q", got, "ok\n")
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
