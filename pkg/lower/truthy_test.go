package lower

import (
	"strings"
	"testing"
)

// TestTruthyNumberConditionInlines pins that a number in an if condition lowers to
// the inlined ToBoolean, the zero test with the NaN guard riding along, rather than
// a bare Go bool it does not have.
func TestTruthyNumberConditionInlines(t *testing.T) {
	src := "function f(n: number): number { if (n) { return 1; } return 0; }\nconsole.log(f(2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if n != 0 && n == n {") {
		t.Errorf("number condition did not inline the truthiness test:\n%s", source)
	}
}

// TestTruthyStringConditionInlines pins that a string in an if condition lowers to
// the non-empty test on its code-unit count.
func TestTruthyStringConditionInlines(t *testing.T) {
	src := "function f(s: string): number { if (s) { return 1; } return 0; }\nconsole.log(f(\"x\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if s.Length() > 0 {") {
		t.Errorf("string condition did not inline the emptiness test:\n%s", source)
	}
}

// TestTruthyImpureNumberUsesHelper pins that a number condition with a side effect
// routes through value.NumberToBool so the operand is evaluated once, not named
// twice by the inlined form.
func TestTruthyImpureNumberUsesHelper(t *testing.T) {
	src := "function f(x: number): number { if (Math.floor(x)) { return 1; } return 0; }\nconsole.log(f(2.5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToBool(math.Floor(x))") {
		t.Errorf("impure number condition did not route through NumberToBool:\n%s", source)
	}
}

// TestTruthyImpureStringUsesHelper pins that a string condition with a side effect
// routes through value.StringToBool for the same one-evaluation reason.
func TestTruthyImpureStringUsesHelper(t *testing.T) {
	src := "function f(s: string): number { if (s.trim()) { return 1; } return 0; }\nconsole.log(f(\" x \"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.StringToBool(s.Trim())") {
		t.Errorf("impure string condition did not route through StringToBool:\n%s", source)
	}
}

// TestTruthyLogicalNot pins that ! over a non-boolean negates its truthiness: !s is
// the emptiness test negated, the parenthesized form Go prints for a not over a
// comparison.
func TestTruthyLogicalNot(t *testing.T) {
	src := "function f(s: string): number { if (!s) { return 1; } return 0; }\nconsole.log(f(\"\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !(s.Length() > 0) {") {
		t.Errorf("logical not on a string did not negate the truthiness test:\n%s", source)
	}
}

// TestTruthyWhileCondition pins that the same lowering serves a while condition, so
// while (n) counts down on the number's truthiness.
func TestTruthyWhileCondition(t *testing.T) {
	src := "function f(n: number): number { let c = 0; while (n) { n = n - 1; c = c + 1; } return c; }\nconsole.log(f(3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for n != 0 && n == n {") {
		t.Errorf("while condition did not lower the truthiness test:\n%s", source)
	}
}

// TestTruthyObjectHandsBack pins the boundary: an object in boolean position has a
// falsy rule this slice does not model, so it hands the unit back rather than guess
// one. renderProgramHandBack asserts the whole program falls back to the engine.
func TestTruthyObjectHandsBack(t *testing.T) {
	src := "function f(o: { x: number }): number { if (o) { return 1; } return 0; }\nconsole.log(f({ x: 1 }));\n"
	renderProgramHandBack(t, src)
}

// TestTruthyRuns builds and runs the falsy set end to end and matches the Node
// oracle: zero and NaN are falsy numbers, the empty string is the only falsy
// string, a non-empty "0" is truthy, and ! flips each.
func TestTruthyRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function num(n: number): string {
  if (n) {
    return "t";
  }
  return "f";
}
function str(s: string): string {
  if (s) {
    return "t";
  }
  return "f";
}
function notNum(n: number): boolean {
  return !n;
}
function nanOf(x: number): number {
  return x / x - x / x;
}
console.log(num(5));
console.log(num(-1));
console.log(num(0));
console.log(num(nanOf(0)));
console.log(str("hi"));
console.log(str("0"));
console.log(str(""));
console.log(notNum(0));
console.log(notNum(3));
`
	got := runProgramGo(t, src)
	want := "t\nt\nf\nf\nt\nt\nf\ntrue\nfalse\n"
	if got != want {
		t.Fatalf("truthiness program printed %q, want %q", got, want)
	}
}
