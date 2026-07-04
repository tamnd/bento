package lower

import (
	"strings"
	"testing"
)

// TestArrayWithEmitsMethod pins that with lowers to the With method on the array
// receiver, passing the index and the value through.
func TestArrayWithEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.with(1, 99).join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".With(") {
		t.Errorf("with did not lower to With:\n%s", source)
	}
}

// TestArrayWithRuns builds and runs with against the Node oracle, covering a
// plain replace, a negative index, and that the source array is left alone.
func TestArrayWithRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
console.log(a.with(1, 99).join(","));
console.log(a.with(-1, 88).join(","));
console.log(a.join(","));
const words = ["a", "b", "c"];
console.log(words.with(0, "z").join(""));
`
	got := runProgramGo(t, src)
	want := "1,99,3,4\n1,2,3,88\n1,2,3,4\nzbc\n"
	if got != want {
		t.Fatalf("with program printed %q, want %q", got, want)
	}
}
