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

// TestConstIntArithFoldsToFloat pins that arithmetic over two integer-spelled number
// literals folds to a float64 literal, not the Go integer expression that := would
// infer as int. 5 + 3 has to read as 8.0 so the local is float64 like every JavaScript
// number, where 5 + 3 alone would make a Go int and fail wherever a float64 is wanted.
func TestConstIntArithFoldsToFloat(t *testing.T) {
	src := `const x = 5 + 3; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "8.0") {
		t.Fatalf("constant integer sum did not fold to a float literal:\n%s", out)
	}
	if strings.Contains(out, "5 + 3") {
		t.Fatalf("constant integer sum kept its Go integer expression:\n%s", out)
	}
}

// TestConstIntDivFoldsToRealQuotient pins that a constant divide folds to the real
// float64 quotient JavaScript computes, so 7 / 2 is 3.5 rather than the Go integer
// division 3 that a constant 7 / 2 folds to. This is a value bug, not only a type one.
func TestConstIntDivFoldsToRealQuotient(t *testing.T) {
	src := `const x = 7 / 2; console.log(String(x));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "3.5") {
		t.Fatalf("constant divide did not fold to its float quotient:\n%s", out)
	}
}

// TestConstIntArithFoldsRuns builds and runs a spread of integer-literal arithmetic so
// the folded float64 prints the plain number JavaScript does, with the chained divide
// and the fractional quotient carrying their real values.
func TestConstIntArithFoldsRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const a = 5 + 3;
const b = 18 / 2 / 9;
const c = 7 / 2;
const d = 6 * 2;
console.log(String(a));
console.log(String(b));
console.log(String(c));
console.log(String(d));
`
	got := runProgramGo(t, src)
	want := "8\n1\n3.5\n12\n"
	if got != want {
		t.Fatalf("constant arithmetic run mismatch:\n got %q\nwant %q", got, want)
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

// TestConstModulusZeroDivisorFolds pins that a divide by a constant modulus that
// evaluates to zero folds to math.Inf, not the Go constant division by zero the
// compiler rejects. The remainder path wraps its Go % in a float64 cast, so 1 % 1
// lowers to float64((1)%1), a Go constant zero, and 1 / (1 % 1) would be a build
// error without the fold seeing through the cast to evaluate the modulus.
func TestConstModulusZeroDivisorFolds(t *testing.T) {
	src := `console.log(String(1 / (1 % 1)));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "math.Inf(1)") {
		t.Fatalf("divide by a constant zero modulus did not fold to math.Inf(1):\n%s", out)
	}
}

// TestConstModulusZeroDivisorRuns builds and runs the constant-modulus divisor forms
// so the folded infinity prints the way Number does, with the numerator's sign and
// the dividend-zero case both carrying the value JavaScript yields.
func TestConstModulusZeroDivisorRuns(t *testing.T) {
	skipIfShort(t)
	src := `
console.log(String(1 / (1 % 1)));
console.log(String(1 / (0 % 1)));
console.log(String(-1 / (1 % 1)));
`
	got := runProgramGo(t, src)
	want := "Infinity\nInfinity\n-Infinity\n"
	if got != want {
		t.Fatalf("constant modulus divisor run mismatch:\n got %q\nwant %q", got, want)
	}
}
