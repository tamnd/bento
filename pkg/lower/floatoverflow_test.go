package lower

import (
	"strings"
	"testing"
)

// TestFloatConstOverflowEmitsInf pins that a constant multiply whose result runs
// past the float64 range lowers to math.Inf rather than a Go constant the compiler
// rejects as overflowing. 1e308 * 2 folds to 2e308, which Go will not fold into a
// float64, so without this the generated Go did not build.
func TestFloatConstOverflowEmitsInf(t *testing.T) {
	src := `const x = 1e308 * 2; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(1)") {
		t.Fatalf("overflowing constant multiply did not lower to math.Inf(1):\n%s", out)
	}
}

// TestFloatConstOverflowNegative pins the sign is carried, so -1e308 * 2 lowers to
// the negative infinity JavaScript produces rather than the positive one.
func TestFloatConstOverflowNegative(t *testing.T) {
	src := `const x = -1e308 * 2; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(-1)") {
		t.Fatalf("overflowing negative multiply did not lower to math.Inf(-1):\n%s", out)
	}
}

// TestInRangeConstArithUnchanged pins that ordinary constant arithmetic that fits
// float64 keeps its plain Go form, so the overflow guard does not disturb the
// common case or the folded literals the numeric paths rely on.
func TestInRangeConstArithUnchanged(t *testing.T) {
	src := `const a = 2 * 3; const b = 1.5 + 2.5; console.log(String(a + b));`
	out := renderProgram(t, src)
	if strings.Contains(out, "math.Inf") {
		t.Fatalf("in-range constant arithmetic was rewritten to math.Inf:\n%s", out)
	}
}

// TestFloatConstOverflowRuns builds and runs the overflow so the emitted infinity
// prints the way Number does in JavaScript.
func TestFloatConstOverflowRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const over = 1e308 * 2;
const sum = 1e308 + 1e308;
console.log(String(over));
console.log(String(sum));
`
	got := runProgramGo(t, src)
	want := "Infinity\nInfinity\n"
	if got != want {
		t.Fatalf("float overflow run mismatch:\n got %q\nwant %q", got, want)
	}
}
