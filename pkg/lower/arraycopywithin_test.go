package lower

import (
	"strings"
	"testing"
)

// TestArrayCopyWithinEmitsMethod pins that copyWithin lowers to the value.Array
// CopyWithin method with its numeric bounds passed straight through.
func TestArrayCopyWithinEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3, 4, 5];\na.copyWithin(0, 3);\nconsole.log(a.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".CopyWithin(0, 3)") {
		t.Errorf("copyWithin did not lower to the CopyWithin method:\n%s", source)
	}
}

// TestArrayCopyWithinRuns builds and runs copyWithin against the Node oracle,
// covering the two-bound form, an explicit end, negative indices, and an
// overlapping copy.
func TestArrayCopyWithinRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log([1, 2, 3, 4, 5].copyWithin(0, 3).join(","));
console.log([1, 2, 3, 4, 5].copyWithin(0, 3, 4).join(","));
console.log([1, 2, 3, 4, 5].copyWithin(-2, 0).join(","));
console.log([1, 2, 3, 4, 5].copyWithin(2, 0).join(","));
`
	got := runProgramGo(t, src)
	want := "4,5,3,4,5\n4,2,3,4,5\n1,2,3,1,2\n1,2,1,2,3\n"
	if got != want {
		t.Fatalf("copyWithin program printed %q, want %q", got, want)
	}
}
