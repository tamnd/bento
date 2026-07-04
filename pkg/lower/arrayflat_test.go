package lower

import (
	"strings"
	"testing"
)

// TestArrayFlatEmitsFreeFunc pins that flat on an array of arrays lowers to the
// value.Flat free function instantiated at the inner element type.
func TestArrayFlatEmitsFreeFunc(t *testing.T) {
	src := "const a = [[1, 2], [3], [4, 5]];\nconsole.log(a.flat().length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Flat[float64](") {
		t.Errorf("flat did not lower to value.Flat[float64]:\n%s", source)
	}
}

// TestArrayFlatWithDepthHandsBack pins that an explicit depth argument is a later
// slice and hands the unit back.
func TestArrayFlatWithDepthHandsBack(t *testing.T) {
	src := "const a = [[1, 2], [3]];\nconsole.log(a.flat(1).length);\n"
	renderProgramHandBack(t, src)
}

// TestArrayFlatRuns builds and runs flat against the Node oracle, flattening an
// array of number arrays one level and confirming empty inner arrays drop out.
func TestArrayFlatRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log([[1, 2], [3], [4, 5, 6]].flat().join(","));
const nums: number[][] = [[1], [2, 3]];
console.log(nums.flat().join(","));
const a = [["a", "b"], ["c"]];
console.log(a.flat().join(""));
`
	got := runProgramGo(t, src)
	want := "1,2,3,4,5,6\n1,2,3\nabc\n"
	if got != want {
		t.Fatalf("flat program printed %q, want %q", got, want)
	}
}
