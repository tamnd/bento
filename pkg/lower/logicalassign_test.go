package lower

import (
	"strings"
	"testing"
)

// TestNullishAssignEmitsGuardedAssign pins that x ??= y on an optional target
// with an optional right-hand side lowers to an if guarded by IsUndefined around
// a plain assignment, so y is evaluated only when x is undefined.
func TestNullishAssignEmitsGuardedAssign(t *testing.T) {
	src := `
function pick(seed: string | undefined, d: string | undefined): string | undefined {
  let x: string | undefined = seed;
  x ??= d;
  return x;
}
console.log(pick(undefined, "fallback") !== undefined);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "IsUndefined()") {
		t.Fatalf("expected an IsUndefined guard, got:\n%s", out)
	}
	if !strings.Contains(out, "if ") {
		t.Fatalf("expected an if statement, got:\n%s", out)
	}
}

// TestNullishAssignDefiniteHandsBack pins that x ??= y with a definite right-hand
// side, which narrows the target, hands back until narrowing at an assignment
// lands.
func TestNullishAssignDefiniteHandsBack(t *testing.T) {
	src := `
function pick(x: string | undefined): string {
  x ??= "fallback";
  return x;
}
console.log(pick(undefined));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "narrows the target") {
		t.Fatalf("expected a narrowing handback, got: %q", reason)
	}
}

// TestOrAssignEmitsNegatedGuard pins that x ||= y on a boolean target lowers to
// an if guarded by the negation of x.
func TestOrAssignEmitsNegatedGuard(t *testing.T) {
	src := `
let flag = false;
flag ||= true;
console.log(flag);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "if !flag {") {
		t.Fatalf("expected an if !flag guard, got:\n%s", out)
	}
}

// TestAndAssignEmitsGuard pins that x &&= y on a boolean target lowers to an if
// guarded by x itself.
func TestAndAssignEmitsGuard(t *testing.T) {
	src := `
let flag = true;
flag &&= false;
console.log(flag);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "if flag {") {
		t.Fatalf("expected an if flag guard, got:\n%s", out)
	}
}

// TestOrAssignNumberGuard pins that ||= on a number target lowers through the
// number ToBoolean, so the guard is the negated non-zero-and-not-NaN test rather
// than a handback.
func TestOrAssignNumberGuard(t *testing.T) {
	src := `
let n = 0;
n ||= 5;
console.log(n);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "if !(n != 0 && n == n) {") {
		t.Fatalf("expected the number truthiness guard, got:\n%s", out)
	}
}

// TestOrAssignStringGuard pins that ||= on a string target guards on the
// code-unit length, the empty-string falsy test negated.
func TestOrAssignStringGuard(t *testing.T) {
	src := `
let s = "";
s ||= "x";
console.log(s);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "if !(s.Length() > 0) {") {
		t.Fatalf("expected the string truthiness guard, got:\n%s", out)
	}
}

// TestLogicalAssignDynamicGuard pins that ||= and &&= on a dynamic target guard
// on value.ToBoolean, the whole falsy set behind one call, negated for ||=.
func TestLogicalAssignDynamicGuard(t *testing.T) {
	or := renderProgram(t, "function f(x: any): void { x ||= 5; }\nf(0);\n")
	if !strings.Contains(or, "if !value.ToBoolean(x) {") {
		t.Fatalf("expected a negated ToBoolean guard for ||=, got:\n%s", or)
	}
	and := renderProgram(t, "function g(x: any): void { x &&= 5; }\ng(1);\n")
	if !strings.Contains(and, "if value.ToBoolean(x) {") {
		t.Fatalf("expected a ToBoolean guard for &&=, got:\n%s", and)
	}
}

// TestNullishAssignRuns builds and runs the emitted Go and checks the value
// against the Node oracle: an undefined optional takes the fallback, a present
// one keeps its value, and the fallback is not evaluated when the value is
// present.
func TestNullishAssignRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function tag(seed: string | undefined, d: string | undefined): string {
  let x: string | undefined = seed;
  x ??= d;
  if (x !== undefined) {
    return x;
  }
  return "none";
}
console.log(tag(undefined, "filled"));
console.log(tag("set", "filled"));
console.log(tag(undefined, undefined));
`
	got := runProgramGo(t, src)
	want := "filled\nset\nnone\n"
	if got != want {
		t.Fatalf("??= run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestBooleanLogicalAssignRuns builds and runs ||= and &&= on booleans and
// checks the results against the Node oracle.
func TestBooleanLogicalAssignRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let a = false;
a ||= true;
console.log(a);
let b = true;
b ||= false;
console.log(b);
let c = true;
c &&= false;
console.log(c);
let d = false;
d &&= true;
console.log(d);
`
	got := runProgramGo(t, src)
	want := "true\ntrue\nfalse\nfalse\n"
	if got != want {
		t.Fatalf("boolean logical-assign run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestNonBooleanLogicalAssignRuns builds and runs ||= and &&= on number, string,
// and dynamic targets and checks the results against the JavaScript answers: a
// zero and an empty string are falsy so ||= fills them, a non-zero and a
// non-empty string keep their value, and a dynamic target reads the same falsy
// set at runtime.
func TestNonBooleanLogicalAssignRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let n = 0;
n ||= 5;
console.log(n);
let m = 3;
m ||= 9;
console.log(m);
let s = "";
s ||= "filled";
console.log(s);
let t2 = "set";
t2 &&= "changed";
console.log(t2);
function d(x: any): number { x ||= 7; return x; }
console.log(d(0));
console.log(d(2));
`
	got := runProgramGo(t, src)
	want := "5\n3\nfilled\nchanged\n7\n2\n"
	if got != want {
		t.Fatalf("non-boolean logical-assign run mismatch:\n got %q\nwant %q", got, want)
	}
}
