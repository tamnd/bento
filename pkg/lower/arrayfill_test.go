package lower

import (
	"strings"
	"testing"
)

// TestArrayFillEmitsMethod pins that a .fill(v, start, end) call lowers to the
// value.Array Fill method, the in-place range write. The value and the two
// Number bounds pass straight through, since the checker has already typed the
// value against the element type.
func TestArrayFillEmitsMethod(t *testing.T) {
	const src = `const a: number[] = [1, 2, 3, 4, 5];
a.fill(9, 1, 3);
console.log(a.join(","));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Fill(") {
		t.Errorf("array fill did not lower to the Fill method:\n%s", source)
	}
}

// TestArrayFillRuns builds and runs the generated Go, proving fill overwrites
// the same range JavaScript's Array.prototype.fill does: the whole array with
// no bounds, a half-open range with both bounds, and a negative start that
// counts from the end. The last case fills through the returned reference and
// then pushes on it, so the mutation is visible on the original binding, which
// proves fill returns the receiver rather than a copy.
func TestArrayFillRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a: number[] = [1, 2, 3, 4, 5];
a.fill(0);
console.log(a.join(","));
const b: number[] = [1, 2, 3, 4, 5];
b.fill(9, 1, 3);
console.log(b.join(","));
const c: number[] = [1, 2, 3, 4, 5];
c.fill(7, -2);
console.log(c.join(","));
const d: number[] = [1, 2, 3];
const e = d.fill(5);
e.push(6);
console.log(d.join(","));
`
	got := runProgramGo(t, src)
	const want = "0,0,0,0,0\n1,9,9,4,5\n1,2,3,7,7\n5,5,5,6\n"
	if got != want {
		t.Fatalf("array fill printed %q, want %q", got, want)
	}
}
