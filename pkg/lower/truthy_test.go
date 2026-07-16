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

// TestTruthyObjectCollapses pins that an object in boolean position collapses to the
// Go constant true, since an object has no falsy member: the checker proves the type
// carries no null or undefined, so the condition can only be truthy.
func TestTruthyObjectCollapses(t *testing.T) {
	src := "function f(o: { x: number }): number { if (o) { return 1; } return 0; }\nconsole.log(f({ x: 1 }));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if true {") {
		t.Errorf("object condition did not collapse to a constant:\n%s", source)
	}
}

// TestTruthyUnionCallsToBoolean pins that a tagged-sum union in boolean position reads
// its truth through the union's ToBoolean method, which switches the tag to the active
// arm's falsy rule rather than mixing the rules by hand.
func TestTruthyUnionCallsToBoolean(t *testing.T) {
	src := "function f(x: number | string): number { if (x) { return 1; } return 0; }\nconsole.log(f(1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if x.ToBoolean() {") {
		t.Errorf("union condition did not lower to ToBoolean:\n%s", source)
	}
}

// TestTruthyOptionalNumberInlines pins that an optional number in a condition tests
// presence and the inner ToBoolean together: !x.IsUndefined() gates undefined, then
// the inner zero-and-NaN test runs on x.Get(), so an absent option and a present zero
// are both falsy.
func TestTruthyOptionalNumberInlines(t *testing.T) {
	src := "function f(x?: number): number { if (x) { return 1; } return 0; }\nconsole.log(f(2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !x.IsUndefined() && (x.Get() != 0 && x.Get() == x.Get()) {") {
		t.Errorf("optional number condition did not inline presence plus the inner test:\n%s", source)
	}
}

// TestTruthyOptionalStringInlines pins the string inner: an absent option is falsy,
// and a present option runs the emptiness test on the unwrapped inner.
func TestTruthyOptionalStringInlines(t *testing.T) {
	src := "function f(s?: string): number { if (s) { return 1; } return 0; }\nconsole.log(f(\"x\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !s.IsUndefined() && s.Get().Length() > 0 {") {
		t.Errorf("optional string condition did not inline presence plus the emptiness test:\n%s", source)
	}
}

// TestTruthyOptionalObjectInlines pins the non-primitive inner: an optional over a
// shape (or array, or class instance) the checker proved always truthy when present
// tests presence alone, so the emitted condition is just !x.IsUndefined() with no inner
// ToBoolean, because a present object has no falsy member.
func TestTruthyOptionalObjectInlines(t *testing.T) {
	src := "function f(x?: { a: number }): number { if (x) { return 1; } return 0; }\nconsole.log(f({ a: 1 }));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !x.IsUndefined() {") {
		t.Errorf("optional object condition did not reduce to the presence test alone:\n%s", source)
	}
	if strings.Contains(source, "x.Get()") {
		t.Errorf("optional object condition spelled an inner test it should not:\n%s", source)
	}
}

// TestTruthyOptionalObjectRuns builds and runs an optional over an object shape and an
// array across the boolean positions and matches the oracle: an absent option is falsy
// and any present object or array is truthy, since neither carries a falsy member.
func TestTruthyOptionalObjectRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function shape(x?: { a: number }): string {
  if (x) {
    return "y";
  }
  return "n";
}
function arr(x?: number[]): string {
  return x ? "some" : "none";
}
console.log(shape({ a: 1 }));
console.log(shape());
console.log(arr([1]));
console.log(arr());
`
	got := runProgramGo(t, src)
	want := "y\nn\nsome\nnone\n"
	if got != want {
		t.Fatalf("optional object truthiness program printed %q, want %q", got, want)
	}
}

// TestTruthyOptionalNonRepeatableHandsBack pins that an optional whose evaluation has
// a side effect, a Map.get here, cannot inline: the presence test and the inner test
// each name the operand, so a non-repeatable one keeps the handback rather than fire
// the effect twice.
func TestTruthyOptionalNonRepeatableHandsBack(t *testing.T) {
	src := "function f(m: Map<string, number>): number { if (m.get(\"a\")) { return 1; } return 0; }\nconsole.log(f(new Map()));\n"
	renderProgramHandBack(t, src)
}

// TestTruthyOptionalRuns builds and runs an optional across the boolean positions, a
// condition, a negation, and a ternary, over both a number and a string inner, and
// matches the oracle: an absent option is falsy, a present zero or empty string is
// falsy, any other present value is truthy.
func TestTruthyOptionalRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function num(x?: number): string {
  if (x) {
    return "t";
  }
  return "f";
}
function str(s?: string): string {
  if (s) {
    return "has";
  }
  return "empty";
}
function neg(x?: number): string {
  if (!x) {
    return "falsy";
  }
  return "truthy";
}
function tern(x?: number): string {
  return x ? "y" : "n";
}
console.log(num(5));
console.log(num(0));
console.log(num());
console.log(str("hi"));
console.log(str(""));
console.log(str());
console.log(neg(0));
console.log(neg(3));
console.log(tern(5));
console.log(tern());
`
	got := runProgramGo(t, src)
	want := "t\nf\nf\nhas\nempty\nempty\nfalsy\ntruthy\ny\nn\n"
	if got != want {
		t.Fatalf("optional truthiness program printed %q, want %q", got, want)
	}
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
