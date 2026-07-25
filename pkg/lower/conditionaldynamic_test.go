package lower

import (
	"strings"
	"testing"
)

// TestConditionalDynamicLowers pins that a ternary whose branches are an any-typed
// value and a concrete primitive, so the checker types the whole expression any,
// lowers to an IIFE returning value.Value: the any branch passes through and the
// string branch boxes through value.StringValue, the way an argument crossing into
// an any parameter does. Before this slice the mixed shape handed back at the
// same-primitive union gate because no tagged sum spells an any-typed result.
func TestConditionalDynamicLowers(t *testing.T) {
	src := `function f(cond: boolean, a: any): void {
  const v: any = cond ? a : "/";
  console.log(v);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func() value.Value {") {
		t.Fatalf("dynamic ternary did not lower to a value.Value IIFE:\n%s", out)
	}
	if !strings.Contains(out, "value.StringValue(") {
		t.Fatalf("dynamic ternary's static branch did not box:\n%s", out)
	}
}

// TestConditionalDynamicRuns builds and runs a ternary that mixes an any-typed
// argument with a string and with a number literal, reading each result back
// through console.log, so the box is proven to render the chosen branch the way
// JavaScript does whichever arm the condition selects.
func TestConditionalDynamicRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function pick(cond: boolean, a: any): void {
  const s: any = cond ? a : "fallback";
  const n: any = cond ? 7 : a;
  console.log(s);
  console.log(n);
}
pick(true, "given");
pick(false, 42);
`
	got := runProgramGo(t, src)
	want := "given\n7\nfallback\n42\n"
	if got != want {
		t.Fatalf("dynamic ternary run mismatch:\n got %q\nwant %q", got, want)
	}
}
