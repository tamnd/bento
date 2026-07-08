package lower

import (
	"strings"
	"testing"
)

// TestUnaryNonNumberEmits pins that unary +, -, and ~ on a non-number primitive
// coerce it through the direct numeric conversion: a string parses through
// value.StringToNumber and a boolean maps through value.BoolToNumber, the readable
// forms rather than a box round trip.
func TestUnaryNonNumberEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"plus on a string",
			"function f(s: string): number { return +s; }\nconsole.log(f(\"5\"));\n",
			"value.StringToNumber(s)",
		},
		{
			"minus on a string",
			"function f(s: string): number { return -s; }\nconsole.log(f(\"5\"));\n",
			"-value.StringToNumber(s)",
		},
		{
			"bitwise not on a boolean",
			"function f(b: boolean): number { return ~b; }\nconsole.log(f(true));\n",
			"value.ToInt32(value.BoolToNumber(b))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("unary operator did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestUnaryNonNumberRuns builds and runs unary +, -, and ~ over strings and booleans
// and matches Node: a numeric string parses to its number, whitespace trims, a
// non-numeric string is NaN, a boolean maps to one or zero, and ~ complements the
// 32-bit integer.
func TestUnaryNonNumberRuns(t *testing.T) {
	skipIfShort(t)
	const src = `console.log(+"5");
console.log(-"5");
console.log(~"5");
console.log(+true);
console.log(~true);
console.log(+"3.14");
console.log(-"  10  ");
console.log(+"abc");
`
	got := runProgramGo(t, src)
	want := "5\n-5\n-6\n1\n-2\n3.14\n-10\nNaN\n"
	if got != want {
		t.Fatalf("unary coercion program printed %q, want %q", got, want)
	}
}
