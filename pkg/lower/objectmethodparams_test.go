package lower

import (
	"strings"
	"testing"
)

// takeSrc is the preamble every case here shares. take's parameter is typed any, so the
// object literal passed to it has to box, which is what puts its methods through the
// value.NewFunc path rather than the struct path a statically shaped object takes.
const takeSrc = "function take(x: any): any { return x; }\n"

// TestBoxedMethodWithParametersBindsArguments pins the shape of this slice: a boxed
// object method that names parameters lowers to a wrapper that reads each argument off
// the boxed list rather than a thunk that ignores it.
func TestBoxedMethodWithParametersBindsArguments(t *testing.T) {
	src := takeSrc + "const o: any = take({ add(a: number, b: number): number { return a + b; } });\n" +
		"console.log(o.add(2, 3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ToNumber(value.Arg(__a, 0))") ||
		!strings.Contains(source, "value.ToNumber(value.Arg(__a, 1))") {
		t.Errorf("the method's parameters did not bind to the boxed argument list:\n%s", source)
	}
	if !strings.Contains(source, "value.Number(") {
		t.Errorf("the method's result did not box back into the value model:\n%s", source)
	}
}

// TestBoxedMethodWithoutParametersKeepsTheThunk pins the other side of the gate. A
// parameterless single-return method is the coercion shape (valueOf, toString,
// [Symbol.toPrimitive]) and keeps the compact closure that ignores its argument list, so
// nothing that lowered before pays for a wrapper it has no arguments to fill.
func TestBoxedMethodWithoutParametersKeepsTheThunk(t *testing.T) {
	src := takeSrc + "const o: any = take({ valueOf(): number { return 7; } });\n" +
		"console.log(o.valueOf());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(__a []value.Value) value.Value {\n\t\treturn value.Number(7)") {
		t.Errorf("a parameterless method did not keep the compact thunk:\n%s", source)
	}
	if strings.Contains(source, "value.Arg(__a") {
		t.Errorf("a parameterless method read an argument it has no parameter for:\n%s", source)
	}
}

// TestBoxedMethodWithABlockBodyLowersItsStatements pins that a method body with locals
// and control flow lowers through the same machinery an arrow's block body does, so the
// parameter is a real Go binding the statements read.
func TestBoxedMethodWithABlockBodyLowersItsStatements(t *testing.T) {
	src := takeSrc + "const o: any = take({ sum(n: number): number { let t = 0; for (let i = 0; i < n; i++) { t += i; } return t; } });\n" +
		"console.log(o.sum(4));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for i := 0.0; i < n; i++ {") {
		t.Errorf("the method's block body did not lower its loop:\n%s", source)
	}
	if !strings.Contains(source, "func(n float64) float64 {") {
		t.Errorf("the method did not lower to a typed func over its parameter:\n%s", source)
	}
}

// TestBoxedMethodLiteralUnionResultBoxes pins the result half. A body with two returns
// of different literals has a closed literal union for its return type, which lowers to
// the primitive the union folds to, so the box is that primitive's and not a hand-back
// for want of a flag the Go type already carries.
func TestBoxedMethodLiteralUnionResultBoxes(t *testing.T) {
	src := takeSrc + "const o: any = take({ size(n: number) { if (n > 2) { return \"big\"; } return \"small\"; } });\n" +
		"console.log(o.size(5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.StringValue(") {
		t.Errorf("a string-literal union result did not box as a string:\n%s", source)
	}
}

// TestBoxedMethodRestParameterHandsBack pins the boundary this slice leaves. A rest
// parameter has no fixed slot for the wrapper to fill, so it hands back with a named
// reason rather than emitting a wrapper that would drop the tail of the call.
func TestBoxedMethodRestParameterHandsBack(t *testing.T) {
	src := takeSrc + "const o: any = take({ count(...xs: number[]): number { return xs.length; } });\n" +
		"console.log(o.count(1, 2, 3));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "parameter") {
		t.Errorf("hand-back reason %q does not name the parameter shape", reason)
	}
}
