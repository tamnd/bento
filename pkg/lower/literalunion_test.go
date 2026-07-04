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
// string-literal union is not covered here: it lowers to an integer tag enum
// (union.go). A union that folds nothing (a mixed or optional union) keeps its own
// flags and hands the unit back to the interpreter.

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

// TestMixedUnionReturnHandsBack proves a union that folds nothing (string | number,
// where the members disagree on their primitive) keeps its own flags and hands the
// unit back rather than being coerced to one primitive.
func TestMixedUnionReturnHandsBack(t *testing.T) {
	const src = "export function m(n: number): string | number { if (n > 0) return \"pos\"; return n; }\nconsole.log(m(1));\n"
	renderProgramHandBack(t, src)
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
