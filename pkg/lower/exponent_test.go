package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestExponentEmits pins the shape of the lowering: ** on numbers is Math.pow,
// so it emits a value.Pow call rather than a Go operator, and **= fuses to
// x = x ** n and reaches the same call from a statement. A chained ** nests
// right, the way the source parses. value.Pow rather than math.Pow carries the
// JavaScript NaN result at a unit base with an infinite or NaN exponent.
func TestExponentEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"binary",
			"export function raise(a: number, b: number): number { return a ** b; }\n",
			"value.Pow(a, b)",
		},
		{
			"rightAssociative",
			"export function raise(a: number, b: number, c: number): number { return a ** b ** c; }\n",
			"value.Pow(a, value.Pow(b, c))",
		},
		{
			"compound",
			"export function grow(x: number, n: number): number { x **= n; return x; }\n",
			"x = value.Pow(x, n)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("exponent did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestExponentHandsBack pins the boundary: ** on a dynamic operand cannot pick
// math.Pow at compile time because the operand's runtime kind is unknown, so it
// hands back rather than coercing with machinery that does not exist yet.
func TestExponentHandsBack(t *testing.T) {
	const src = "export function raise(a: any, b: number): number { return a ** b; }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "mixed or non-primitive") {
		t.Errorf("hand-back reason = %q, want it to mention mixed or non-primitive operands", nyl.Reason)
	}
}
