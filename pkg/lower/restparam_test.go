package lower

import (
	"strings"
	"testing"
)

// A rest parameter gathers the trailing arguments into an array. Go has no rest
// parameter, so it lowers to a final field of the parameter's *value.Array[T] type,
// and each call packs its extra arguments into that array at the call site. The
// body reads the parameter as an ordinary array, so only the call convention
// differs from a plain array parameter.

// TestRestParamGathersArgs proves the trailing arguments pack into a value.NewArray
// of the element type and the parameter lowers to a single array field.
func TestRestParamGathersArgs(t *testing.T) {
	const src = "function sum(...ns: number[]): number { return ns.length; }\nsum(1, 2, 3);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Sum(ns *value.Array[float64])") {
		t.Errorf("rest parameter did not lower to a single array field:\n%s", source)
	}
	if !strings.Contains(source, "Sum(value.NewArray[float64](1, 2, 3))") {
		t.Errorf("call did not gather its arguments into the rest array:\n%s", source)
	}
}

// TestRestParamEmptyCall proves a call that passes no rest arguments gathers an
// empty array, so the callee always receives an array to iterate.
func TestRestParamEmptyCall(t *testing.T) {
	const src = "function sum(...ns: number[]): number { return ns.length; }\nsum();\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Sum(value.NewArray[float64]())") {
		t.Errorf("an empty rest call did not gather an empty array:\n%s", source)
	}
}

// TestRestParamRuns builds and runs a rest sum so the gathered array is proven to
// carry every trailing argument, including the empty and single-argument cases.
func TestRestParamRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function sum(...ns: number[]): number {
  let t = 0;
  for (const n of ns) t = t + n;
  return t;
}
console.log(sum(1, 2, 3, 4));
console.log(sum());
console.log(sum(10));
`
	if got, want := runProgramGo(t, src), "10\n0\n10\n"; got != want {
		t.Fatalf("rest sum printed %q, want %q", got, want)
	}
}

// TestRestParamAfterFixed proves a fixed parameter before the rest keeps its
// position while the rest gathers only the arguments past it.
func TestRestParamAfterFixed(t *testing.T) {
	skipIfShort(t)
	const src = `
function join(sep: string, ...parts: string[]): string {
  let s = "";
  for (let i = 0; i < parts.length; i++) {
    if (i > 0) s = s + sep;
    s = s + parts[i];
  }
  return s;
}
console.log(join("-", "a", "b", "c"));
console.log(join("-"));
`
	if got, want := runProgramGo(t, src), "a-b-c\n\n"; got != want {
		t.Fatalf("fixed-then-rest join printed %q, want %q", got, want)
	}
}

// TestRestParamWithDefaultFixed proves a rest composes with a defaulted fixed
// parameter: an omitted default fills its slot and the rest gathers the remainder.
func TestRestParamWithDefaultFixed(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number = 10, ...rest: number[]): number {
  let t = a + b;
  for (const r of rest) t = t + r;
  return t;
}
console.log(f(1));
console.log(f(1, 2));
console.log(f(1, 2, 3, 4));
`
	if got, want := runProgramGo(t, src), "11\n3\n10\n"; got != want {
		t.Fatalf("default-then-rest printed %q, want %q", got, want)
	}
}

// TestSpreadIntoRestHandsBack proves a spread argument into a rest parameter hands
// back, since packing a spread source into the gather waits on the spread slice.
func TestSpreadIntoRestHandsBack(t *testing.T) {
	const src = "function sum(...ns: number[]): number { return ns.length; }\nconst xs = [1, 2];\nsum(...xs);\n"
	renderProgramHandBack(t, src)
}

// TestRestFuncUsedAsValueHandsBack proves a function with a rest parameter used as a
// value hands back, since its gathered call convention has no single func value.
func TestRestFuncUsedAsValueHandsBack(t *testing.T) {
	const src = "function sum(...ns: number[]): number { return ns.length; }\nconst g = sum;\ng(1, 2);\n"
	renderProgramHandBack(t, src)
}

// A rest-parameter function type lowers to a plain Go func value whose trailing
// argument is the *value.Array[T] a rest parameter gathers, not a Go variadic, so a
// callback typed (...a: T[]) => R reads as one array field and the call site packs
// its trailing arguments into the array. A pure rest-parameter function (a rest, no
// defaults) fits such a slot directly, since its own Go form is that same shape.

// TestRestFuncTypeParameterLowers proves a parameter typed as a rest-parameter
// function lowers to a Go func with one trailing array field, a pure rest-parameter
// function passes into it by name, and a call through the value packs its arguments.
func TestRestFuncTypeParameterLowers(t *testing.T) {
	const src = "function total(...xs: number[]): number {\n" +
		"  let s = 0;\n" +
		"  for (const x of xs) { s = s + x; }\n" +
		"  return s;\n" +
		"}\n" +
		"function run(f: (...a: number[]) => number): number { return f(1, 2, 3); }\n" +
		"console.log(run(total));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "func Run(f func(*value.Array[float64]) float64) float64") {
		t.Errorf("the rest-parameter function type did not lower to a trailing array field:\n%s", source)
	}
	if !strings.Contains(source, "return f(value.NewArray[float64](1, 2, 3))") {
		t.Errorf("the call through the func value did not pack its arguments into the array:\n%s", source)
	}
	if !strings.Contains(source, "Run(Total)") {
		t.Errorf("the pure rest-parameter function was not passed by name:\n%s", source)
	}
}

// TestRestFuncTypeCallbackRuns builds and runs a rest-parameter callback so the
// lowered Go is proven against the JavaScript result.
func TestRestFuncTypeCallbackRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function total(...xs: number[]): number {\n" +
		"  let s = 0;\n" +
		"  for (const x of xs) { s = s + x; }\n" +
		"  return s;\n" +
		"}\n" +
		"function run(f: (...a: number[]) => number): number { return f(1, 2, 3); }\n" +
		"console.log(run(total));\n"
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("rest-parameter callback printed %q, want %q", got, want)
	}
}

// TestRestFuncToNonRestSlotHandsBack proves a pure rest-parameter function passed to
// a fixed-arity func-typed slot hands back rather than emit Go that does not compile.
// TypeScript accepts the assignment, but the function's Go form takes one array field
// where the fixed-arity slot takes two floats, so only a rest-typed slot fits it and
// this one keeps the value path's handback.
func TestRestFuncToNonRestSlotHandsBack(t *testing.T) {
	const src = "function total(...xs: number[]): number { return xs.length; }\n" +
		"function run(f: (a: number, b: number) => number): number { return f(1, 2); }\n" +
		"run(total);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "defaulting wrapper") {
		t.Fatalf("rest-to-fixed hand-back reason = %q, want the value-path wrapper reason", reason)
	}
}
