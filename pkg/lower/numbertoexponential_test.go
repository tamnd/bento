package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestNumberToExponentialEmits pins that n.toExponential(d) with a literal digit
// count lowers to value.NumberToExponential with the count folded in as an int
// literal, the same shape toFixed takes to value.NumberToFixed.
func TestNumberToExponentialEmits(t *testing.T) {
	const src = "function f(x: number): string { return x.toExponential(2); }\nconsole.log(f(1234.5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToExponential(x, 2)") {
		t.Errorf("toExponential did not lower to value.NumberToExponential(x, 2):\n%s", source)
	}
}

// TestNumberToExponentialDynamicEmits pins that a non-literal number count lowers
// to value.NumberToExponentialDynamic, which applies ToInteger and range-checks at
// runtime, rather than handing back the way a literal-in-range count folds to the
// exact formatter.
func TestNumberToExponentialDynamicEmits(t *testing.T) {
	const src = "function f(x: number, d: number): string { return x.toExponential(d); }\nconsole.log(f(1, 2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToExponentialDynamic(x, d)") {
		t.Errorf("dynamic toExponential did not lower to value.NumberToExponentialDynamic(x, d):\n%s", source)
	}
}

// TestNumberToExponentialHandsBack pins the two remaining boundaries: a count of a
// non-number type is a ToInteger-of-anything slice, and the omitted count is a
// different rule than toFixed's zero default, so both hand back with named reasons.
func TestNumberToExponentialHandsBack(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"nonNumberCount",
			"function f(x: number, d: string): string { return x.toExponential(d as any); }\nconsole.log(f(1, \"2\"));\n",
			"non-number digit count",
		},
		{
			"omittedCount",
			"function f(x: number): string { return x.toExponential(); }\nconsole.log(f(1));\n",
			"primitive method .toExponential",
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

// TestNumberToExponentialRuns builds and runs toExponential end to end against
// the Node oracle: a value that rounds cleanly, one whose rounding carries a nine
// into a new place and lifts the exponent, a small value with a negative
// exponent, and the zero-digit and negative-number forms all match V8 byte for
// byte, including the signed no-leading-zero exponent Go's strconv would not
// print.
func TestNumberToExponentialRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function two(x: number): string {
  return x.toExponential(2);
}
function one(x: number): string {
  return x.toExponential(1);
}
function zero(x: number): string {
  return x.toExponential(0);
}
function three(x: number): string {
  return x.toExponential(3);
}
console.log(two(1234.5));
console.log(one(9.99));
console.log(two(0.0001234));
console.log(zero(5));
console.log(one(-0.5));
console.log(three(0));
`
	got := runProgramGo(t, src)
	want := "1.23e+3\n" +
		"1.0e+1\n" +
		"1.23e-4\n" +
		"5e+0\n" +
		"-5.0e-1\n" +
		"0.000e+0\n"
	if got != want {
		t.Fatalf("toExponential program printed %q, want %q", got, want)
	}
}
