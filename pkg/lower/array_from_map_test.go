package lower

import (
	"strings"
	"testing"
)

// TestArrayFromMapLowers pins that Array.from(arr, fn) over a real array delegates to
// the array map lowering: a type-changing callback spells value.MapArray[T, U].
func TestArrayFromMapLowers(t *testing.T) {
	const src = "const a = Array.from([1, 2, 3], x => String(x));\nconsole.log(a.join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.MapArray") {
		t.Fatalf("Array.from with a type-changing map callback did not lower to value.MapArray:\n%s", source)
	}
}

// TestArrayFromMapRuns builds and runs Array.from with a map callback over an array:
// a same-type callback (x*2) and a type-changing one (String(x)), each matching Node.
func TestArrayFromMapRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(Array.from([1, 2, 3], x => x * 2).join(\",\"));\n" +
		"console.log(Array.from([1, 2, 3], x => String(x)).join(\",\"));\n"
	got := runProgramGo(t, src)
	want := "2,4,6\n1,2,3\n"
	if got != want {
		t.Fatalf("Array.from map program printed %q, want %q", got, want)
	}
}

// TestArrayFromMapNonArrayHandsBack pins the boundary: Array.from with a map callback
// over a string source is a later slice, distinct from the array delegation.
func TestArrayFromMapNonArrayHandsBack(t *testing.T) {
	const src = "const a = Array.from(\"abc\", c => c.toUpperCase());\nconsole.log(a.join(\",\"));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "non-array source") {
		t.Errorf("hand-back reason = %q, want it to mention a non-array source", reason)
	}
}
