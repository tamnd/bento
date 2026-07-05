package lower

import (
	"strings"
	"testing"
)

// A default parameter (one with a `= expr` initializer) lets a caller omit the
// argument. Go has no optional arguments, so the parameter lowers to a plain Go
// field of its type and every call fills the omitted slot with the default. The
// default must be a self-contained constant, since a default that reads a variable
// or makes a call would need the callee's parameter scope at the call site.

// TestDefaultParamFillsOmittedArg proves an omitted trailing argument lowers to the
// parameter's default at the call site, while a provided argument passes through.
func TestDefaultParamFillsOmittedArg(t *testing.T) {
	const src = "function inc(x: number, by: number = 1): number { return x + by; }\ninc(5);\ninc(5, 3);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Inc(5, 1)") {
		t.Errorf("an omitted default argument did not fill with the default:\n%s", source)
	}
	if !strings.Contains(source, "Inc(5, 3)") {
		t.Errorf("a provided argument did not pass through:\n%s", source)
	}
	if !strings.Contains(source, "func Inc(x float64, by float64)") {
		t.Errorf("the default parameter did not lower to a plain Go field:\n%s", source)
	}
}

// TestDefaultParamRuns builds and runs numeric, string, and boolean defaults so the
// filled slot is proven to carry each default's value and a provided argument to
// override it.
func TestDefaultParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function inc(x: number, by: number = 1): number {
  return x + by;
}
function greet(name: string, greeting: string = "hi"): string {
  return greeting + ", " + name;
}
function pick(x: number, on: boolean = true): number {
  return on ? x : 0;
}
console.log(inc(5));
console.log(inc(5, 3));
console.log(greet("sam"));
console.log(greet("sam", "yo"));
console.log(pick(9));
console.log(pick(9, false));
`
	want := "6\n8\nhi, sam\nyo, sam\n9\n0\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("default parameters printed %q, want %q", got, want)
	}
}

// TestDefaultParamPartialOmit proves a function with two defaults fills only the
// slots a call leaves off, so box(), box(5), and box(5, 6) each read the right pair.
func TestDefaultParamPartialOmit(t *testing.T) {
	skipIfShort(t)
	const src = `
function box(w: number = 2, h: number = 3): number {
  return w * h;
}
console.log(box());
console.log(box(5));
console.log(box(5, 6));
`
	if got, want := runProgramGo(t, src), "6\n15\n30\n"; got != want {
		t.Fatalf("partial default omission printed %q, want %q", got, want)
	}
}

// TestDefaultParamNegativeLiteral proves a signed literal default lowers, since the
// prefix minus over a literal is still a self-contained constant.
func TestDefaultParamNegativeLiteral(t *testing.T) {
	skipIfShort(t)
	const src = `
function shift(x: number, by: number = -2): number {
  return x + by;
}
console.log(shift(10));
console.log(shift(10, 5));
`
	if got, want := runProgramGo(t, src), "8\n15\n"; got != want {
		t.Fatalf("negative literal default printed %q, want %q", got, want)
	}
}

// TestDefaultParamReadingVariableHandsBack proves a default that reads a module
// binding hands back, since evaluating it needs the callee's scope at the call site.
func TestDefaultParamReadingVariableHandsBack(t *testing.T) {
	const src = "const base = 10;\nfunction f(x: number, y: number = base): number { return x + y; }\nf(1);\n"
	renderProgramHandBack(t, src)
}

// TestDefaultParamCallingFunctionHandsBack proves a default that calls a function
// hands back, since a package-level fill cannot reproduce the call at the call site.
func TestDefaultParamCallingFunctionHandsBack(t *testing.T) {
	const src = "function seed(): number { return 3; }\nfunction f(x: number, y: number = seed()): number { return x + y; }\nf(1);\n"
	renderProgramHandBack(t, src)
}

// TestOptionalParamWithoutDefaultHandsBack proves a bare `x?: T` with no default
// still hands back, since the omitted-argument case wants the undefined optional
// synthesis, a separate later slice.
func TestOptionalParamWithoutDefaultHandsBack(t *testing.T) {
	const src = "function f(x: number, y?: number): number { return x + (y ?? 0); }\nf(1);\n"
	renderProgramHandBack(t, src)
}

// TestDefaultedFuncUsedAsValueHandsBack proves a function with a default parameter
// used as a value, rather than called, hands back: its Go arity exceeds the minimal
// call, so no single func value fits a slot expecting the shorter signature.
func TestDefaultedFuncUsedAsValueHandsBack(t *testing.T) {
	const src = "function inc(x: number, by: number = 1): number { return x + by; }\nconst g = inc;\ng(5, 2);\n"
	renderProgramHandBack(t, src)
}
