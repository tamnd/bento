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

// TestOrAssignNonBooleanHandsBack pins that ||= on a non-boolean target, which
// needs JavaScript truthiness, hands back until truthiness lands.
func TestOrAssignNonBooleanHandsBack(t *testing.T) {
	src := `
let n = 0;
n ||= 5;
console.log(n);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "truthiness") {
		t.Fatalf("expected a truthiness handback, got: %q", reason)
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
