package lower

import (
	"strings"
	"testing"
)

// TestArrowAnnotatedParamLowers pins that an arrow parameter with an explicit type
// annotation lowers the same as a bare one: the annotation is already folded into
// the checker's type for the name, so (n: number) reads its name off the first
// child and emits func(n float64) float64 just as n => ... does. Reading the code
// keeps the annotated form's shape visible without the toolchain.
func TestArrowAnnotatedParamLowers(t *testing.T) {
	const src = `const nums = [1, 2, 3];
const doubled = nums.map((n: number): number => n * 2);
console.log(doubled.length);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "func(n float64) float64") {
		t.Errorf("annotated arrow param did not lower to a typed func literal:\n%s", source)
	}
}

// TestArrowAnnotatedParamRuns proves the annotated form runs to the same output as
// the bare one, so the annotation changes nothing but the source a developer wrote.
func TestArrowAnnotatedParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const nums = [1, 2, 3, 4];
const doubled = nums.map((n: number): number => n * 2);
console.log(doubled.length);
console.log(doubled[3]);
`
	got := runProgramGo(t, src)
	if want := "4\n8\n"; got != want {
		t.Errorf("annotated arrow param ran to %q, want %q", got, want)
	}
}

// TestArrowDefaultParamHandsBack keeps a defaulted callback parameter a later slice. An
// inline callback's Go func type has to fit the slot the callee declares, here the
// func(float64) float64 Map takes, so the parameter cannot take the boxed slot a
// body-entry fill needs, and neither can the call site fill it: Map decides the arity.
// It routes to the interpreter rather than drop the default on the floor.
func TestArrowDefaultParamHandsBack(t *testing.T) {
	handsBack(t, "const nums = [1, 2, 3]; const out = nums.map((n = 5) => n * 2); console.log(out.length);\n")
}

// TestArrowRestParamHandsBack keeps a rest parameter in an arrow a later slice: the
// first child is the rest token, not the binding name, so the shape is not the
// plain-identifier form the lowering claims.
func TestArrowRestParamHandsBack(t *testing.T) {
	handsBack(t, "const f = (...n: number[]) => n.length; console.log(f(1, 2));\n")
}
