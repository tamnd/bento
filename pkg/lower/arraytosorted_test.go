package lower

import (
	"strings"
	"testing"
)

// TestArrayToSortedEmitsMethod pins that toSorted lowers to the ToSorted method
// on the array receiver, passing the comparator through.
func TestArrayToSortedEmitsMethod(t *testing.T) {
	src := "const a = [3, 1, 2];\nconsole.log(a.toSorted((x, y) => x - y).join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToSorted(") {
		t.Errorf("toSorted did not lower to ToSorted:\n%s", source)
	}
}

// TestArrayToSortedNoComparatorHandsBack pins that a toSorted call without a
// comparator, which would need the default string-order sort, is a later slice.
func TestArrayToSortedNoComparatorHandsBack(t *testing.T) {
	src := "const a = [3, 1, 2];\nconsole.log(a.toSorted().join(\",\"));\n"
	renderProgramHandBack(t, src)
}

// TestArrayToSortedRuns builds and runs toSorted against the Node oracle,
// checking the sorted result and that the source array is left in its order.
func TestArrayToSortedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [3, 1, 4, 1, 5, 9, 2, 6];
console.log(a.toSorted((x, y) => x - y).join(","));
console.log(a.join(","));
console.log(a.toSorted((x, y) => y - x).join(","));
`
	got := runProgramGo(t, src)
	want := "1,1,2,3,4,5,6,9\n3,1,4,1,5,9,2,6\n9,6,5,4,3,2,1,1\n"
	if got != want {
		t.Fatalf("toSorted program printed %q, want %q", got, want)
	}
}
