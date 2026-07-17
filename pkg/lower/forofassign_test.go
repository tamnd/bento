package lower

import (
	"strings"
	"testing"
)

// TestForOfAssignTargetLowers pins that a for...of whose head assigns each element to
// an existing binding, `for (x of it)`, ranges the array's backing slice and assigns
// each element to the target at the top of the body, rather than handing back.
func TestForOfAssignTargetLowers(t *testing.T) {
	src := "function f(xs: number[]): void { let x = 0; for (x of xs) { console.log(x); } }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "range xs.Elems()") {
		t.Fatalf("for-of assignment target did not range the backing slice:\n%s", out)
	}
	if !strings.Contains(out, "x = ") {
		t.Fatalf("for-of assignment target did not assign the element to the existing binding:\n%s", out)
	}
}

// TestForOfAssignTargetBooleanLowers pins the boolean shape: the checker spells a
// boolean array's element type true | false, which folds to a plain Go bool, so the
// assignment target lowers rather than tripping the union guard.
func TestForOfAssignTargetBooleanLowers(t *testing.T) {
	src := "function f(flags: boolean[]): void { let b = false; for (b of flags) { console.log(b); } }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "range flags.Elems()") {
		t.Fatalf("boolean for-of assignment target did not range the backing slice:\n%s", out)
	}
}

// TestForOfAssignTargetStringCodePoints pins that a string source drives CodePoints,
// the same backing walk the declared-binding path uses, into the assignment target.
func TestForOfAssignTargetStringCodePoints(t *testing.T) {
	src := `function f(text: string): void { let c = ""; for (c of text) { console.log(c); } }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "range text.CodePoints()") {
		t.Fatalf("string for-of assignment target did not range CodePoints:\n%s", out)
	}
}

// TestForOfAssignTargetMixedUnionHandsBack pins the soundness boundary: a target
// typed number | string has no single plain Go representation, so assigning a float64
// element to it by name would not compile. The slice hands back rather than miscompile.
func TestForOfAssignTargetMixedUnionHandsBack(t *testing.T) {
	src := "function f(xs: number[]): void { let v: number | string = 0; for (v of xs) { console.log(v); } }"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "single loop binding") {
		t.Fatalf("mixed-union for-of assignment target handed back for the wrong reason: %q", reason)
	}
}

// TestForOfAssignTargetOptionalHandsBack pins that an optional target, string |
// undefined stored as value.Opt, is not a plain primitive repr, so it hands back
// rather than assign a bare string element into an Opt slot.
func TestForOfAssignTargetOptionalHandsBack(t *testing.T) {
	src := `function f(xs: string[]): void { let s: string | undefined; for (s of xs) { console.log(s); } }`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "single loop binding") {
		t.Fatalf("optional for-of assignment target handed back for the wrong reason: %q", reason)
	}
}

// TestForOfAssignTargetRuns builds and runs the shapes end to end: the target carries
// each element the way a declared binding would, and keeps the last element after the
// loop, matching JavaScript.
func TestForOfAssignTargetRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function sumInto(xs: number[]): number {
  let x = 0;
  let total = 0;
  for (x of xs) {
    total = total + x;
  }
  return total + x;
}
function joinInto(parts: string[]): string {
  let s = "";
  let out = "";
  for (s of parts) {
    out = out + s;
  }
  return out;
}
sumInto([1, 2, 3]);
console.log(sumInto([1, 2, 3, 4]));
console.log(joinInto(["a", "b", "c"]));
`
	got := runProgramGo(t, src)
	want := "14\nabc\n"
	if got != want {
		t.Fatalf("for-of assignment target run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestForOfAssignPatternLowers pins that an array-pattern head, `for ([a, b] of pairs)`,
// ranges the array of tuples and assigns each position to its existing binding at the top
// of the body, the assignment-form sibling of the const destructure.
func TestForOfAssignPatternLowers(t *testing.T) {
	src := `function f(pairs: [number, string][]): void {
  let a = 0;
  let b = "";
  for ([a, b] of pairs) { console.log(a + b); }
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "range pairs.Elems()") {
		t.Fatalf("for-of assignment pattern did not range the backing slice:\n%s", out)
	}
	if !strings.Contains(out, "a, b = ") {
		t.Fatalf("for-of assignment pattern did not assign the tuple positions to the existing bindings:\n%s", out)
	}
}

// TestForOfAssignPatternRefinedHandsBack pins the soundness boundary: a target refined
// to an int holds a Go type the float64 tuple field does not, so assigning it by name
// would not compile. The slice hands back rather than miscompile.
func TestForOfAssignPatternRefinedHandsBack(t *testing.T) {
	src := `function f(pairs: [number, string][]): void {
  let a = 0 | 0;
  let b = "";
  for ([a, b] of pairs) { console.log(b); a = a | 0; }
}`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "single loop binding") {
		t.Fatalf("refined for-of assignment pattern handed back for the wrong reason: %q", reason)
	}
}

// TestForOfAssignPatternRuns builds and runs the pattern head end to end: each position
// carries its tuple field the way a declared binding would, and both keep the last pair's
// values after the loop, matching JavaScript.
func TestForOfAssignPatternRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function run(pairs: [number, string][]): string {
  let a = 0;
  let b = "";
  let out = "";
  for ([a, b] of pairs) {
    out = out + b + a;
  }
  return out + "|" + a + b;
}
console.log(run([[1, "a"], [2, "b"], [3, "c"]]));
`
	got := runProgramGo(t, src)
	want := "a1b2c3|3c\n"
	if got != want {
		t.Fatalf("for-of assignment pattern run mismatch:\n got %q\nwant %q", got, want)
	}
}
