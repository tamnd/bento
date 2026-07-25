package lower

import (
	"strings"
	"testing"
)

// TestArraySomeIndexLowers pins that a some callback taking (element, index)
// routes to the index-aware runtime variant rather than the element-only Some,
// which takes a func(T) bool the two-parameter arrow could not fit.
func TestArraySomeIndexLowers(t *testing.T) {
	const src = "console.log([1, 2, 3].some((x, i) => i === 2 && x === 3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".SomeIndex(") {
		t.Fatalf("some with an (element, index) callback did not lower to SomeIndex:\n%s", source)
	}
}

// TestArrayCallbackIndexRuns builds and runs the (element, index) forms of every
// arrayCallbackMethod method, each matching Node.
func TestArrayCallbackIndexRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log([1, 2, 3].some((x, i) => i === 2 && x === 3));\n" +
		"console.log([1, 2, 3].every((x, i) => x > i));\n" +
		"[10, 20, 30].forEach((x, i) => console.log(`${i}=${x}`));\n" +
		"console.log([5, 6, 7].find((x, i) => i === 1));\n" +
		"console.log([5, 6, 7].findIndex((x, i) => x === 7 && i === 2));\n" +
		"console.log([5, 6, 7].findLast((x, i) => i < 2));\n" +
		"console.log([5, 6, 7].findLastIndex((x, i) => x > 5));\n"
	got := runProgramGo(t, src)
	want := "true\ntrue\n0=10\n1=20\n2=30\n6\n2\n6\n2\n"
	if got != want {
		t.Fatalf("index-callback program printed %q, want %q", got, want)
	}
}

// TestArrayCallbackArrayParamHandsBack pins the boundary: a callback that also
// reads the third array parameter is a later slice.
func TestArrayCallbackArrayParamHandsBack(t *testing.T) {
	const src = "console.log([1, 2, 3].some((x, i, arr) => x === arr.length));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "array parameter") {
		t.Errorf("hand-back reason = %q, want it to mention the array parameter", reason)
	}
}
