package lower

import (
	"strings"
	"testing"
)

// TestDynamicBitwiseEmits pins the lowering of a bitwise operator over a dynamic
// operand: each side coerces through value.ToNumber and then the same ToInt32-based
// construction the static number path uses, so a dynamic & narrows both sides to a
// 32-bit integer, runs the Go &, and casts back to float64.
func TestDynamicBitwiseEmits(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"and coerces through ToNumber and ToInt32",
			"function f(a: any, b: any): any { return a & b; }\nconsole.log(f(6, 3));\n",
			"value.ToInt32(value.ToNumber(a))",
		},
		{
			"unsigned shift uses ToUint32 on the left",
			"function f(a: any, b: any): any { return a >>> b; }\nconsole.log(f(-1, 28));\n",
			"value.ToUint32(value.ToNumber(a))",
		},
		{
			"exponent runs value.Pow",
			"function f(a: any, b: any): any { return a ** b; }\nconsole.log(f(2, 10));\n",
			"value.Pow(value.ToNumber(a), value.ToNumber(b))",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("dynamic operator did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestDynamicBitwiseRuns builds and runs the bitwise and exponent operators over
// dynamic operands and matches Node: a numeric string coerces to a number the same
// as a number does, a negative number shifts as a signed int32 for >> and as an
// unsigned one for >>>, and ** raises to the power.
func TestDynamicBitwiseRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function and(a: any, b: any): any { return a & b; }
function or(a: any, b: any): any { return a | b; }
function xor(a: any, b: any): any { return a ^ b; }
function shl(a: any, b: any): any { return a << b; }
function shr(a: any, b: any): any { return a >> b; }
function ushr(a: any, b: any): any { return a >>> b; }
function pow(a: any, b: any): any { return a ** b; }
console.log(and(6, 3));
console.log(or(6, 1));
console.log(xor(5, 1));
console.log(shl(1, 4));
console.log(shr(-8, 1));
console.log(ushr(-1, 28));
console.log(pow(2, 10));
console.log(and("6", "3"));
`
	got := runProgramGo(t, src)
	want := "2\n7\n4\n16\n-4\n15\n1024\n2\n"
	if got != want {
		t.Fatalf("dynamic bitwise program printed %q, want %q", got, want)
	}
}
