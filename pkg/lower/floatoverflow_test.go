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

// TestFloatConstDivByZeroEmitsInf pins that a constant divide by zero lowers to
// math.Inf rather than a Go constant division the compiler rejects. Go folds 1 / 0
// at compile time and errors on the zero divisor, where JavaScript evaluates it to
// Infinity, so without this the generated Go did not build.
func TestFloatConstDivByZeroEmitsInf(t *testing.T) {
	src := `const x = 1 / 0; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(1)") {
		t.Fatalf("constant divide by zero did not lower to math.Inf(1):\n%s", out)
	}
}

// TestFloatConstDivByZeroNegative pins the sign of the numerator carries, so -1 / 0
// lowers to the negative infinity JavaScript produces.
func TestFloatConstDivByZeroNegative(t *testing.T) {
	src := `const x = -1 / 0; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(-1)") {
		t.Fatalf("constant negative divide by zero did not lower to math.Inf(-1):\n%s", out)
	}
}

// TestFloatConstZeroOverZeroEmitsNaN pins that 0 / 0 lowers to math.NaN, the value
// JavaScript yields for an indeterminate divide, rather than the compile error Go
// raises on a constant zero divisor.
func TestFloatConstZeroOverZeroEmitsNaN(t *testing.T) {
	src := `const x = 0 / 0; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.NaN()") {
		t.Fatalf("constant zero over zero did not lower to math.NaN:\n%s", out)
	}
}

// TestNumberMaxValueOverflowFolds pins that arithmetic over the value package's
// named numeric constants is folded too, so Number.MAX_VALUE + Number.MAX_VALUE sees
// the overflow and lowers to math.Inf rather than a reference the compiler rejects
// as an overflowing constant. The addition tests reach this to name the boundary.
func TestNumberMaxValueOverflowFolds(t *testing.T) {
	src := `const x = Number.MAX_VALUE + Number.MAX_VALUE; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(1)") {
		t.Fatalf("Number.MAX_VALUE overflow did not fold to math.Inf(1):\n%s", out)
	}
}

// TestFloatConstDivByZeroRuns builds and runs the divide-by-zero forms so the emitted
// infinity and NaN print the way Number does in JavaScript.
func TestFloatConstDivByZeroRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const pos = 1 / 0;
const neg = -1 / 0;
const nan = 0 / 0;
console.log(String(pos));
console.log(String(neg));
console.log(String(nan));
`
	got := runProgramGo(t, src)
	want := "Infinity\n-Infinity\nNaN\n"
	if got != want {
		t.Fatalf("divide by zero run mismatch:\n got %q\nwant %q", got, want)
	}
}
