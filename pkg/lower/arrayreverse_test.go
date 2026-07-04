package lower

import (
	"strings"
	"testing"
)

// TestArrayReverseEmitsMethod pins that reverse lowers to the value.Array
// Reverse method on the receiver.
func TestArrayReverseEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\na.reverse();\nconsole.log(a[0]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Reverse()") {
		t.Errorf("reverse did not lower to the Reverse method:\n%s", source)
	}
}

// TestArrayReverseRuns builds and runs reverse against the Node oracle: the
// in-place reordering read back through indices, that reverse returns the same
// array so a push through the returned handle is visible on the original, and
// that an empty array reverses to itself.
func TestArrayReverseRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
a.reverse();
console.log(a[0]);
console.log(a[3]);
const b = [10, 20, 30];
const same = b.reverse();
console.log(same[0]);
same.push(99);
console.log(b.length);
console.log(b[3]);
const empty: number[] = [];
empty.reverse();
console.log(empty.length);
`
	got := runProgramGo(t, src)
	want := "4\n1\n30\n4\n99\n0\n"
	if got != want {
		t.Fatalf("array reverse program printed %q, want %q", got, want)
	}
}
