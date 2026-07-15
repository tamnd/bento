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

// TestDefaultParamModuleReadLowers proves a default that reads a module binding or
// calls a top-level function now lowers, and its read lands at the omitting call
// site rather than in the function signature. The binding is hoisted to a package
// var and a top-level function is package-visible, so the call site sees the same
// value the callee scope would.
func TestDefaultParamModuleReadLowers(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"moduleVar",
			"const base = 10;\nfunction f(n: number = base): number { return n; }\nconsole.log(f());\n",
			"F(base)",
		},
		{
			"moduleCall",
			"function d(): number { return 42; }\nfunction f(n: number = d()): number { return n; }\nconsole.log(f());\n",
			"F(D())",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := renderProgram(t, tc.src)
			if !strings.Contains(source, tc.want) {
				t.Errorf("default parameter fill did not print %q:\n%s", tc.want, source)
			}
		})
	}
}

// TestDefaultParamModuleReadRuns builds and runs a module-var default and a
// top-level-call default, both omitted and supplied, so the filled value is proven
// against the JavaScript result rather than just the emitted shape.
func TestDefaultParamModuleReadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const base = 10;
function d(): number {
  return 42;
}
function f(n: number = base): number {
  return n;
}
function g(n: number = d()): number {
  return n;
}
console.log(f());
console.log(f(3));
console.log(g());
console.log(g(7));
`
	if got, want := runProgramGo(t, src), "10\n3\n42\n7\n"; got != want {
		t.Fatalf("default parameter program printed %q, want %q", got, want)
	}
}

// TestDefaultParamReadingEarlierParamLowers proves the one default form the call
// site cannot reconstruct now lowers through a callee-scope variadic tail: the
// optional parameters collapse to one Go variadic and the body fills each from the
// variadic or its default, evaluated where the earlier parameter is in scope.
func TestDefaultParamReadingEarlierParamLowers(t *testing.T) {
	const src = "function f(a: number, b: number = a + 1): number { return a + b; }\nconsole.log(f(5));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "...float64)") {
		t.Errorf("the optional tail did not collapse to a Go variadic:\n%s", source)
	}
	if !strings.Contains(source, "var b float64") {
		t.Errorf("the optional parameter did not become a callee-scope local:\n%s", source)
	}
	if !strings.Contains(source, "b = a + 1") {
		t.Errorf("the default was not filled in the callee scope reading the earlier parameter:\n%s", source)
	}
}

// TestDefaultParamReadingEarlierParamRuns builds and runs a default that reads an
// earlier parameter, both omitted and supplied, so the filled value is proven
// against the JavaScript result.
func TestDefaultParamReadingEarlierParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number = a + 1): number {
  return a + b;
}
console.log(f(5));
console.log(f(5, 2));
`
	if got, want := runProgramGo(t, src), "11\n7\n"; got != want {
		t.Fatalf("default reading an earlier parameter printed %q, want %q", got, want)
	}
}

// TestDefaultParamReadingEarlierParamChainRuns proves a chain of optional defaults,
// each reading the one before it, fills left to right in the callee scope so a later
// default sees the earlier one already settled.
func TestDefaultParamReadingEarlierParamChainRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number = a + 1, c: number = b + 1): number {
  return a + b + c;
}
console.log(f(1));
console.log(f(1, 5));
console.log(f(1, 5, 9));
`
	if got, want := runProgramGo(t, src), "6\n12\n15\n"; got != want {
		t.Fatalf("chained defaults printed %q, want %q", got, want)
	}
}

// TestOptionalParamWithoutDefaultRuns proves a bare `x?: T` with no default lowers
// to a value.Opt[T] field: a supplied argument wraps in Some, an omitted one fills
// None, and the body reads the option, here through the nullish default y ?? 0.
func TestOptionalParamWithoutDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f(x: number, y?: number): number { return x + (y ?? 0); }\nconsole.log(f(1));\nconsole.log(f(1, 4));\n"
	if got, want := runProgramGo(t, src), "1\n5\n"; got != want {
		t.Fatalf("optional parameter printed %q, want %q", got, want)
	}
}

// TestDefaultedFuncUsedAsValueHandsBack proves a function with a default parameter
// used as a value, rather than called, hands back: its Go arity exceeds the minimal
// call, so no single func value fits a slot expecting the shorter signature.
func TestDefaultedFuncUsedAsValueHandsBack(t *testing.T) {
	const src = "function inc(x: number, by: number = 1): number { return x + by; }\nconst g = inc;\ng(5, 2);\n"
	renderProgramHandBack(t, src)
}
