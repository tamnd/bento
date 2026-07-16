package lower

import (
	"strings"
	"testing"
)

// TestAnyConstTernaryBoxesString pins that a string ternary bound to an any const
// boxes into a value.Value rather than landing a value.BStr in the slot: the
// flatten path bails on a dynamic binding, so the ordinary decl lowers the ternary
// to its IIFE and boxes the one result through value.StringValue, the same box a
// plain `const v: any = "s"` gets. Before the fix the flatten form declared v as a
// value.BStr and a later dynamic use called value.ToString on it, which go build
// rejected.
func TestAnyConstTernaryBoxesString(t *testing.T) {
	src := `function f(x: number): void {
  const v: any = x > 0 ? "u" : "d";
  console.log(v);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.StringValue(") {
		t.Fatalf("any const string ternary did not box through StringValue:\n%s", out)
	}
	if strings.Contains(out, "var v value.BStr") {
		t.Fatalf("any const still declared v as a bare BStr slot:\n%s", out)
	}
}

// TestPlainConstTernaryStillFlattens guards the boundary: a statically typed
// binding keeps the flattened if form, its slot the branches' widened primitive,
// so the dynamic bail does not disturb the common case.
func TestPlainConstTernaryStillFlattens(t *testing.T) {
	src := `function f(x: number): void {
  const s: string = x > 0 ? "u" : "d";
  console.log(s);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var s value.BStr") {
		t.Fatalf("plain string ternary decl no longer flattens to a BStr slot:\n%s", out)
	}
}

// TestAnyConstTernaryRuns builds and runs a string, a number, and a boolean ternary
// each bound to an any const, so the box is proven to render through console.log the
// way JavaScript does for all three primitives.
func TestAnyConstTernaryRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function run(x: number): void {
  const s: any = x > 0 ? "u" : "d";
  const n: any = x > 0 ? 1 : 2;
  const b: any = x > 0 ? true : false;
  console.log(s);
  console.log(n);
  console.log(b);
}
run(1);
run(-1);
`
	got := runProgramGo(t, src)
	want := "u\n1\ntrue\nd\n2\nfalse\n"
	if got != want {
		t.Fatalf("any const ternary run mismatch:\n got %q\nwant %q", got, want)
	}
}
