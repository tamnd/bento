package lower

import (
	"strings"
	"testing"
)

// TestDynamicCompoundStringBoxes pins that += a string onto an any-typed target
// wraps the concat back into a box. value.Concat returns a bstr, so without the
// wrap the bstr would not fit the value.Value slot and the emitted Go would not
// compile. assert.throws and assert.sameValue in the test262 prelude append a
// separator this way with message += ' '.
func TestDynamicCompoundStringBoxes(t *testing.T) {
	src := `function f(message: any): void { message += " world"; }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.StringValue(value.Concat(") {
		t.Fatalf("dynamic += string did not box the concat result:\n%s", out)
	}
}

// TestDynamicCompoundNumberStaysBoxed pins that += a number onto an any-typed
// target keeps the boxed value.Add path and takes no extra wrap: the operator
// already produces a box, so wrapping it again would be a type error.
func TestDynamicCompoundNumberStaysBoxed(t *testing.T) {
	src := `function f(x: any): void { x += 5; }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Add(x, value.Number(5))") {
		t.Fatalf("dynamic += number did not stay on the boxed Add path:\n%s", out)
	}
	if strings.Contains(out, "StringValue(value.Add") {
		t.Fatalf("dynamic += number wrapped the boxed result a second time:\n%s", out)
	}
}

// TestDynamicCompoundStringRuns builds and runs += a string onto an any target
// and checks the concatenation reads back through the box.
func TestDynamicCompoundStringRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function label(message: any): string {
  message += " world";
  return message;
}
console.log(label("hi"));
`
	got := runProgramGo(t, src)
	want := "hi world\n"
	if got != want {
		t.Fatalf("dynamic += string run mismatch:\n got %q\nwant %q", got, want)
	}
}
