package lower

import (
	"strings"
	"testing"
)

// TestTernaryReturnFlattens pins the return-position shape: return c ? a : b lowers
// to an if that returns each branch, not to the immediately-invoked function the
// expression position emits.
func TestTernaryReturnFlattens(t *testing.T) {
	src := "function pick(c: boolean, a: number, b: number): number { return c ? a : b; }\nconsole.log(pick(true, 1, 2));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"if c {", "return a", "return b"} {
		if !strings.Contains(source, want) {
			t.Errorf("return ternary did not print %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "func() float64") {
		t.Errorf("return ternary lowered to an IIFE instead of an if:\n%s", source)
	}
}

// TestTernaryDeclFlattens pins the binding shape: const x = c ? a : b lowers to a
// bare var declaration plus an if that assigns the taken branch, the temporary doc
// 05 asks for.
func TestTernaryDeclFlattens(t *testing.T) {
	src := "function f(n: number): string { const s = n > 0 ? \"pos\" : \"neg\"; return s; }\nconsole.log(f(1));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"var s value.BStr", "s = value.FromGoString(\"pos\")", "} else {", "s = value.FromGoString(\"neg\")"} {
		if !strings.Contains(source, want) {
			t.Errorf("declaration ternary did not print %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "func() value.BStr") {
		t.Errorf("declaration ternary lowered to an IIFE instead of an if:\n%s", source)
	}
}

// TestTernaryAssignFlattens pins the assignment shape: x = c ? a : b lowers to an if
// that assigns each branch to the existing local.
func TestTernaryAssignFlattens(t *testing.T) {
	src := "function f(c: boolean): number { let x = 0; x = c ? 1 : 2; return x; }\nconsole.log(f(true));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"if c {", "x = 1", "} else {", "x = 2"} {
		if !strings.Contains(source, want) {
			t.Errorf("assignment ternary did not print %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "func() float64") {
		t.Errorf("assignment ternary lowered to an IIFE instead of an if:\n%s", source)
	}
}

// TestTernaryReturnChainFlattens pins the chained return shape: a ? x : b ? y : z
// becomes a straight run of ifs, one per condition, ending in the final return.
func TestTernaryReturnChainFlattens(t *testing.T) {
	src := "function grade(n: number): string { return n >= 90 ? \"a\" : n >= 80 ? \"b\" : \"c\"; }\nconsole.log(grade(95));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"if n >= 90 {", "return value.FromGoString(\"a\")", "if n >= 80 {", "return value.FromGoString(\"b\")", "return value.FromGoString(\"c\")"} {
		if !strings.Contains(source, want) {
			t.Errorf("chained return ternary did not print %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "func() value.BStr") {
		t.Errorf("chained return ternary lowered to an IIFE instead of ifs:\n%s", source)
	}
}

// TestTernaryDeclChainFlattens pins the chained binding shape: the false branch
// being another ternary becomes an else-if ladder rather than a nested block.
func TestTernaryDeclChainFlattens(t *testing.T) {
	src := "function grade(n: number): string { const g = n >= 90 ? \"a\" : n >= 80 ? \"b\" : \"c\"; return g; }\nconsole.log(grade(85));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"var g value.BStr", "if n >= 90 {", "} else if n >= 80 {", "} else {"} {
		if !strings.Contains(source, want) {
			t.Errorf("chained declaration ternary did not print %q:\n%s", want, source)
		}
	}
}

// TestTernaryExpressionPositionStaysIIFE pins the boundary: a ternary nested inside
// a larger expression still needs a value in place, so it keeps the
// immediately-invoked function conditionalExpr emits rather than flatten.
func TestTernaryExpressionPositionStaysIIFE(t *testing.T) {
	src := "function f(c: boolean, a: number, b: number): number { const x = (c ? a : b) + 1; return x; }\nconsole.log(f(true, 1, 2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() float64") {
		t.Errorf("nested ternary should stay an IIFE:\n%s", source)
	}
}

// TestTernaryUnbracedBodyFlattens pins that a ternary standing as the whole body of
// an unbraced if flattens the same way it does at the top level of a block, since
// the brace-optional body path lowers through the same multi-statement lowering.
func TestTernaryUnbracedBodyFlattens(t *testing.T) {
	src := "function f(c: boolean, n: number): number { let x = 0; if (n > 0) x = c ? 1 : 2; return x; }\nconsole.log(f(true, 5));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"if n > 0 {", "if c {", "x = 1", "} else {", "x = 2"} {
		if !strings.Contains(source, want) {
			t.Errorf("unbraced-body ternary did not print %q:\n%s", want, source)
		}
	}
	if strings.Contains(source, "func() float64") {
		t.Errorf("unbraced-body ternary lowered to an IIFE instead of an if:\n%s", source)
	}
}

// TestTernaryOptionalWrapsSomeNone pins the T | undefined ternary shape: a ternary
// whose whole-expression type is a two-member optional lowers to an IIFE returning
// value.Opt[T], the present branch wrapped in value.Some and the undefined branch in
// value.None, not the tagged-sum union.
func TestTernaryOptionalWrapsSomeNone(t *testing.T) {
	src := "const c = 1 > 0;\nconst x: string | undefined = c ? \"a\" : undefined;\nconsole.log(x !== undefined ? x : \"none\");\n"
	source := renderProgram(t, src)
	for _, want := range []string{
		"func() value.Opt[value.BStr]",
		"return value.Some[value.BStr](value.FromGoString(\"a\"))",
		"return value.None[value.BStr]()",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("optional ternary did not print %q:\n%s", want, source)
		}
	}
}

// TestTernaryOptionalRuns builds and runs the T | undefined ternary end to end: the
// present branch reads back through the !== undefined guard and the undefined branch
// takes the fallback, so both the Some and None runtime paths are exercised.
func TestTernaryOptionalRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const yes = 1 > 0;
const no = 1 < 0;
const a: string | undefined = yes ? "present" : undefined;
const b: string | undefined = no ? "present" : undefined;
console.log(a !== undefined ? a : "absent");
console.log(b !== undefined ? b : "absent");
`
	got := runProgramGo(t, src)
	want := "present\nabsent\n"
	if got != want {
		t.Fatalf("optional ternary program printed %q, want %q", got, want)
	}
}

// TestTernaryRuns builds and runs the flattened forms end to end and matches the
// Node oracle: a return ternary, a chained binding ternary, and a chained
// assignment ternary all pick the branch the condition selects.
func TestTernaryRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pick(c: boolean, a: number, b: number): number {
  return c ? a : b;
}
function grade(n: number): string {
  const g = n >= 90 ? "A" : n >= 80 ? "B" : "C";
  return g;
}
function classify(n: number): string {
  let s = "";
  s = n > 0 ? "pos" : n < 0 ? "neg" : "zero";
  return s;
}
console.log(pick(true, 1, 2));
console.log(pick(false, 1, 2));
console.log(grade(95));
console.log(grade(85));
console.log(grade(70));
console.log(classify(5));
console.log(classify(-5));
console.log(classify(0));
`
	got := runProgramGo(t, src)
	want := "1\n2\nA\nB\nC\npos\nneg\nzero\n"
	if got != want {
		t.Fatalf("ternary program printed %q, want %q", got, want)
	}
}
