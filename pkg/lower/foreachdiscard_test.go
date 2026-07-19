package lower

import (
	"strings"
	"testing"
)

// TestForEachDiscardsValueCallback pins that a forEach callback with an expression
// body, which lowers to a value-returning func, is wrapped so its result is dropped
// and the callback fits ForEach's void func(T) parameter. Without the wrap the emitted
// func(T) R does not fit and Go rejects the call.
func TestForEachDiscardsValueCallback(t *testing.T) {
	const src = `const xs = ["a", "b"];
xs.forEach(x => x.length);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ForEach(func(") {
		t.Fatalf("forEach did not lower to a ForEach call:\n%s", out)
	}
	// The wrapping adapter has no result, so the inner value-returning literal is
	// embedded and driven for effect.
	if strings.Contains(out, ".ForEach(func(x value.BStr) float64") {
		t.Fatalf("forEach kept a value-returning callback that does not fit func(T):\n%s", out)
	}
}

// TestForEachDiscardRuns builds and runs a forEach whose callback returns a value: the
// result is dropped, and the side effect the body performs is observed, the way forEach
// ignores the return.
func TestForEachDiscardRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let total = 0;
const xs = [1, 2, 3];
xs.forEach(x => total += x);
console.log(total);
`
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("forEach discard run mismatch:\n got %q\nwant %q", got, want)
	}
}
