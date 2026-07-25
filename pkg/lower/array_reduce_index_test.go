package lower

import (
	"strings"
	"testing"
)

// TestArrayReduceIndexLowers pins that a reduce callback taking (acc, element,
// index) routes to the index-aware free function, which takes a
// func(A, T, float64) A the two-parameter form's ReduceIndex could not.
func TestArrayReduceIndexLowers(t *testing.T) {
	const src = "const a = [1, 2, 3, 4];\nconsole.log(a.reduce((acc: number, x: number, i: number): number => acc + x * i, 0));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ReduceIndex[") {
		t.Fatalf("reduce with an (acc, element, index) callback did not lower to ReduceIndex:\n%s", source)
	}
}

// TestArrayReduceIndexRuns builds and runs the (acc, element, index) forms of
// reduce and reduceRight, with and without an initial value, each matching Node.
func TestArrayReduceIndexRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log([1, 2, 3, 4].reduce((acc, x, i) => acc + x * i, 0));\n" +
		"console.log([1, 2, 3, 4].reduce((acc, x, i) => acc + x * i));\n" +
		"console.log([\"a\", \"b\", \"c\"].reduceRight((acc, x, i) => acc + x + i, \"\"));\n" +
		"console.log([1, 2, 3, 4].reduceRight((acc, x, i) => acc + x * i));\n"
	got := runProgramGo(t, src)
	want := "20\n21\nc2b1a0\n12\n"
	if got != want {
		t.Fatalf("reduce index-callback program printed %q, want %q", got, want)
	}
}

// TestArrayReduceArrayParamHandsBack pins the boundary: a reduce callback that
// also reads the fourth array parameter is a later slice.
func TestArrayReduceArrayParamHandsBack(t *testing.T) {
	const src = "console.log([1, 2, 3].reduce((acc, x, i, arr) => acc + x + arr.length, 0));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "array parameter") {
		t.Errorf("hand-back reason = %q, want it to mention the array parameter", reason)
	}
}
