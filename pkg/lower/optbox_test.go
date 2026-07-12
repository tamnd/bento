package lower

import (
	"strings"
	"testing"
)

// TestOptionalBoxesToDynamic pins that an optional result read into a dynamic sink
// lowers through value.OptToValue rather than handing back: console.log takes an
// any, so the number | undefined an array's at returns boxes there, the present
// case through value.Number and the undefined case through the undefined singleton.
func TestOptionalBoxesToDynamic(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3];
console.log(a.at(0));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.OptToValue(") {
		t.Errorf("optional into a dynamic slot did not lower through OptToValue:\n%s", source)
	}
}

// TestOptionalBoxToDynamicRuns builds and runs the generated Go, proving a present
// optional prints its value and an undefined one prints undefined, across a numeric
// array read and a string read, the two element boxes the coercion spells.
func TestOptionalBoxToDynamicRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [10, 20, 30];
console.log(a.at(0));
console.log(a.at(10));
const s = "hi";
console.log(s.at(1));
console.log(s.at(5));
`
	got := runProgramGo(t, src)
	const want = "10\nundefined\ni\nundefined\n"
	if got != want {
		t.Fatalf("optional boxing printed %q, want %q", got, want)
	}
}
