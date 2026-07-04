package lower

import (
	"strings"
	"testing"
)

// TestArrayConcatArraysEmitsMethod pins that concatenating two arrays of the same
// element type lowers to the value.Array Concat method with the argument array
// passed straight through to spread.
func TestArrayConcatArraysEmitsMethod(t *testing.T) {
	src := "const a = [1, 2];\nconst b = [3, 4];\nconsole.log(a.concat(b).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Concat(b)") {
		t.Errorf("array concat did not lower to the Concat method:\n%s", source)
	}
}

// TestArrayConcatValueWrapsElement pins that a non-array argument is wrapped in a
// one-element array so the runtime method sees a uniform list of arrays.
func TestArrayConcatValueWrapsElement(t *testing.T) {
	src := "const a = [1, 2];\nconsole.log(a.concat(3).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Concat(value.NewArray[float64](3))") {
		t.Errorf("array concat of a value did not wrap it in a one-element array:\n%s", source)
	}
}

// TestArrayConcatMixedArgs pins that concat spreads an array argument and appends
// a value argument in the same call, each classified by its type.
func TestArrayConcatMixedArgs(t *testing.T) {
	src := "const a = [1];\nconst b = [2, 3];\nconsole.log(a.concat(b, 4).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Concat(b, value.NewArray[float64](4))") {
		t.Errorf("array concat did not classify mixed arguments:\n%s", source)
	}
}

// TestArrayConcatRuns builds and runs concat against the Node oracle: array to
// array, appending a value, mixing the two, and confirming the receiver is left
// unchanged since concat returns a fresh array.
func TestArrayConcatRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3];
const b = [4, 5];
console.log(a.concat(b).join(","));
console.log(a.concat(6).join(","));
console.log(a.concat(b, 7).join(","));
console.log(a.join(","));
`
	got := runProgramGo(t, src)
	want := "1,2,3,4,5\n1,2,3,6\n1,2,3,4,5,7\n1,2,3\n"
	if got != want {
		t.Fatalf("concat program printed %q, want %q", got, want)
	}
}
