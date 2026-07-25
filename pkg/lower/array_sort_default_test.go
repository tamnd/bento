package lower

import (
	"strings"
	"testing"
)

// TestArraySortDefaultLowers pins that a comparator-less sort synthesizes the
// default string-order comparator: each element is read to a value.BStr and the two
// are compared with value.BStr.Compare, wrapped to float64 for the value method.
func TestArraySortDefaultLowers(t *testing.T) {
	const src = "const nums = [10, 9, 100, 1];\nconsole.log(nums.sort().join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Sort(func(a, b") {
		t.Fatalf("comparator-less sort did not synthesize a default comparator:\n%s", source)
	}
	if !strings.Contains(source, ".Compare(") || !strings.Contains(source, "float64(") {
		t.Fatalf("default sort comparator did not compare through value.BStr.Compare as float64:\n%s", source)
	}
}

// TestArraySortDefaultRuns builds and runs the comparator-less sort and toSorted over
// numbers (lexicographic default order: 1, 10, 100, 9), strings, and booleans, plus a
// toSorted that leaves the receiver untouched. Each matches Node's default order.
func TestArraySortDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const nums = [10, 9, 100, 1];\n" +
		"console.log(nums.sort().join(\",\"));\n" +
		"const strs = [\"banana\", \"apple\", \"cherry\"];\n" +
		"console.log(strs.sort().join(\",\"));\n" +
		"const orig = [3, 1, 2];\n" +
		"console.log(orig.toSorted().join(\",\"));\n" +
		"console.log(orig.join(\",\"));\n" +
		"const bools = [true, false, true];\n" +
		"console.log(bools.sort().join(\",\"));\n"
	got := runProgramGo(t, src)
	want := "1,10,100,9\napple,banana,cherry\n1,2,3\n3,1,2\nfalse,true,true\n"
	if got != want {
		t.Fatalf("default sort program printed %q, want %q", got, want)
	}
}
