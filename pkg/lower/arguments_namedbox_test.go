package lower

import (
	"strings"
	"testing"
)

// A named top-level function that reads its arguments object and flows into a dynamic
// slot by reference (const g: any = f) no longer hands the unit back. It materializes
// once as a shared module-level value.Value whose wrapper inlines the body and threads
// the real call-site arguments through a hidden trailing parameter, and every box of the
// same function reads that one var so JavaScript reference identity holds (g === h). A
// shape the wrapper cannot back (a rest or optional parameter, a binding-held function)
// keeps the existing handback.

// TestNamedBoxArgumentsRenders pins the render: boxing a named arguments-reading
// function by reference emits a module-level var and threads the real call-site
// arguments, rather than handing back at "needs the call-site count".
func TestNamedBoxArgumentsRenders(t *testing.T) {
	const src = `
function f(a: number): number { return arguments.length; }
const g: any = f;
console.log(g(1, 2, 3));
`
	source := renderProgramTolerant(t, src)
	if strings.Contains(source, "needs the call-site count") {
		t.Fatalf("boxing a named arguments-reading function still handed back:\n%s", source)
	}
	if !strings.Contains(source, "value.Value = value.NewFunc(") {
		t.Errorf("no module-level var wrapper for the boxed named function:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[value.Value](__a...)") {
		t.Errorf("the wrapper did not thread the real call-site arguments:\n%s", source)
	}
}

// TestNamedBoxArgumentsLengthRuns runs a boxed named function called through the box at
// three arities and directly, matching Node: arguments.length reads the real call count
// at each dynamic call, and the direct call still works.
func TestNamedBoxArgumentsLengthRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function f(a: number): number { return arguments.length; }
const g: any = f;
console.log(g(1, 2, 3));
console.log(g());
console.log(f(9));
`
	got := runProgramGoTolerant(t, src)
	// g(1,2,3) -> 3, g() -> 0, direct f(9) -> the snapshot arity, one parameter -> 1.
	want := "3\n0\n1\n"
	if got != want {
		t.Fatalf("named box arguments.length run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestNamedBoxArgumentsIdentity is the load-bearing identity test: boxing the same
// function at two sites must yield the same value.Value, so g === h prints true. A fresh
// wrapper per site would print false.
func TestNamedBoxArgumentsIdentity(t *testing.T) {
	skipIfShort(t)
	src := `
function f(a: number): number { return arguments.length; }
const g: any = f;
const h: any = f;
console.log(g === h);
`
	got := runProgramGoTolerant(t, src)
	want := "true\n"
	if got != want {
		t.Fatalf("named box identity mismatch (memoization broken):\n got %q\nwant %q", got, want)
	}
}

// TestNamedBoxArgumentsIndexRuns runs a boxed named function that reads arguments by
// index across the real arity, including a slot past the last parameter, matching the
// count the dynamic call actually passed.
func TestNamedBoxArgumentsIndexRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function join(a: number): string {
  let s = "";
  for (let i = 0; i < arguments.length; i++) {
    s += arguments[i];
  }
  return s;
}
const g: any = join;
console.log(g(1, 2, 3));
console.log(g(7));
`
	got := runProgramGoTolerant(t, src)
	want := "123\n7\n"
	if got != want {
		t.Fatalf("named box arguments index run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestNamedBoxArgumentsBindingHandsBack keeps the handback for a shape this box does not
// back: a function held by a value binding (const f = function(){...}), not a top-level
// declaration, boxed by reference. Only a symbol resolving to a single function
// declaration is materialized as the shared var; a binding-held function expression has
// no such declaration, so it stays on the existing later-slice handback.
func TestNamedBoxArgumentsBindingHandsBack(t *testing.T) {
	src := `
const f = function(a: number): number { return arguments.length; };
const g: any = f;
console.log(g(1, 2, 3));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "call-site count") {
		t.Fatalf("binding-held named box handback reason = %q, want the call-site-count handback", reason)
	}
}

// TestNamedBoxArgumentsRestHandsBack keeps the handback for a rest-parameter function
// that reads arguments, boxed by reference: the boxed wrapper coerces one argument per
// declared parameter and has no slot for the rest, so the unit stays on a later-slice
// handback (here the rest-arity handback the declaration itself raises).
func TestNamedBoxArgumentsRestHandsBack(t *testing.T) {
	// renderProgramTolerantHandBack fails the test if the program does not hand back, so
	// asserting it returns is the assertion: a rest-parameter arguments-reading function
	// boxed by reference is not lowered.
	_ = renderProgramTolerantHandBack(t, `
function f(...a: number[]): number { return arguments.length; }
const g: any = f;
console.log(g(1, 2, 3));
`)
}
