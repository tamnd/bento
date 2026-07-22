package lower

import (
	"strings"
	"testing"
)

// A string or boolean operand of an arithmetic operator, `"5" - 1` or `true * 3`, is
// one JavaScript runs by coercing through ToNumber but TypeScript rejects with 2362
// (left-hand side) or 2363 (right-hand side): the operand is not a number, bigint, or
// any. The ahead-of-time path compiles TypeScript, so the checker's rejection stands
// and the honest outcome is a hand-back, not the running ToNumber coercion the earlier
// tolerant slice emitted. The hand-back keys on the checker's 2362/2363 span, so it
// fires only where the checker actually reported the error: a .js source, which the
// checker does not flag, would keep the coercion once the AOT path admits .js.

// TestStringOperandArithHandsBack pins that a numeric string subtracted from a number,
// `"5" - 1`, hands the whole file back rather than emit the ToNumber coercion, since
// the checker rejects the string operand with 2362.
func TestStringOperandArithHandsBack(t *testing.T) {
	const src = `let s = "5";
let n = 1;
let r = s - n;
`
	assertArithHandsBack(t, src)
}

// TestBooleanOperandArithHandsBack pins that a boolean arithmetic operand, `true * 3`,
// hands back the same way: a boolean is not a number-typed operand the checker accepts
// in an arithmetic operation.
func TestBooleanOperandArithHandsBack(t *testing.T) {
	const src = `let t = true;
let n = 3;
let r = t * n;
`
	assertArithHandsBack(t, src)
}

// TestStringRemainderExponentHandsBack pins that % and **, the operators that lower to
// math.Mod and value.Pow rather than a plain Go operator, hand back on a string operand
// too, so no arithmetic operator slips a checker-rejected string operand through.
func TestStringRemainderExponentHandsBack(t *testing.T) {
	const src = `let a = "7";
let r = a % 3;
let s = a ** 3;
`
	assertArithHandsBack(t, src)
}

// TestStringBitwiseHandsBack pins that a bitwise operator, which coerces its operands
// to int32, hands back on a string operand as well: `"6" & 3` is a 2363 the checker
// rejects, so it does not emit the int32 coercion.
func TestStringBitwiseHandsBack(t *testing.T) {
	const src = `let a = "6";
let r = a & 3;
`
	assertArithHandsBack(t, src)
}

// assertArithHandsBack renders src through the tolerant front door and fails unless the
// whole unit hands back, the outcome an arithmetic operator with a string or boolean
// operand must take now the checker's 2362/2363 rejection is honored.
func assertArithHandsBack(t *testing.T, src string) {
	t.Helper()
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatalf("arithmetic on a string or boolean operand lowered, want a hand-back:\n%s", src)
	}
}

// TestPureNumberArithNotCoerced pins that the hand-back does not over-fire: a plain
// two-number subtraction carries no 2362/2363, so it stays the direct Go operator, never
// routes through value.StringToNumber, and never hands back.
func TestPureNumberArithNotCoerced(t *testing.T) {
	const src = `let a = 5;
let b = 1;
console.log(a - b);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "value.StringToNumber") || strings.Contains(source, "value.BoolToNumber") {
		t.Errorf("two-number arithmetic was needlessly coerced:\n%s", source)
	}
}

// TestObjectOperandArithHandsBack pins the zero-fail boundary: an object operand of an
// arithmetic operator is not a number-coercible primitive, so the operator hands back
// rather than emit Go the operator cannot take on a struct pointer. It hands back
// whether or not the checker also reported a 2362/2363, so the boundary holds
// independently of the diagnostic-driven arithmetic guard.
func TestObjectOperandArithHandsBack(t *testing.T) {
	const src = `let o = { v: 1 };
let s = "5";
let r = s * o;
`
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("object arithmetic operand lowered, want a hand-back")
	}
}
