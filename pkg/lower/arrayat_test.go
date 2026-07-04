package lower

import (
	"strings"
	"testing"
)

// TestArrayAtEmitsMethod pins that a .at(i) call lowers to the value.Array
// AtOpt method, the relative-index read whose T | undefined result is an
// optional. The receiver's element type carries through the method, so no type
// argument is spelled at the call site.
func TestArrayAtEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3];
const x = a.at(-1);
if (x !== undefined) {
  console.log(x);
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".AtOpt(") {
		t.Errorf("array at did not lower to the AtOpt method:\n%s", source)
	}
}

// TestArrayAtRuns builds and runs the generated Go, proving at reads the same
// element JavaScript's Array.prototype.at does: a forward index, a negative
// index counting from the end, and an out-of-range index that yields undefined
// so the presence guard takes its else branch. The optional is consumed through
// the x !== undefined narrowing the pop path already exercises.
func TestArrayAtRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [10, 20, 30, 40];
const first = a.at(0);
if (first !== undefined) {
  console.log(first);
}
const last = a.at(-1);
if (last !== undefined) {
  console.log(last);
}
const mid = a.at(-2);
if (mid !== undefined) {
  console.log(mid);
}
const oob = a.at(10);
if (oob !== undefined) {
  console.log(oob);
} else {
  console.log(0);
}
`
	got := runProgramGo(t, src)
	const want = "10\n40\n30\n0\n"
	if got != want {
		t.Fatalf("array at printed %q, want %q", got, want)
	}
}
