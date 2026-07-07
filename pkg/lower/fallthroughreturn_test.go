package lower

import (
	"strings"
	"testing"
)

// TestFallThroughReturnsUndefined pins that a function whose declared return type
// is any and whose body can run off its end gets the trailing return
// value.Undefined. Without it the emitted Go had a value-returning function with
// no final return and did not compile. formatIdentityFreeValue in the test262
// prelude takes this shape with a switch over the value kind and no default arm.
func TestFallThroughReturnsUndefined(t *testing.T) {
	src := `function classify(x: string): any { switch (x) { case "a": return 1; } }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("fall-through any return did not emit the trailing undefined return:\n%s", out)
	}
}

// TestFallThroughReturnInFunctionExpr pins the same trailing return for a function
// expression, the other body form that can fall through, since compareArray and
// its siblings in the prelude are function expressions.
func TestFallThroughReturnInFunctionExpr(t *testing.T) {
	src := `const classify = function (x: string): any { switch (x) { case "a": return 1; } };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("fall-through any return in a function expression did not emit the trailing undefined return:\n%s", out)
	}
}

// TestTerminatingBodyKeepsNoTrailingReturn pins that a body that already returns on
// every path takes no extra trailing return, so a returning function is left as the
// developer wrote it.
func TestTerminatingBodyKeepsNoTrailingReturn(t *testing.T) {
	src := `function pick(x: string): any { if (x === "a") { return 1; } return 2; }`
	out := renderProgram(t, src)
	if strings.Contains(out, "return value.Undefined") {
		t.Fatalf("terminating body gained a spurious trailing undefined return:\n%s", out)
	}
}

// TestFallThroughRuns builds and runs the fall-through and checks the missing arm
// yields undefined the way JavaScript does.
func TestFallThroughRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function classify(x: string): any {
  switch (x) {
    case "num":
      return 1;
  }
}
console.log(String(classify("num")));
console.log(String(classify("other")));
`
	got := runProgramGo(t, src)
	want := "1\nundefined\n"
	if got != want {
		t.Fatalf("fall-through run mismatch:\n got %q\nwant %q", got, want)
	}
}
