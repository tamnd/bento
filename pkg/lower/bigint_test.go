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

// TestBigIntWideLiteralInternsVar pins the wide-literal form: a literal past int64
// cannot be a big.NewInt call, so it becomes a package var parsed once at init by
// value.BigIntMustParse, and two sites naming the same value share the one var
// while a different value gets its own.
func TestBigIntWideLiteralInternsVar(t *testing.T) {
	const src = `const a: bigint = 36893488147419103232n;
const b: bigint = 36893488147419103232n;
const c: bigint = 0x1ffffffffffffffffn;
console.log(a);
console.log(b);
console.log(c);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, `bigLit1 = value.BigIntMustParse("36893488147419103232")`) {
		t.Errorf("a wide bigint literal did not intern a BigIntMustParse package var:\n%s", source)
	}
	if got := strings.Count(source, `"36893488147419103232"`); got != 1 {
		t.Errorf("the same wide literal parsed %d times, want one shared var:\n%s", got, source)
	}
	if !strings.Contains(source, `bigLit2 = value.BigIntMustParse("36893488147419103231")`) {
		t.Errorf("a second wide value did not get its own interned var:\n%s", source)
	}
}

// TestBigIntAccumulatorMutatesInPlace pins the ownership optimization: a factorial
// loop whose accumulator and counter provably share their big.Int with nothing
// lowers to the in-place acc.Mul(acc, i) a person writes with math/big, with no
// fresh allocation inside the loop.
func TestBigIntAccumulatorMutatesInPlace(t *testing.T) {
	const src = `let acc: bigint = 1n;
for (let i: bigint = 2n; i <= 20n; i = i + 1n) {
  acc = acc * i;
}
console.log(acc);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "acc.Mul(acc, i)") {
		t.Errorf("an owned accumulator did not mutate in place:\n%s", source)
	}
	if !strings.Contains(source, "i.Add(i, big.NewInt(1))") {
		t.Errorf("an owned counter did not step in place:\n%s", source)
	}
	if strings.Contains(source, "new(big.Int).Mul") {
		t.Errorf("the loop still allocates a fresh big.Int per iteration:\n%s", source)
	}
}

// TestBigIntCompoundMutatesInPlace pins that a compound update of an owned local
// takes the same in-place form as the spelled-out self assignment.
func TestBigIntCompoundMutatesInPlace(t *testing.T) {
	const src = `let acc: bigint = 1n;
acc *= 3n;
acc += 2n;
acc &= 6n;
console.log(acc);
`
	source := renderProgram(t, src)
	for _, want := range []string{"acc.Mul(acc, big.NewInt(3))", "acc.Add(acc, big.NewInt(2))", "acc.And(acc, big.NewInt(6))"} {
		if !strings.Contains(source, want) {
			t.Errorf("compound bigint update did not emit %s:\n%s", want, source)
		}
	}
}

// TestBigIntAliasKeepsFreshForm pins the soundness side of the ownership analysis:
// a local whose pointer was copied into a second binding must keep the
// always-fresh assignment, since mutating it in place would be visible through the
// alias.
func TestBigIntAliasKeepsFreshForm(t *testing.T) {
	const src = `let a: bigint = 1n;
const b: bigint = a;
a = a * 2n;
console.log(a);
console.log(b);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "a.Mul(") {
		t.Errorf("an aliased bigint local mutated in place:\n%s", source)
	}
	if !strings.Contains(source, "new(big.Int).Mul") {
		t.Errorf("an aliased bigint local lost the fresh assignment form:\n%s", source)
	}
}

// TestBigIntCallEscapeKeepsFreshForm pins that passing a bigint local to a user
// function disqualifies it: the callee may retain the pointer, so the local keeps
// the fresh form.
func TestBigIntCallEscapeKeepsFreshForm(t *testing.T) {
	const src = `function peek(x: bigint): bigint {
  return x * 1n;
}
let a: bigint = 1n;
console.log(peek(a));
a = a * 2n;
console.log(a);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "a.Mul(") {
		t.Errorf("a bigint local passed to a user function mutated in place:\n%s", source)
	}
}

// TestBigIntWideInitKeepsFreshForm pins that a local initialized from a wide
// literal keeps the fresh form: the interned package var is shared by every site
// naming the value, so mutating a local that holds it would corrupt the constant.
func TestBigIntWideInitKeepsFreshForm(t *testing.T) {
	const src = `let a: bigint = 36893488147419103232n;
a = a + 1n;
console.log(a);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "a.Add(") {
		t.Errorf("a local holding a shared wide-literal var mutated in place:\n%s", source)
	}
	if !strings.Contains(source, "new(big.Int).Add") {
		t.Errorf("a wide-initialized local lost the fresh assignment form:\n%s", source)
	}
}

// TestBigIntBitwiseEmitsBigMethods pins that the bitwise operators on two bigints
// lower to the fresh method form like the arithmetic ones: big.Int's And/Or/Xor
// compute on the infinite two's complement a negative JavaScript bigint means, and
// ~ is Not, the -(x+1) with no 32-bit window.
func TestBigIntBitwiseEmitsBigMethods(t *testing.T) {
	const src = `const a: bigint = 12n;
const b: bigint = 10n;
console.log(a & b);
console.log(a | b);
console.log(a ^ b);
console.log(~a);
`
	source := renderProgram(t, src)
	for _, want := range []string{"new(big.Int).And", "new(big.Int).Or", "new(big.Int).Xor", "new(big.Int).Not"} {
		if !strings.Contains(source, want) {
			t.Errorf("bigint bitwise did not emit %s:\n%s", want, source)
		}
	}
}

// TestBigIntPowShiftEmitHelpers pins that /, %, ** and the shifts lower to the value
// helpers rather than bare big.Int methods, since each carries a throw path (a zero
// divisor, a negative exponent, the size cap) and the shifts the sign-of-count rule,
// and that a program using one defers the uncaught reporter.
func TestBigIntPowShiftEmitHelpers(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"/", "value.BigIntDiv"},
		{"%", "value.BigIntRem"},
		{"**", "value.BigIntPow"},
		{"<<", "value.BigIntLsh"},
		{">>", "value.BigIntRsh"},
	}
	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			src := "const a: bigint = 6n;\nconst b: bigint = 4n;\nconsole.log(a " + tc.op + " b);\n"
			source := renderProgram(t, src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("bigint %s did not lower to %s:\n%s", tc.op, tc.want, source)
			}
			if !strings.Contains(source, "defer value.ReportUncaught()") {
				t.Errorf("a throwing bigint operator did not defer the uncaught reporter:\n%s", source)
			}
		})
	}
}

// TestBigIntConversionsEmitHelpers pins the four conversions around bigint: BigInt
// of a number, string, and boolean map to the value helpers, Number and Boolean of
// a bigint map back, and BigInt of an integer literal folds to the big.NewInt the
// literal itself would be.
func TestBigIntConversionsEmitHelpers(t *testing.T) {
	const src = `const n: number = 5;
const s: string = "123";
const f: boolean = true;
const b: bigint = 7n;
console.log(BigInt(n));
console.log(BigInt(s));
console.log(BigInt(f));
console.log(Number(b));
console.log(Boolean(b));
console.log(BigInt(42));
`
	source := renderProgram(t, src)
	for _, want := range []string{
		"value.NumberToBigInt(n)",
		"value.StringToBigInt(s)",
		"value.BoolToBigInt(f)",
		"value.BigIntToNumber(b)",
		"value.BigIntToBool(b)",
		"big.NewInt(42)",
	} {
		if !strings.Contains(source, want) {
			t.Errorf("bigint conversion did not emit %s:\n%s", want, source)
		}
	}
	if strings.Contains(source, "value.NumberToBigInt(42)") {
		t.Errorf("BigInt of an integer literal did not fold to big.NewInt:\n%s", source)
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

// TestBigIntCompletionRuns proves this slice end to end in one program: a wide
// literal computes exactly, the in-place factorial loop produces the right value
// (which fails loudly if the ownership analysis ever mutates a shared pointer),
// an alias observes the old value after its source is reassigned, **, the shifts,
// and the bitwise family follow the JavaScript rules (arithmetic >>, reversed
// negative counts, infinite two's complement), the four conversions round-trip,
// and the two throwing conversions raise the named JavaScript errors catch sees.
func TestBigIntCompletionRuns(t *testing.T) {
	skipIfShort(t)
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go toolchain not found on PATH; the bigint test builds and runs generated Go")
	}
	const src = `const wide: bigint = 36893488147419103232n;
console.log(wide + 1n);
let acc: bigint = 1n;
for (let i: bigint = 2n; i <= 25n; i = i + 1n) {
  acc = acc * i;
}
console.log(acc);
let a: bigint = 5n;
const b: bigint = a;
a = a + 1n;
console.log(a);
console.log(b);
console.log(2n ** 64n);
console.log(-7n >> 1n);
console.log(8n << -2n);
console.log(1n << 100n);
console.log(12n & 10n);
console.log(12n | 10n);
console.log(12n ^ 10n);
console.log(~5n);
console.log(BigInt(42));
console.log(BigInt("0x10"));
console.log(BigInt("  -123  "));
console.log(BigInt(true));
console.log(Number(1n << 60n));
console.log(Boolean(0n));
console.log(String(wide));
try {
  const bad: bigint = BigInt("nope");
  console.log(bad);
} catch (e) {
  if (e instanceof SyntaxError) {
    console.log(e.name);
  }
}
try {
  const bad2: bigint = 2n ** -1n;
  console.log(bad2);
} catch (e) {
  if (e instanceof RangeError) {
    console.log(e.message);
  }
}
`
	got := runProgramGo(t, src)
	want := "36893488147419103233n\n" +
		"15511210043330985984000000n\n" +
		"6n\n" +
		"5n\n" +
		"18446744073709551616n\n" +
		"-4n\n" +
		"2n\n" +
		"1267650600228229401496703205376n\n" +
		"8n\n" +
		"14n\n" +
		"6n\n" +
		"-6n\n" +
		"42n\n" +
		"16n\n" +
		"-123n\n" +
		"1n\n" +
		"1152921504606847000\n" +
		"false\n" +
		"36893488147419103232\n" +
		"SyntaxError\n" +
		"Exponent must be non-negative\n"
	if got != want {
		t.Fatalf("bigint completion program printed %q, want %q", got, want)
	}
}
