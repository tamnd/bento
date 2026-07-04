package lower

import (
	"strings"
	"testing"
)

// TestArraySortEmitsMethod pins that a .sort(cmp) call with an inline comparator
// lowers to the value.Array Sort method, the in-place ordering.
func TestArraySortEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [3, 1, 2];
a.sort((x, y) => x - y);
console.log(a.join(","));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Sort(") {
		t.Errorf("array sort did not lower to the Sort method:\n%s", source)
	}
}

// TestArraySortNoComparatorHandsBack pins that sort with no comparator defers:
// the default order coerces every element to a string and compares by code
// unit, a different element-to-string path that is its own later slice.
func TestArraySortNoComparatorHandsBack(t *testing.T) {
	const src = `const a: number[] = [3, 1, 2];
a.sort();
console.log(a.join(","));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "comparator") {
		t.Errorf("sort no-comparator hand-back reason = %q, want it to mention comparator", reason)
	}
}

// TestArraySortRuns builds and runs the generated Go, proving sort orders the
// array by its comparator: an ascending numeric sort, a descending one from a
// reversed comparator, a string array sorted by length so the value.BStr element
// runs through a numeric comparator, and a fourth case that pushes through the
// returned reference to prove sort returns the receiver rather than a copy.
func TestArraySortRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [3, 1, 4, 1, 5, 9, 2, 6];
a.sort((x, y) => x - y);
console.log(a.join(","));
const b: number[] = [3, 1, 4, 1, 5];
b.sort((x, y) => y - x);
console.log(b.join(","));
const s: string[] = ["ccc", "a", "bb"];
s.sort((x, y) => x.length - y.length);
console.log(s.join(","));
const c: number[] = [10, 2, 1];
const d = c.sort((x, y) => x - y);
d.push(0);
console.log(c.join(","));
`
	got := runProgramGo(t, src)
	const want = "1,1,2,3,4,5,6,9\n5,4,3,1,1\na,bb,ccc\n1,2,10,0\n"
	if got != want {
		t.Fatalf("array sort printed %q, want %q", got, want)
	}
}
