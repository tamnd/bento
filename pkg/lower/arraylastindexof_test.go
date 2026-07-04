package lower

import (
	"strings"
	"testing"
)

// TestArrayLastIndexOfEmitsMethod pins that a .lastIndexOf(x) call lowers to the
// value.Array LastIndexOf method over a synthesized equality closure, the same
// shape indexOf takes.
func TestArrayLastIndexOfEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3, 2, 1];
console.log(a.lastIndexOf(2));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".LastIndexOf(") {
		t.Errorf("array lastIndexOf did not lower to the LastIndexOf method:\n%s", source)
	}
}

// TestArrayLastIndexOfFromIndexHandsBack pins that lastIndexOf with a fromIndex
// argument defers, the same boundary indexOf keeps: the second argument bounds
// where the reverse scan starts, which the whole-array method does not model.
func TestArrayLastIndexOfFromIndexHandsBack(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3, 2, 1];
console.log(a.lastIndexOf(2, 2));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "fromIndex") {
		t.Errorf("lastIndexOf fromIndex hand-back reason = %q, want it to mention fromIndex", reason)
	}
}

// TestArrayLastIndexOfRuns builds and runs the generated Go, proving lastIndexOf
// returns the index of the last matching element: a duplicated number found at
// its later position, a match at the very end, a miss returning -1, and a string
// array over the value.BStr equality.
func TestArrayLastIndexOfRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [1, 2, 3, 2, 1];
console.log(a.lastIndexOf(2));
console.log(a.lastIndexOf(1));
console.log(a.lastIndexOf(9));
const s: string[] = ["a", "b", "a"];
console.log(s.lastIndexOf("a"));
`
	got := runProgramGo(t, src)
	const want = "3\n4\n-1\n2\n"
	if got != want {
		t.Fatalf("array lastIndexOf printed %q, want %q", got, want)
	}
}
