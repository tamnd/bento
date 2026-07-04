package lower

import (
	"strings"
	"testing"
)

// TestArrayToReversedEmitsMethod pins that toReversed lowers to the ToReversed
// method on the array receiver.
func TestArrayToReversedEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.toReversed().join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToReversed()") {
		t.Errorf("toReversed did not lower to ToReversed:\n%s", source)
	}
}

// TestArrayToReversedRuns builds and runs toReversed against the Node oracle,
// checking both the reversed result and that the source array is left alone.
func TestArrayToReversedRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
console.log(a.toReversed().join(","));
console.log(a.join(","));
const words = ["a", "b", "c"];
console.log(words.toReversed().join(""));
`
	got := runProgramGo(t, src)
	want := "4,3,2,1\n1,2,3,4\ncba\n"
	if got != want {
		t.Fatalf("toReversed program printed %q, want %q", got, want)
	}
}
