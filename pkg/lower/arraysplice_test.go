package lower

import (
	"strings"
	"testing"
)

// TestArraySpliceRemoveEmitsMethod pins that splice with a start and delete count
// lowers to the value.Array Splice method.
func TestArraySpliceRemoveEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\na.splice(1, 1);\nconsole.log(a.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Splice(1, 1)") {
		t.Errorf("splice did not lower to the Splice method:\n%s", source)
	}
}

// TestArraySpliceInsertEmitsItems pins that inserted items pass through as the
// method's variadic tail after the start and count.
func TestArraySpliceInsertEmitsItems(t *testing.T) {
	src := "const a = [1, 2, 3];\na.splice(1, 1, 9, 8);\nconsole.log(a.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Splice(1, 1, 9, 8)") {
		t.Errorf("splice did not pass its inserted items:\n%s", source)
	}
}

// TestArraySpliceToEndEmitsMethod pins that the one-argument splice(start) form
// lowers to SpliceToEnd.
func TestArraySpliceToEndEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\na.splice(1);\nconsole.log(a.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".SpliceToEnd(1)") {
		t.Errorf("one-argument splice did not lower to SpliceToEnd:\n%s", source)
	}
}

// TestArraySpliceRuns builds and runs splice against the Node oracle: a
// remove-only splice returning the removed elements, an insert that grows the
// array, a negative start, and the one-argument to-end form.
func TestArraySpliceRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4, 5];
console.log(a.splice(1, 2).join(","));
console.log(a.join(","));
const b = [1, 2, 3];
b.splice(1, 1, 9, 8, 7);
console.log(b.join(","));
const c = [1, 2, 3, 4, 5];
console.log(c.splice(-2, 1).join(","));
console.log(c.join(","));
const d = [1, 2, 3, 4, 5];
console.log(d.splice(2).join(","));
console.log(d.join(","));
`
	got := runProgramGo(t, src)
	want := "2,3\n1,4,5\n1,9,8,7,3\n4\n1,2,3,5\n3,4,5\n1,2\n"
	if got != want {
		t.Fatalf("splice program printed %q, want %q", got, want)
	}
}
