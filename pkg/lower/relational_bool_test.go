package lower

import (
	"strings"
	"testing"
)

// A relational operator on two booleans runs the Abstract Relational Comparison,
// which coerces each boolean through ToNumber (true to 1, false to 0) and compares
// the numbers. Go has no relational operator on bool, so each operand lowers through
// value.BoolToNumber and the numeric comparison applies to the two float64 results.

// TestBooleanRelationalLowers pins that a boolean relational comparison lowers to a
// numeric compare of the two coerced operands rather than handing back.
func TestBooleanRelationalLowers(t *testing.T) {
	const src = "console.log(true < false);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.BoolToNumber(true) < value.BoolToNumber(false)") {
		t.Errorf("boolean relational did not lower to a coerced numeric compare:\n%s", source)
	}
}

// TestBooleanRelationalRuns builds and runs each of the four relational operators on
// booleans and checks the result matches the ToNumber coercion JavaScript runs.
func TestBooleanRelationalRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
console.log(true < false);
console.log(false < true);
console.log(true <= true);
console.log(false <= true);
console.log(true > false);
console.log(false > true);
console.log(true >= true);
console.log(false >= true);
`
	want := "false\ntrue\ntrue\ntrue\ntrue\nfalse\ntrue\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("boolean relational printed %q, want %q", got, want)
	}
}

// TestBooleanRelationalLocalsRun proves the coercion holds when the operands are
// boolean locals rather than literals, so the path does not depend on constant
// folding.
func TestBooleanRelationalLocalsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
let a: boolean = true;
let b: boolean = false;
console.log(a < b);
console.log(a > b);
console.log(a <= a);
console.log(b >= a);
`
	want := "false\ntrue\ntrue\nfalse\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("boolean relational over locals printed %q, want %q", got, want)
	}
}
