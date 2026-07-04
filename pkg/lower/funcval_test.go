package lower

import (
	"strings"
	"testing"
)

// TestFuncTypeParamLowersToGoFunc pins that a parameter typed as a function
// lowers to a Go func type, not the empty struct an object shape would give, so
// the callback reads with a callable Go type.
func TestFuncTypeParamLowersToGoFunc(t *testing.T) {
	src := "function apply(g: (n: number) => number, x: number): number { return g(x); }\nconsole.log(apply((n: number): number => n + 1, 2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "g func(float64) float64") {
		t.Errorf("function-typed parameter did not lower to a Go func type:\n%s", source)
	}
}

// TestCallLocalFuncValue pins that calling a local holding an arrow lowers to a
// direct Go call on the func value, not a hand-back.
func TestCallLocalFuncValue(t *testing.T) {
	src := "const f = (a: number): number => a + 1;\nconsole.log(f(2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "f(2)") {
		t.Errorf("call on a local func value did not lower:\n%s", source)
	}
}

// TestTopLevelFuncAsValue pins that a top-level function passed by name lowers
// to the exported Go name its declaration takes, so the reference and the
// declaration agree.
func TestTopLevelFuncAsValue(t *testing.T) {
	src := "function inc(n: number): number { return n + 1; }\nfunction apply(g: (n: number) => number, x: number): number { return g(x); }\nconsole.log(apply(inc, 3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Apply(Inc, 3)") {
		t.Errorf("top-level function passed by name did not lower to its exported Go name:\n%s", source)
	}
}

// TestVoidCallbackHandsBackNothing pins that a callback returning void lowers to
// a Go func with no results and its calls are plain statements.
func TestVoidCallbackRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function twice(cb: (n: number) => void, x: number): void {
  cb(x);
  cb(x);
}
function show(n: number): void {
  console.log(n);
}
twice(show, 5);
`
	got := runProgramGo(t, src)
	want := "5\n5\n"
	if got != want {
		t.Fatalf("void callback program printed %q, want %q", got, want)
	}
}

// TestCallableObjectHandsBack pins that a callable object type, a function value
// that also carries its own property, hands the unit back rather than falling
// through to the empty struct the object lowering would give a shape it reads as
// having no representable fields.
func TestCallableObjectHandsBack(t *testing.T) {
	src := "type Fn = { (n: number): number; tag: string };\nfunction use(g: Fn): number { return g(1); }\n"
	renderProgramHandBack(t, src)
}

// TestOverloadedFuncTypeHandsBack pins that a function type with more than one
// call signature (an overload set) hands back, since selecting the Go func shape
// per call site is a later slice.
func TestOverloadedFuncTypeHandsBack(t *testing.T) {
	src := "function use(g: { (n: number): number; (s: string): string }): number { return g(1); }\n"
	renderProgramHandBack(t, src)
}

// TestFuncValuesRun builds and runs higher-order code against the Node oracle: a
// local arrow called directly, a function-typed parameter applied to both an
// arrow local and a top-level function passed by name, and a top-level function
// stored in a local and called.
func TestFuncValuesRun(t *testing.T) {
	skipIfShort(t)
	const src = `const add1 = (a: number): number => a + 1;
function inc(n: number): number {
  return n + 1;
}
function apply(g: (n: number) => number, x: number): number {
  return g(x);
}
function twice(g: (n: number) => number, x: number): number {
  return g(g(x));
}
const h = inc;
console.log(add1(2));
console.log(apply(add1, 10));
console.log(apply(inc, 3));
console.log(twice(add1, 0));
console.log(h(41));
`
	got := runProgramGo(t, src)
	want := "3\n11\n4\n2\n42\n"
	if got != want {
		t.Fatalf("func-value program printed %q, want %q", got, want)
	}
}
