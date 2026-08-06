package lower

import (
	"strings"
	"testing"
)

// Boolean(x) called as a function is exactly the ToBoolean x undergoes standing in a
// boolean position, so it lowers the way `if (x)` does. It used to carry a list of its
// own, three primitives wide, and hand back on everything else with "Boolean() on this
// argument type is a later slice". That reason was the single largest refusal in the
// Node compatibility suite, 831 tests, and every one of them entered through the same
// line in the suite's own harness:
//
//	const hasCrypto = Boolean(process.versions.openssl) && ...
//
// which is a dynamic property read, the plainest thing lowerTruthy already handled. So
// the fix is not a wider list here, it is delegating past the primitives to lowerTruthy
// (truthy.go) and letting the one implementation of truthiness serve both spellings.
//
// The primitives stay spelled in booleanCoercion because each has a named helper that
// says what it is at the call site, and because Boolean(b) on a boolean is the identity
// rather than any test at all.

// TestBooleanOfADynamicReadsThroughToBoolean pins the case the suite gates on: an
// argument whose kind is only known at runtime reads its whole falsy set through the
// value model's ToBoolean, the same call an if over that read makes.
func TestBooleanOfADynamicReadsThroughToBoolean(t *testing.T) {
	got := renderUncheckedJS(t, "const x = Boolean(process.versions.openssl);\nconsole.log(x);\n")
	if !strings.Contains(got, "value.ToBoolean(bentoProcess.Get(") {
		t.Errorf("Boolean() over a dynamic read emitted:\n%s\nwant a value.ToBoolean call", got)
	}
}

// TestBooleanOfAnObjectCollapses pins that an argument the checker proved always truthy
// folds to the Go constant, since an object carries no falsy member. This is the same
// collapse `if (o)` takes, and taking it here is what makes Boolean(o) free rather than
// a runtime test whose answer is already fixed.
func TestBooleanOfAnObjectCollapses(t *testing.T) {
	src := "function f(o: { x: number }): boolean { return Boolean(o); }\nconsole.log(f({ x: 1 }));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "return true") {
		t.Errorf("Boolean() over an object emitted:\n%s\nwant a collapse to true", got)
	}
}

// TestBooleanOfAnOptionalTestsPresence pins the optional path: Boolean(x) over a T |
// undefined is falsy two ways, absent or present-but-falsy, and inlines the same
// presence-plus-inner test the condition form gets.
func TestBooleanOfAnOptionalTestsPresence(t *testing.T) {
	src := "function f(s?: string): boolean { return Boolean(s); }\nconsole.log(f(\"x\"));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "!s.IsUndefined() && s.Get().Length() > 0") {
		t.Errorf("Boolean() over an optional string emitted:\n%s\nwant the presence and emptiness test", got)
	}
}

// TestBooleanOfAUnionCallsItsToBoolean pins that a tagged-sum union asks its own
// generated ToBoolean method, which switches the tag to the active arm's falsy rule
// rather than mixing the rules by hand at the call site.
func TestBooleanOfAUnionCallsItsToBoolean(t *testing.T) {
	src := "function f(x: number | string): boolean { return Boolean(x); }\nconsole.log(f(0));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "x.ToBoolean()") {
		t.Errorf("Boolean() over a union emitted:\n%s\nwant the union's own ToBoolean", got)
	}
}

// TestBooleanOfAPrimitiveKeepsItsHelper pins that the four primitives still lower to
// the named helper that says what the test is, rather than being routed through the
// general path. A boolean argument is the identity, so it names no helper at all.
func TestBooleanOfAPrimitiveKeepsItsHelper(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want string
	}{
		{"number", "function f(n: number): boolean { return Boolean(n); }\nconsole.log(f(1));\n", "value.NumberToBool(n)"},
		{"string", "function f(s: string): boolean { return Boolean(s); }\nconsole.log(f(\"a\"));\n", "value.StringToBool(s)"},
		{"bigint", "function f(b: bigint): boolean { return Boolean(b); }\nconsole.log(f(1n));\n", "value.BigIntToBool(b)"},
		{"boolean", "function f(b: boolean): boolean { return Boolean(b); }\nconsole.log(f(true));\n", "return b"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := renderProgram(t, tc.src)
			if !strings.Contains(got, tc.want) {
				t.Errorf("Boolean() over a %s emitted:\n%s\nwant %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestBooleanOfTwoArgumentsHandsBack pins the arity guard. JavaScript evaluates the
// extra arguments for their side effects and drops them, and this slice does not model
// that, so a wider call keeps its refusal rather than silently losing an effect. The
// snippet is unchecked JavaScript because TypeScript rejects the second argument
// outright, and the point here is what the lowerer does with source the checker let
// through, not what the checker says about it.
func TestBooleanOfTwoArgumentsHandsBack(t *testing.T) {
	renderUncheckedJSHandBack(t, "function f(n, m) { return Boolean(n, m); }\nconsole.log(f(1, 2));\n")
}

// TestBooleanOfAnObjectBlanksItsBinding pins the half of this that is not in the
// coercion at all. Collapsing Boolean(o) to a constant drops o's only read, which would
// leave the Go binding declared and not used and fail the build with the reason lost.
// countElidedReads (program.go) now records the read the Boolean position drops, the
// same way it already recorded the one an if drops, so the binding gets its trailing
// blank and the program compiles.
func TestBooleanOfAnObjectBlanksItsBinding(t *testing.T) {
	got := renderUncheckedJS(t, "const a = [];\nconsole.log(Boolean(a));\n")
	if !strings.Contains(got, "_ = a") {
		t.Errorf("Boolean() over an unread array emitted:\n%s\nwant a blank for the orphaned binding", got)
	}
}

// TestNotOfAnObjectBlanksItsBinding pins the same for the ! spelling, which folds
// through lowerTruthy too and had the identical hole. The prefix operator is not a
// child node, so the case reads it the way prefixUnary does, as the node's text with
// the operand's text removed; a case that looked for an operator child never matched.
func TestNotOfAnObjectBlanksItsBinding(t *testing.T) {
	got := renderUncheckedJS(t, "const a = [];\nconsole.log(!a);\n")
	if !strings.Contains(got, "_ = a") {
		t.Errorf("! over an unread array emitted:\n%s\nwant a blank for the orphaned binding", got)
	}
}

// TestBooleanCoercionRuns builds and runs Boolean() across the argument kinds and
// matches the Node oracle: the falsy set is the same one truthiness reads, so a zero,
// a NaN, an empty string, an absent optional, and a missing property are all false,
// and every object, array, and non-empty string is true.
func TestBooleanCoercionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function num(n: number): boolean {
  return Boolean(n);
}
function str(s: string): boolean {
  return Boolean(s);
}
function opt(s?: string): boolean {
  return Boolean(s);
}
function obj(o: { a: number }): boolean {
  return Boolean(o);
}
console.log(num(1), num(0), num(0 / 0), num(-1));
console.log(str("x"), str("0"), str(""));
console.log(opt("x"), opt(""), opt());
console.log(obj({ a: 1 }));
`
	got := runProgramGo(t, src)
	want := "true false false true\ntrue true false\ntrue false false\ntrue\n"
	if got != want {
		t.Fatalf("Boolean() program printed %q, want %q", got, want)
	}
}
