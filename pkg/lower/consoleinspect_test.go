package lower

import (
	"strings"
	"testing"
)

// console.log does not coerce its argument to a string, it inspects it, and the two
// only agree on the primitives. These pin the routing that sends everything else to
// the inspector: an object read as "[object Object]" is the shape of the bug, and a
// static object literal did not even build before this, since a string coercion of
// an object hands back.

// TestConsoleObjectLiteralInspects pins the reproducer. console.log({ a: 1 }) is a
// line most programs have, and it used to hand the whole build back.
func TestConsoleObjectLiteralInspects(t *testing.T) {
	const src = `console.log({ a: 1, b: "x" });
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ConsoleValue") {
		t.Errorf("console.log of an object literal did not lower through the inspector:\n%s", source)
	}
}

// TestConsoleObjectBindingInspects pins the same for a named object rather than a
// literal, which is the form that used to build and print "[object Object]".
func TestConsoleObjectBindingInspects(t *testing.T) {
	const src = `const o = { a: 1 };
console.log(o);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ConsoleValue") {
		t.Errorf("console.log of an object binding did not lower through the inspector:\n%s", source)
	}
}

// TestConsoleArrayInspects pins the array case, which Node prints as "[ 1, 2, 3 ]"
// with its brackets and spaces rather than as the bare comma-joined string an
// Array.prototype.toString coercion gives.
func TestConsoleArrayInspects(t *testing.T) {
	const src = `const nums = [1, 2, 3];
console.log(nums);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ConsoleValue") {
		t.Errorf("console.log of an array did not lower through the inspector:\n%s", source)
	}
}

// TestConsoleNumberKeepsTheSign pins the one primitive where the console and the
// string coercion part ways: console.log(-0) prints "-0" and String(-0) is "0".
func TestConsoleNumberKeepsTheSign(t *testing.T) {
	const src = `const n = 1;
console.log(n);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NumberToConsole") {
		t.Errorf("console.log of a number did not lower through the console renderer:\n%s", source)
	}
}

// TestConsoleStringStaysRaw pins the argument the console does not inspect. A
// logged string is its own text, so it must not pick up the quotes the inspector
// puts around a nested one.
func TestConsoleStringStaysRaw(t *testing.T) {
	const src = `const s = "hi";
console.log(s);
`
	source := renderProgram(t, src)
	if strings.Contains(source, "value.ConsoleValue") {
		t.Errorf("console.log of a string went through the inspector:\n%s", source)
	}
}

// TestConsoleRegExpStaysDirect pins that a regexp keeps its direct lowering. Its
// console form and its string form are the same "/ab+c/gi", so boxing it would buy
// nothing and cost an allocation on every logged pattern.
func TestConsoleRegExpStaysDirect(t *testing.T) {
	const src = `const re = /ab+c/gi;
console.log(re);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "ToStringBStr") {
		t.Errorf("console.log of a regexp did not lower to its own string form:\n%s", source)
	}
}

// TestConsoleFunctionKeepsItsName pins the name on the box. A boxed function used
// to lose it, so a logged callback read as "[Function (anonymous)]" whatever it was
// called, and a .name read off a boxed function answered nothing.
func TestConsoleFunctionKeepsItsName(t *testing.T) {
	const src = `function foo(a: number): number { return a; }
console.log(foo);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.WithName(value.NewFunc(`) {
		t.Errorf("a boxed function did not carry its name:\n%s", source)
	}
}

// TestConsoleNamedFunctionExpressionKeepsItsName is the name a function expression
// carries when it is written where it is boxed. A reference to a binding takes the
// binding's name, which the test above covers, but a function expression written
// inline has a name only if one is spelled on it, and that one is the name JavaScript
// gives it whether or not it is ever bound. An anonymous literal in the same position
// stays anonymous, which is the other half of the rule.
func TestConsoleNamedFunctionExpressionKeepsItsName(t *testing.T) {
	skipIfShort(t)
	const src = `console.log(function keep(a: number): number { return a; });
console.log(function (a: number): number { return a; });
`
	if got, want := runProgramGo(t, src), "[Function: keep]\n[Function (anonymous)]\n"; got != want {
		t.Errorf("logging function expressions printed %q, want %q", got, want)
	}
}
