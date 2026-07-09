package lower

import (
	"strings"
	"testing"
)

// TestDeeplyNestedTypeHandsBack pins the depth guard in typeExpr: a type nested
// far past anything a real annotation reaches hands back instead of recursing
// until the goroutine stack overflows and takes the worker down with it. A few
// hundred nested array levels is well past maxTypeDepth and stands in for the
// self-referential types the checker can hand typeExpr.
func TestDeeplyNestedTypeHandsBack(t *testing.T) {
	src := "function f(x: number" + strings.Repeat("[]", maxTypeDepth+8) + "): void {}"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "self-referential or excessively nested") {
		t.Fatalf("deeply nested type handed back for the wrong reason: %q", reason)
	}
}

// TestShallowNestedTypeLowers pins the other side of the guard: a type nested a
// handful of levels, the way a genuine annotation is, lowers as before. The
// ceiling only ever trips on a pathological type, never on a real one.
func TestShallowNestedTypeLowers(t *testing.T) {
	src := "function f(x: number[][][]): number { return x.length; }"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.Array[*value.Array[*value.Array[float64]]]") {
		t.Fatalf("an ordinary nested type did not lower to the expected shape:\n%s", out)
	}
}
