package lower

import (
	"strings"
	"testing"
)

// TestVarNoInitDynamicLowers pins that a dynamic binding declared with no
// initializer lowers to a bare value.Value declaration, whose Go zero value is
// the undefined a JavaScript `var x;` reads before its first assignment.
func TestVarNoInitDynamicLowers(t *testing.T) {
	src := "function f(): void { let x: any; x = 1; console.log(x); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "var x value.Value") {
		t.Fatalf("dynamic binding with no initializer did not lower to a bare value.Value:\n%s", out)
	}
}

// TestVarNoInitMultiLowers pins the multi-binding shape assert.throws uses, two
// dynamic names declared together with no initializer, each getting its own bare
// value.Value declaration.
func TestVarNoInitMultiLowers(t *testing.T) {
	src := "function f(): void { let a: any, b: any; a = 1; b = 2; console.log(a); console.log(b); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "a value.Value") || !strings.Contains(out, "b value.Value") {
		t.Fatalf("multi dynamic binding with no initializer did not lower to bare value.Value declarations:\n%s", out)
	}
}

// TestVarNoInitTypedHandsBack pins that a statically typed binding with no
// initializer still hands back: its Go zero value is not undefined, so declaring
// it bare would read the wrong value before its first assignment.
func TestVarNoInitTypedHandsBack(t *testing.T) {
	src := "function f(): void { let x: number; x = 1; console.log(x); }"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "no initializer") {
		t.Fatalf("typed binding with no initializer handed back for the wrong reason: %q", reason)
	}
}

// TestVarNoInitOptionalLowers pins that an optional binding declared with no
// initializer lowers to a bare value.Opt[T] declaration rather than handing back:
// the Go zero value of value.Opt[T] is the undefined case (None), exactly what a
// JavaScript optional reads before its first assignment, so the declaration is
// correct on its own.
func TestVarNoInitOptionalLowers(t *testing.T) {
	src := "function f(): void { let x: number | undefined; console.log(x === undefined); }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "var x value.Opt[float64]") {
		t.Fatalf("optional binding with no initializer did not lower to a bare value.Opt:\n%s", out)
	}
}

// TestVarNoInitOptionalRuns builds and runs the optional no-initializer shape: a
// fresh binding reads undefined, accepts a later assignment, and flows where the
// callee expects the same T | undefined, matching Node.
func TestVarNoInitOptionalRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function pick(v: string | undefined): string {
  return v ?? "fallback";
}
function f(): void {
  let s: string | undefined;
  console.log(pick(s));
  s = "hi";
  console.log(pick(s));
  let n: number | undefined;
  console.log(n === undefined);
}
f();
`
	got := runProgramGo(t, src)
	want := "fallback\nhi\ntrue\n"
	if got != want {
		t.Fatalf("optional no-initializer run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestVarNoInitRuns builds and runs the no-initializer shape and checks a dynamic
// binding reads undefined before its first assignment and the assigned value
// after, the way JavaScript does.
func TestVarNoInitRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function f(): void {
  let a: any, b: any;
  console.log(a === undefined);
  a = "x";
  b = 2;
  console.log(a);
  console.log(b);
}
f();
`
	got := runProgramGo(t, src)
	want := "true\nx\n2\n"
	if got != want {
		t.Fatalf("no-initializer run mismatch:\n got %q\nwant %q", got, want)
	}
}
