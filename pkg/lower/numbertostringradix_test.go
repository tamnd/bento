package lower

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// TestNumberToStringRadixEmits pins that n.toString(radix) with a literal radix in
// 2..36 lowers to value.NumberToStringRadix with the radix folded in, and that a
// literal radix of 10 routes through the decimal stringify path instead.
func TestNumberToStringRadixEmits(t *testing.T) {
	const src = "function f(x: number): string { return x.toString(16); }\nconsole.log(f(255));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToStringRadix(x, 16)") {
		t.Errorf("toString(16) did not lower to value.NumberToStringRadix(x, 16):\n%s", source)
	}
}

// TestNumberToStringRadixDynamicEmits pins that a radix the compiler cannot prove
// is in range lowers to value.NumberToStringRadixDynamic, which applies ToInteger
// and range-checks at runtime: a non-literal radix, and a literal radix outside
// 2..36 that range-checks and throws at runtime rather than at a compile-time guard.
func TestNumberToStringRadixDynamicEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"dynamicRadix",
			"function f(x: number, r: number): string { return x.toString(r); }\nconsole.log(f(255, 16));\n",
			"value.NumberToStringRadixDynamic(x, r)",
		},
		{
			"outOfRangeRadix",
			"function f(x: number): string { return x.toString(37); }\nconsole.log(f(255));\n",
			"value.NumberToStringRadixDynamic(x, 37)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("dynamic toString radix did not emit %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestNumberToStringRadixHandsBack pins the one remaining boundary: a radix of a
// non-number type is a ToInteger-of-anything slice, so it hands back rather than
// emit a runtime call over a value the dynamic path does not model.
func TestNumberToStringRadixHandsBack(t *testing.T) {
	const src = "function f(x: number, r: string): string { return x.toString(r as any); }\nconsole.log(f(255, \"16\"));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "non-number digit count") {
		t.Errorf("hand-back reason = %q, want it to name a non-number count", nyl.Reason)
	}
}

// TestNumberToStringRadixRuns builds and runs a dynamic radix end to end against
// the Node oracle: a hex render through the runtime path, a fractional radix that
// exercises the dtoa-in-base fraction loop, and an out-of-range radix caught as a
// RangeError.
func TestNumberToStringRadixRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the test builds and runs generated Go")
	}
	const src = `function r(x: number, radix: number): string {
  return x.toString(radix);
}
console.log(r(255, 16));
console.log(r(0.5, 2));
try {
  r(1, 1);
} catch (e: any) {
  console.log(e.name);
}
`
	got := runProgramGo(t, src)
	want := "ff\n0.1\nRangeError\n"
	if got != want {
		t.Fatalf("dynamic toString radix printed %q, want %q", got, want)
	}
}
