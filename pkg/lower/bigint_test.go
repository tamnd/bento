package lower

import (
	"os/exec"
	"strings"
	"testing"
)

// TestBigIntLiteralEmitsNewInt pins that a bigint literal lowers to a big.NewInt
// call, not a float64 or a value box: the typed side of a bigint is a *big.Int, so
// 10n is big.NewInt(10) and the math/big import is pulled in.
func TestBigIntLiteralEmitsNewInt(t *testing.T) {
	const src = `const x: bigint = 10n;
console.log(x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "big.NewInt(10)") {
		t.Errorf("a bigint literal did not lower to big.NewInt:\n%s", source)
	}
	if !strings.Contains(source, `"math/big"`) {
		t.Errorf("a bigint literal did not pull in the math/big import:\n%s", source)
	}
}

// TestBigIntArithmeticEmitsBigMethods pins that the four common arithmetic
// operators on two bigints lower to the fresh-big.Int method form, never a Go
// operator, so a shared operand is never mutated in place.
func TestBigIntArithmeticEmitsBigMethods(t *testing.T) {
	cases := []struct {
		op     string
		method string
	}{
		{"+", "new(big.Int).Add"},
		{"-", "new(big.Int).Sub"},
		{"*", "new(big.Int).Mul"},
		{"/", "new(big.Int).Quo"},
		{"%", "new(big.Int).Rem"},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			src := "const a: bigint = 6n;\nconst b: bigint = 4n;\nconsole.log(a " + tc.op + " b);\n"
			source := renderProgram(t, src)
			if !strings.Contains(source, tc.method) {
				t.Errorf("bigint %s did not lower to %s:\n%s", tc.op, tc.method, source)
			}
		})
	}
}

// TestBigIntComparisonEmitsCmp pins that a relational or equality operator on two
// bigints lowers to a Cmp against zero, the *big.Int way to compare, rather than a
// Go operator on a pointer.
func TestBigIntComparisonEmitsCmp(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"<", ".Cmp(b) < 0"},
		{"<=", ".Cmp(b) <= 0"},
		{">", ".Cmp(b) > 0"},
		{">=", ".Cmp(b) >= 0"},
		{"===", ".Cmp(b) == 0"},
		{"!==", ".Cmp(b) != 0"},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			src := "const a: bigint = 6n;\nconst b: bigint = 4n;\nconsole.log(a " + tc.op + " b);\n"
			source := renderProgram(t, src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("bigint %s did not lower to a Cmp %q:\n%s", tc.op, tc.want, source)
			}
		})
	}
}

// TestBigIntNegationEmitsNeg pins that unary minus on a bigint lowers to a fresh
// new(big.Int).Neg, not a Go unary minus on a pointer.
func TestBigIntNegationEmitsNeg(t *testing.T) {
	const src = `const a: bigint = 6n;
console.log(-a);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "new(big.Int).Neg") {
		t.Errorf("bigint negation did not lower to new(big.Int).Neg:\n%s", source)
	}
}

// TestBigIntConsoleAddsSuffix pins the inspector quirk: console.log(10n) prints the
// digits with a trailing "n", so the console argument lowers through
// value.BigIntToConsole, not the plain BigIntToString.
func TestBigIntConsoleAddsSuffix(t *testing.T) {
	const src = `const a: bigint = 10n;
console.log(a);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BigIntToConsole") {
		t.Errorf("console.log of a bigint did not lower to value.BigIntToConsole:\n%s", source)
	}
}

// TestBigIntStringNoSuffix pins that String(10n) and a template stay the bare
// digits with no suffix, lowering through value.BigIntToString.
func TestBigIntStringNoSuffix(t *testing.T) {
	const src = `const a: bigint = 10n;
console.log(String(a));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BigIntToString") {
		t.Errorf("String of a bigint did not lower to value.BigIntToString:\n%s", source)
	}
}

// TestBigIntRuns proves the whole bigint path end to end: an arbitrary-precision
// sum past the safe-integer range stays exact, the division truncates toward zero
// the way BigInt / does, a comparison yields a boolean, and console.log adds the
// "n" while String and concatenation do not.
func TestBigIntRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the bigint test builds and runs generated Go")
	}
	const src = `const a: bigint = 9007199254740993n;
const b: bigint = 2n;
console.log(a + b);
console.log(7n / 2n);
console.log(-7n % 3n);
console.log(a > b);
console.log(String(10n));
console.log(10n + "x");
`
	got := runProgramGo(t, src)
	want := "9007199254740995n\n3n\n-1n\ntrue\n10\n10x\n"
	if got != want {
		t.Fatalf("bigint program printed %q, want %q", got, want)
	}
}
