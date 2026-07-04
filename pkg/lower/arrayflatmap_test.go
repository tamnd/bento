package lower

import (
	"strings"
	"testing"
)

// TestArrayFlatMapEmitsFreeFunc pins that flatMap over a callback returning an
// array lowers to value.FlatMap with the element and inner element types spelled
// as its two type arguments.
func TestArrayFlatMapEmitsFreeFunc(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.flatMap((n) => [n, -n]).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.FlatMap[float64, float64](") {
		t.Errorf("flatMap did not lower to value.FlatMap[float64, float64]:\n%s", source)
	}
}

// TestArrayFlatMapChangingType pins that a callback mapping to a different element
// type spells that inner type as the second type argument.
func TestArrayFlatMapChangingType(t *testing.T) {
	src := "const a = [1, 2];\nconsole.log(a.flatMap((n) => [String(n)]).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.FlatMap[float64, value.BStr](") {
		t.Errorf("flatMap did not spell the inner element type:\n%s", source)
	}
}

// TestArrayFlatMapValueCallbackHandsBack pins that a callback returning a bare
// value rather than an array is a later slice and hands the unit back.
func TestArrayFlatMapValueCallbackHandsBack(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.flatMap((n) => n).length);\n"
	renderProgramHandBack(t, src)
}

// TestArrayFlatMapRuns builds and runs flatMap against the Node oracle: a
// callback that doubles each element into a pair, one that expands each element
// into a longer run, and one that maps to a different element type.
func TestArrayFlatMapRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log([1, 2, 3].flatMap((n) => [n, -n]).join(","));
console.log([1, 2, 3].flatMap((n) => [n, n * 10, n * 100]).join(","));
console.log([1, 2].flatMap((n) => [String(n), String(-n)]).join(","));
`
	got := runProgramGo(t, src)
	want := "1,-1,2,-2,3,-3\n1,10,100,2,20,200,3,30,300\n1,-1,2,-2\n"
	if got != want {
		t.Fatalf("flatMap program printed %q, want %q", got, want)
	}
}
