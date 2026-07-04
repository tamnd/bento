package lower

import (
	"strings"
	"testing"
)

// TestArrayShiftEmitsMethod pins that a .shift() call lowers to the value.Array
// Shift method, the front-of-array removal whose T | undefined result is an
// optional consumed through the !== undefined narrowing.
func TestArrayShiftEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3];
const first = a.shift();
if (first !== undefined) {
  console.log(first);
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Shift(") {
		t.Errorf("array shift did not lower to the Shift method:\n%s", source)
	}
}

// TestArrayUnshiftEmitsMethod pins that a .unshift(...) call lowers to the
// value.Array Unshift method, the front-of-array insertion returning the new
// length.
func TestArrayUnshiftEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [3];
const len = a.unshift(1, 2);
console.log(len);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Unshift(") {
		t.Errorf("array unshift did not lower to the Unshift method:\n%s", source)
	}
}

// TestArrayShiftUnshiftRun builds and runs the generated Go, proving the
// front-of-array pair against the Node oracle: shift removes and returns the
// first element and drops the rest by one index, shift on an empty array is the
// undefined optional that takes the else branch, unshift prepends its arguments
// in order and returns the new length, and unshift through an aliased binding is
// visible on the original, proving it mutates in place.
func TestArrayShiftUnshiftRun(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [1, 2, 3];
const first = a.shift();
if (first !== undefined) {
  console.log(first);
}
console.log(a.join(","));
const b: number[] = [];
const gone = b.shift();
if (gone !== undefined) {
  console.log(gone);
} else {
  console.log(-1);
}
const c: number[] = [3];
const len = c.unshift(1, 2);
console.log(len);
console.log(c.join(","));
const e: number[] = [9];
const f = e;
f.unshift(7, 8);
console.log(e.join(","));
`
	got := runProgramGo(t, src)
	const want = "1\n2,3\n-1\n3\n1,2,3\n7,8,9\n"
	if got != want {
		t.Fatalf("array shift/unshift printed %q, want %q", got, want)
	}
}
