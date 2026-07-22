package lower

import (
	"strings"
	"testing"
)

// TestTupleMapMaterializesHomogeneousArray pins that map borrowed on a homogeneous
// tuple materializes the tuple as a value.Array over its element type and dispatches
// through value.MapArray, since the tuple struct itself carries no Map method.
func TestTupleMapMaterializesHomogeneousArray(t *testing.T) {
	src := `let numNum: [number, number] = [100, 100];
export let b = numNum.map(n => n * n);
`
	out := renderProgramTolerant(t, src)
	if strings.Contains(out, "numNum.Map(") {
		t.Fatalf("tuple map dispatched a struct method the tuple has no such Map on:\n%s", out)
	}
	if !strings.Contains(out, "value.MapArray[float64, float64]") {
		t.Fatalf("tuple map did not lower to value.MapArray over the element type:\n%s", out)
	}
	if !strings.Contains(out, "value.NewArray[float64](numNum.E0, numNum.E1)") {
		t.Fatalf("tuple map did not materialize the tuple fields into a value.Array:\n%s", out)
	}
}

// TestTupleMapMaterializesUnionArray pins that map on a heterogeneous tuple
// materializes a tagged-sum array, wrapping each field into the arm its position
// selects, so a [number, string] maps as its number|string element union.
func TestTupleMapMaterializesUnionArray(t *testing.T) {
	src := `let numStr: [number, string] = [100, "hello"];
export let d = numStr.map(x => x);
`
	out := renderProgramTolerant(t, src)
	if strings.Contains(out, "numStr.Map(") {
		t.Fatalf("heterogeneous tuple map dispatched a nonexistent struct method:\n%s", out)
	}
	if !strings.Contains(out, "value.MapArray[") {
		t.Fatalf("heterogeneous tuple map did not lower to value.MapArray:\n%s", out)
	}
	// Each field is wrapped into its union arm constructor before it enters the array.
	if !strings.Contains(out, "Of") || !strings.Contains(out, "numStr.E0") || !strings.Contains(out, "numStr.E1") {
		t.Fatalf("heterogeneous tuple map did not wrap its fields into union arms:\n%s", out)
	}
}
