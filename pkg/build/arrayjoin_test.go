package build

import (
	"strings"
	"testing"
)

// TestDynamicArrayJoinLowers pins that join on an array whose element type is
// dynamic lowers instead of handing back. The lowerer cannot name a fixed
// element ToString for a boxed value.Value, so it passes value.JoinString as the
// per-element closure, which runs the abstract ToString at runtime with join's
// undefined-and-null-become-empty-string rule. A rest parameter typed any[] is
// the shape that reaches this, the same dynamic-element array the assert prelude's
// compareArray.format joins.
func TestDynamicArrayJoinLowers(t *testing.T) {
	src := "function j(...xs: any[]): string {\n  return xs.join(\", \");\n}\nconsole.log(j(1, \"x\", null));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("join on a dynamic-element array should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.JoinString(") {
		t.Fatalf("expected the per-element join to route through value.JoinString, got:\n%s", out)
	}
}

// TestTypedArrayJoinStillUsesFixedStringify pins that admitting the dynamic case
// did not divert an array with a static element type: a number[] join still spells
// its elements through value.NumberToString, not the runtime JoinString, so the
// fast fixed-type path is untouched.
func TestTypedArrayJoinStillUsesFixedStringify(t *testing.T) {
	src := "const xs: number[] = [1, 2, 3];\nconsole.log(xs.join(\", \"));\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("join on a number[] should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.NumberToString") {
		t.Fatalf("expected a number[] join to use value.NumberToString, got:\n%s", out)
	}
	if strings.Contains(out, "value.JoinString(") {
		t.Fatalf("a number[] join should not route through the dynamic JoinString, got:\n%s", out)
	}
}
