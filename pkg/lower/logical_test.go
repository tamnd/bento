package lower

import (
	"strings"
	"testing"
)

// TestValueLogicalOrNumberInlines pins that || over two numbers lowers to the
// value-returning if wrapped in a func: the result is the left operand when it is
// truthy and the right otherwise, so the guard is the number truthiness test and
// the func carries the number type out of the expression.
func TestValueLogicalOrNumberInlines(t *testing.T) {
	src := "function f(x: number, y: number): number { return x || y; }\nconsole.log(f(0, 7));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() float64 {") {
		t.Errorf("value-returning || did not wrap the result in a typed func:\n%s", source)
	}
	if !strings.Contains(source, "if x != 0 && x == x {") {
		t.Errorf("value-returning || did not guard on the left operand's truthiness:\n%s", source)
	}
}

// TestValueLogicalAndNumberNegatesGuard pins that && returns the left operand when
// it is falsy, so its guard is the truthiness test negated, the parenthesized not
// Go prints over the comparison.
func TestValueLogicalAndNumberNegatesGuard(t *testing.T) {
	src := "function f(x: number, y: number): number { return x && y; }\nconsole.log(f(3, 7));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !(x != 0 && x == x) {") {
		t.Errorf("value-returning && did not negate the left operand's truthiness guard:\n%s", source)
	}
}

// TestValueLogicalOrStringInlines pins that || over two strings guards on the string
// emptiness test and carries value.BStr out of the func.
func TestValueLogicalOrStringInlines(t *testing.T) {
	src := "function f(s: string, d: string): string { return s || d; }\nconsole.log(f(\"\", \"x\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() value.BStr {") {
		t.Errorf("value-returning || over strings did not carry value.BStr:\n%s", source)
	}
	if !strings.Contains(source, "if s.Length() > 0 {") {
		t.Errorf("value-returning || over strings did not guard on emptiness:\n%s", source)
	}
}

// TestValueLogicalBooleanStaysOperator pins the boundary the other way: over two
// booleans the result is a boolean Go's own || returns with the same short-circuit,
// so it stays the operator rather than growing a func around a value it already has.
func TestValueLogicalBooleanStaysOperator(t *testing.T) {
	src := "function f(a: boolean, b: boolean): boolean { return a || b; }\nconsole.log(f(false, true));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return a || b") {
		t.Errorf("boolean || did not stay the Go operator:\n%s", source)
	}
	if strings.Contains(source, "func() bool {") {
		t.Errorf("boolean || should not wrap a func:\n%s", source)
	}
}

// TestValueLogicalPropertyLeftInlines pins that a property read counts as repeatable,
// so obj.x || d lowers without a temporary: the read is a struct field with no getter
// to fire, so naming it in both the guard and the returned value is the read repeated.
func TestValueLogicalPropertyLeftInlines(t *testing.T) {
	src := "function f(o: { x: number }, d: number): number { return o.x || d; }\nconsole.log(f({ x: 0 }, 9));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func() float64 {") {
		t.Errorf("value-returning || over a property read did not lower:\n%s", source)
	}
}

// TestValueLogicalImpureLeftHandsBack pins that a left operand with a side effect
// cannot take the no-temporary form, since it appears in both the guard and the
// returned value, so it hands the unit back for the temporary a later slice hoists.
func TestValueLogicalImpureLeftHandsBack(t *testing.T) {
	src := "function f(x: number, y: number): number { return Math.floor(x) || y; }\nconsole.log(f(2.5, 7));\n"
	renderProgramHandBack(t, src)
}

// TestValueLogicalRuns builds and runs both operators over numbers and strings and
// matches the Node oracle: || returns the left when truthy and && the left when
// falsy, so a falsy but present left like 0 or "" is returned by && and replaced by
// ||. A property-read left exercises the repeatable path end to end.
func TestValueLogicalRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function orNum(x: number, y: number): number {
  return x || y;
}
function andNum(x: number, y: number): number {
  return x && y;
}
function orStr(s: string, d: string): string {
  return s || d;
}
function andStr(s: string, d: string): string {
  return s && d;
}
function orProp(o: { x: number }, d: number): number {
  return o.x || d;
}
console.log(orNum(0, 7));
console.log(orNum(3, 7));
console.log(andNum(0, 7));
console.log(andNum(3, 7));
console.log(orStr("", "d"));
console.log(orStr("a", "d"));
console.log(andStr("", "d"));
console.log(andStr("a", "d"));
console.log(orProp({ x: 0 }, 9));
console.log(orProp({ x: 4 }, 9));
`
	got := runProgramGo(t, src)
	want := "7\n3\n0\n7\nd\na\n\nd\n9\n4\n"
	if got != want {
		t.Fatalf("value-logical program printed %q, want %q", got, want)
	}
}
