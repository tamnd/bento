package lower

import (
	"strings"
	"testing"
)

// pickSrc is the preamble every case here shares. pick answers a bigint or a number
// depending on its argument, so the checker types it number | bigint and cannot say
// which kind an operator over two of them will see. That is the disagreement the
// runtime operators exist for; a literal pair would be folded by the checker instead.
const pickSrc = "function pick(big: boolean): number | bigint { return big ? 6n : 6; }\n" +
	"const b: number | bigint = pick(true);\n"

// TestUntypedOperatorsLowerToRuntimeHelpers pins that every arithmetic and bitwise
// operator over a pair the checker cannot type routes to its value-model sibling,
// which decides the bigint and the number case with the operands in hand.
func TestUntypedOperatorsLowerToRuntimeHelpers(t *testing.T) {
	cases := []struct {
		op   string
		want string
	}{
		{"+", "value.Add("},
		{"-", "value.Sub("},
		{"*", "value.Mul("},
		{"/", "value.Div("},
		{"%", "value.Rem("},
		{"**", "value.Exponentiate("},
		{"&", "value.BitAnd("},
		{"|", "value.BitOr("},
		{"^", "value.BitXor("},
		{"<<", "value.ShiftLeft("},
		{">>", "value.ShiftRight("},
		{">>>", "value.UnsignedShiftRight("},
	}
	for _, c := range cases {
		src := pickSrc + "// @ts-ignore\nconst r: unknown = b " + c.op + " pick(true);\nconsole.log(r);\n"
		source := renderProgram(t, src)
		if !strings.Contains(source, c.want) {
			t.Errorf("%s over an untyped pair did not lower to %s:\n%s", c.op, c.want, source)
		}
	}
}

// TestUntypedOperatorBoxesBothOperands pins that each side crosses into the helper as
// a box, so the union's own ToValue carries whichever arm it is holding rather than a
// Go float64 the bigint arm has no room in.
func TestUntypedOperatorBoxesBothOperands(t *testing.T) {
	src := pickSrc + "// @ts-ignore\nconst r: unknown = b * pick(true);\nconsole.log(r);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Mul(b.ToValue(), Pick(true).ToValue())") {
		t.Errorf("the operands did not box into the runtime multiply:\n%s", source)
	}
}

// TestUntypedOperatorBindingTakesValueSlot pins the binding half: a const whose
// initializer the checker could not type takes a value.Value slot, the Go type the
// initializer already has, rather than handing back for want of a type to spell.
func TestUntypedOperatorBindingTakesValueSlot(t *testing.T) {
	src := pickSrc + "// @ts-ignore\nconst r = b * pick(true);\nconsole.log(r);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var r value.Value = value.Mul(") {
		t.Errorf("an untyped initializer did not take a value.Value slot:\n%s", source)
	}
}

// TestUntypedOperatorFunctionReturnsBox pins the result half: a function whose only
// return is such an operator hands back a value.Value, and a call of it flows into a
// dynamic slot as itself with no second boxing.
func TestUntypedOperatorFunctionReturnsBox(t *testing.T) {
	src := "// @ts-ignore\nfunction mul(x: number | bigint, y: number | bigint) { return x * y; }\n" +
		"const z: unknown = mul(3n, 4n);\nconsole.log(z);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ") value.Value {") {
		t.Errorf("the function did not take a value.Value result:\n%s", source)
	}
	if !strings.Contains(source, "z := Mul(") {
		t.Errorf("the call did not enter the dynamic binding as itself:\n%s", source)
	}
}

// TestUntypedOperatorArrowAnswersBox pins that a concise arrow over such an operator
// answers the box rather than being read as a void body and dropping its result, the
// shape a callback passed to a `() => unknown` slot takes.
func TestUntypedOperatorArrowAnswersBox(t *testing.T) {
	src := "function show(f: () => unknown): void { console.log(f()); }\n" + pickSrc +
		"// @ts-ignore\nshow(() => b - pick(true));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() value.Value {") {
		t.Errorf("the arrow did not answer a value.Value:\n%s", source)
	}
	if !strings.Contains(source, "return value.Sub(") {
		t.Errorf("the arrow dropped its result instead of returning it:\n%s", source)
	}
}

// TestTypedNumericOperatorKeepsNativeForm pins the other side of the gate: a pair the
// checker does agree on keeps the native Go operator it always had, so nothing that
// lowers today starts paying for a runtime dispatch it does not need.
func TestTypedNumericOperatorKeepsNativeForm(t *testing.T) {
	src := "function f(a: number, b: number): number { return a * b; }\nconsole.log(f(6, 7));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return a * b") {
		t.Errorf("a two-number multiply did not keep the native operator:\n%s", source)
	}
}
