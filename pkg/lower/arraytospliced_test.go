package lower

import (
	"strings"
	"testing"
)

// TestArrayToSplicedEmitsMethod pins that a two-argument-plus toSpliced lowers to
// the ToSpliced method on the array receiver.
func TestArrayToSplicedEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3, 4];\nconsole.log(a.toSpliced(1, 2, 20).join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToSpliced(") {
		t.Errorf("toSpliced did not lower to ToSpliced:\n%s", source)
	}
}

// TestArrayToSplicedToEndEmitsMethod pins that the one-argument form lowers to
// the ToSplicedToEnd method.
func TestArrayToSplicedToEndEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3, 4];\nconsole.log(a.toSpliced(2).join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToSplicedToEnd(") {
		t.Errorf("one-argument toSpliced did not lower to ToSplicedToEnd:\n%s", source)
	}
}

// TestArrayToSplicedRuns builds and runs toSpliced against the Node oracle,
// covering removal with insertion, the delete-to-end form, a negative start, and
// that the source array is left alone.
func TestArrayToSplicedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4, 5];
console.log(a.toSpliced(1, 2, 20, 30, 40).join(","));
console.log(a.toSpliced(2).join(","));
console.log(a.toSpliced(-2, 1, 99).join(","));
console.log(a.join(","));
`
	got := runProgramGo(t, src)
	want := "1,20,30,40,4,5\n1,2\n1,2,3,99,5\n1,2,3,4,5\n"
	if got != want {
		t.Fatalf("toSpliced program printed %q, want %q", got, want)
	}
}
