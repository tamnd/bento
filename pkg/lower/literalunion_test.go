package lower

import (
	"strings"
	"testing"
)

// A union whose members all share one numeric or boolean facet widens to that
// primitive: a union of numeric literals (1 | 2 | 3) is a number, and true | false
// is a boolean. A function returning such a union must lower to the primitive's Go
// type (float64, bool) rather than route to the tagged-sum machinery a real object
// union needs, which has no decl to render and used to crash the printer. A closed
// string-literal union folds the same way: it is a plain string at run time, so it
// lowers to value.BStr (union.go). A union that folds nothing (a mixed or optional
// union) keeps its own flags and hands the unit back to the interpreter.

// TestNumericLiteralUnionReturnLowers proves a function whose return type is the
// numeric-literal union 1 | 2 | 3 lowers to a float64 result rather than routing to
// the tagged-sum path, which has no decl for a literal union and used to panic.
func TestNumericLiteralUnionReturnLowers(t *testing.T) {
	const src = "export function h(n: number): 1 | 2 | 3 { if (n > 5) return 3; if (n > 0) return 2; return 1; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func H(n float64) float64") {
		t.Errorf("numeric-literal union return did not lower to float64:\n%s", source)
	}
}

// TestBooleanLiteralUnionReturnLowers proves a function whose return type is the
// boolean-literal union true | false lowers to a bool result.
func TestBooleanLiteralUnionReturnLowers(t *testing.T) {
	const src = "export function b(n: number): true | false { return n > 0; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func B(n float64) bool") {
		t.Errorf("boolean-literal union return did not lower to bool:\n%s", source)
	}
}

// TestMixedUnionReturnRuns proves a union that folds nothing (string | number, where
// the members disagree on their primitive) keeps its own flags and lowers to the
// tagged-sum machinery: the function returns the NumOrStr struct, and console.log
// coerces it through the union's ToString, printing the string arm without quotes and
// the number arm as its digits, exactly as Node's console does for each primitive.
func TestMixedUnionReturnRuns(t *testing.T) {
	skipIfShort(t)
	const src = "export function m(n: number): string | number { if (n > 0) return \"pos\"; return n; }\nconsole.log(m(1));\nconsole.log(m(-3));\n"
	got := runProgramGo(t, src)
	want := "pos\n-3\n"
	if got != want {
		t.Fatalf("mixed union run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestStringLiteralUnionReturnLowers proves a function whose return type is the
// closed string-literal union "a" | "b" lowers to a value.BStr result, the same
// string it is at run time, rather than handing back.
func TestStringLiteralUnionReturnLowers(t *testing.T) {
	const src = "export function s(n: number): \"a\" | \"b\" { return n > 0 ? \"a\" : \"b\"; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func S(n float64) value.BStr") {
		t.Errorf("string-literal union return did not lower to value.BStr:\n%s", source)
	}
}

// TestStringLiteralUnionRun builds and runs a program that reads a string-literal
// union across a compare, a print, a concat, and a reassignment, proving it renders
// the way JavaScript does through the ordinary string machinery.
func TestStringLiteralUnionRun(t *testing.T) {
	skipIfShort(t)
	const src = `
type Dir = "north" | "south" | "east" | "west";
function opposite(d: Dir): Dir {
  if (d === "north") return "south";
  if (d === "south") return "north";
  return d;
}
let cur: Dir = "north";
console.log(cur);
console.log(opposite(cur));
console.log("dir=" + cur);
console.log(typeof cur);
cur = "east";
console.log(cur === "east");
`
	if got, want := runProgramGo(t, src), "north\nsouth\ndir=north\nstring\ntrue\n"; got != want {
		t.Fatalf("string-literal union run printed %q, want %q", got, want)
	}
}

// TestLiteralUnionReturnsRun builds and runs the generated Go so a folded union
// return is proven to compute the right value, not just to lower.
func TestLiteralUnionReturnsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
function rank(n: number): 1 | 2 | 3 {
  if (n > 5) return 3;
  if (n > 0) return 2;
  return 1;
}
function positive(n: number): true | false {
  return n > 0;
}
console.log(rank(7));
console.log(rank(3));
console.log(positive(3));
console.log(positive(-1));
`
	if got, want := runProgramGo(t, src), "3\n2\ntrue\nfalse\n"; got != want {
		t.Fatalf("literal-union returns printed %q, want %q", got, want)
	}
}
