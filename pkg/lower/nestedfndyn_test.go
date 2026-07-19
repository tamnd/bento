package lower

import (
	"strings"
	"testing"
)

// TestNestedFnParamNarrowsAndUnboxes pins that a function expression scopes its
// own dynamic locals: an any-typed parameter the checker narrows past a typeof
// guard unboxes through the matching accessor inside the nested body. Before the
// nested body scoped its own set, the parameter stayed a bare box and the
// narrowed arithmetic read emitted Go that did not compile.
func TestNestedFnParamNarrowsAndUnboxes(t *testing.T) {
	src := `const f = function (x: any): number { if (typeof x === "number") { return x * 2; } return 0; };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "x.AsNumber()") {
		t.Fatalf("nested function param did not unbox through AsNumber:\n%s", out)
	}
}

// TestNestedArrowParamNarrowsAndUnboxes pins the same scoping for a block-bodied
// arrow, the other function form blockBodyArrow lowers.
func TestNestedArrowParamNarrowsAndUnboxes(t *testing.T) {
	src := `const f = (x: any): number => { if (typeof x === "number") { return x + 1; } return 0; };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "x.AsNumber()") {
		t.Fatalf("nested arrow param did not unbox through AsNumber:\n%s", out)
	}
}

// TestNestedFuncReadAsValueUsesLocalName pins that reading a nested function
// declaration as a value spells the Go local it binds, not the capitalized
// top-level name; the two routings must agree or the value read names an
// undefined exported symbol.
func TestNestedFuncReadAsValueUsesLocalName(t *testing.T) {
	src := `function foo() {
  function bar() { }
  var x = bar;
  return x;
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "x := bar") {
		t.Fatalf("nested function read as value did not use its Go local name:\n%s", out)
	}
	if strings.Contains(out, "Bar") {
		t.Fatalf("nested function read as value took the exported name:\n%s", out)
	}
}

// TestNestedFnRuns builds and runs the nested-function narrowing end to end and
// checks each branch reads the unboxed value.
func TestNestedFnRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const classify = function (x: any): string {
  if (typeof x === "number") {
    let doubled: number = x * 2;
    return "num " + doubled;
  }
  return "other";
};
console.log(classify(21));
console.log(classify("hi"));
`
	got := runProgramGo(t, src)
	want := "num 42\nother\n"
	if got != want {
		t.Fatalf("nested function run mismatch:\n got %q\nwant %q", got, want)
	}
}
