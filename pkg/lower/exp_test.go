package lower

import (
	"strings"
	"testing"
)

// TestExponentInlinesMathPow pins that ** on two numbers lowers to math.Pow, the
// same Go function Math.pow lowers to, since a ** b is defined as Math.pow(a, b)
// and the two must not drift.
func TestExponentInlinesMathPow(t *testing.T) {
	src := "function f(a: number, b: number): number { return a ** b; }\nconsole.log(f(2, 10));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "math.Pow(a, b)") {
		t.Errorf("** did not lower to math.Pow:\n%s", source)
	}
}

// TestExponentCompoundAssign pins that the compound form a **= b desugars through
// the same path, so it assigns math.Pow(a, b) back to the operand.
func TestExponentCompoundAssign(t *testing.T) {
	src := "function f(a: number, b: number): number { let x = a; x **= b; return x; }\nconsole.log(f(3, 4));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "math.Pow(x, b)") {
		t.Errorf("**= did not lower to a math.Pow assignment:\n%s", source)
	}
}

// TestExponentRuns builds and runs exponentiation against the Node oracle, covering
// a whole power, a fractional exponent (square root), a negative exponent, the
// right-associative chain 2 ** 3 ** 2 which is 2 ** 9, and the compound form.
func TestExponentRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pow(a: number, b: number): number {
  return a ** b;
}
function chain(): number {
  return 2 ** 3 ** 2;
}
function compound(a: number, b: number): number {
  let x = a;
  x **= b;
  return x;
}
console.log(pow(2, 10));
console.log(pow(9, 0.5));
console.log(pow(2, -1));
console.log(chain());
console.log(compound(3, 4));
`
	got := runProgramGo(t, src)
	want := "1024\n3\n0.5\n512\n81\n"
	if got != want {
		t.Fatalf("exponent program printed %q, want %q", got, want)
	}
}
