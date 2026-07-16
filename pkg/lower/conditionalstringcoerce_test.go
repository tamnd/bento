package lower

import (
	"strings"
	"testing"
)

// TestConditionalStringTernaryCoerceLowers pins that a ternary whose branches are
// both string literals coerces to a string rather than handing back: the checker
// types the whole expression as the literal union "u" | "d", which folds no String
// facet, but conditionalExpr lowers it to a value.BStr IIFE, so isString now reads
// it as the string it is and console.log takes the string identity.
func TestConditionalStringTernaryCoerceLowers(t *testing.T) {
	src := `function f(x: number): void {
  console.log(x > 0 ? "u" : "d");
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.FromGoString("u")`) || !strings.Contains(out, `value.FromGoString("d")`) {
		t.Fatalf("string ternary did not lower its branches to a BStr:\n%s", out)
	}
}

// TestConditionalStringTernaryPresenceReturnLowers proves the item-115 shape, a
// presence test whose result is a string ternary in return position, lowers: the
// optional comparison is a Go bool and both branches are strings, so the ternary
// is a value.BStr the function returns without a coercion handback.
func TestConditionalStringTernaryPresenceReturnLowers(t *testing.T) {
	src := `function f(n: number | undefined): string {
  return n === undefined ? "missing" : "present";
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.FromGoString("missing")`) || !strings.Contains(out, `value.FromGoString("present")`) {
		t.Fatalf("presence-test string ternary did not lower:\n%s", out)
	}
}

// TestConditionalStringTernaryNestedLowers proves a chained string ternary lowers,
// the inner ternary caught by the isString delegation re-entering on the smaller
// node so the outer branches both read as strings.
func TestConditionalStringTernaryNestedLowers(t *testing.T) {
	src := `function f(x: number): void {
  console.log(x > 0 ? "a" : (x < 0 ? "b" : "c"));
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, `value.FromGoString("a")`) || !strings.Contains(out, `value.FromGoString("c")`) {
		t.Fatalf("nested string ternary did not lower:\n%s", out)
	}
}

// TestConditionalStringUnionBindingLowers pins that a value typed as a closed
// string-literal union stored in a binding (a parameter here) now lowers to a
// value.BStr, the same string it is at run time, so a bare read prints through the
// ordinary string machinery rather than handing back. This is the binding shape the
// conditional-expression isString hook did not cover, closed by rendering the union
// type itself as value.BStr.
func TestConditionalStringUnionBindingLowers(t *testing.T) {
	src := `function h(v: "on" | "off"): void {
  console.log(v);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "v value.BStr") {
		t.Fatalf("string-literal-union parameter should lower to a value.BStr, got:\n%s", out)
	}
}

// TestConditionalStringTernaryRuns builds and runs the coercion shapes so the
// string ternary is proven to render the way JavaScript does through console.log,
// String(), a template literal, and a return.
func TestConditionalStringTernaryRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function pick(x: number): void {
  console.log(x > 0 ? "u" : "d");
  console.log(String(x > 0 ? "u" : "d"));
  console.log(` + "`" + `val=${x > 0 ? "u" : "d"}` + "`" + `);
}
function tag(n: number | undefined): string {
  return n === undefined ? "missing" : "present";
}
pick(1);
pick(-1);
console.log(tag(undefined));
console.log(tag(5));
`
	got := runProgramGo(t, src)
	want := "u\nu\nval=u\nd\nd\nval=d\nmissing\npresent\n"
	if got != want {
		t.Fatalf("string ternary run mismatch:\n got %q\nwant %q", got, want)
	}
}
