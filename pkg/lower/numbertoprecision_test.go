package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestNumberToPrecisionEmits pins that n.toPrecision(p) with a literal precision
// lowers to value.NumberToPrecision with the precision folded in as an int
// literal, the same shape toExponential and toFixed take.
func TestNumberToPrecisionEmits(t *testing.T) {
	const src = "function f(x: number): string { return x.toPrecision(3); }\nconsole.log(f(123.456));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToPrecision(x, 3)") {
		t.Errorf("toPrecision did not lower to value.NumberToPrecision(x, 3):\n%s", source)
	}
}

// TestNumberToPrecisionHandsBack pins the boundaries: a non-literal precision
// cannot be range-checked at compile time, a literal zero is below the valid
// 1..100 range (zero significant digits throws), and the omitted form is
// Number::toString rather than a default, so all three hand back with named
// reasons.
func TestNumberToPrecisionHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"dynamicPrecision",
			"function f(x: number, p: number): string { return x.toPrecision(p); }\nconsole.log(f(1, 3));\n",
			"non-literal or out-of-range",
		},
		{
			"zeroPrecision",
			"function f(x: number): string { return x.toPrecision(0); }\nconsole.log(f(1));\n",
			"non-literal or out-of-range",
		},
		{
			"omittedPrecision",
			"function f(x: number): string { return x.toPrecision(); }\nconsole.log(f(1));\n",
			"primitive method .toPrecision",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := compile(t, tc.src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, tc.want) {
				t.Errorf("hand-back reason = %q, want it to contain %q", nyl.Reason, tc.want)
			}
		})
	}
}

// TestNumberToPrecisionRuns builds and runs toPrecision end to end against the
// Node oracle over both layouts: a mid-range value that keeps its point and pads
// with trailing zeros, a large value that tips into exponential notation, a small
// value that stays fixed with leading zeros, and a rounding carry, all matching
// V8 byte for byte.
func TestNumberToPrecisionRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function three(x: number): string {
  return x.toPrecision(3);
}
function five(x: number): string {
  return x.toPrecision(5);
}
function two(x: number): string {
  return x.toPrecision(2);
}
console.log(three(123.456));
console.log(five(100));
console.log(three(0.00012345));
console.log(three(123456));
console.log(two(9.99));
`
	got := runProgramGo(t, src)
	want := "123\n" +
		"100.00\n" +
		"0.000123\n" +
		"1.23e+5\n" +
		"10\n"
	if got != want {
		t.Fatalf("toPrecision program printed %q, want %q", got, want)
	}
}
