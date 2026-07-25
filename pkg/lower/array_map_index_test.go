package lower

import (
	"strings"
	"testing"
)

// TestArrayMapIndexLowers pins that a map callback taking (element, index) lowers to
// the index-aware runtime variant rather than the element-only Map (which would emit
// a func(T, float64) the one-parameter method could not take).
func TestArrayMapIndexLowers(t *testing.T) {
	const src = "const a = [1, 2, 3].map((x, i) => x + i);\nconsole.log(a.join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".MapIndex(") {
		t.Fatalf("map with an (element, index) callback did not lower to MapIndex:\n%s", source)
	}
}

// TestArrayMapFilterIndexRuns builds and runs the (element, index) forms of map,
// filter, a type-changing map, and Array.from, each matching Node.
func TestArrayMapFilterIndexRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log([1, 2, 3].map((x, i) => x + i).join(\",\"));\n" +
		"console.log([10, 20, 30].map((x, i) => `${i}:${x}`).join(\",\"));\n" +
		"console.log([1, 2, 3, 4].filter((x, i) => i % 2 === 0).join(\",\"));\n" +
		"console.log(Array.from([1, 2, 3], (x, i) => x + i).join(\",\"));\n"
	got := runProgramGo(t, src)
	want := "1,3,5\n0:10,1:20,2:30\n1,3\n1,3,5\n"
	if got != want {
		t.Fatalf("index-callback program printed %q, want %q", got, want)
	}
}

// TestArrayMapArrayParamHandsBack pins the boundary: a map callback that also reads
// the third array parameter is a later slice.
func TestArrayMapArrayParamHandsBack(t *testing.T) {
	const src = "const a = [1, 2, 3].map((x, i, arr) => x + arr.length);\nconsole.log(a.join(\",\"));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "array parameter") {
		t.Errorf("hand-back reason = %q, want it to mention the array parameter", reason)
	}
}
