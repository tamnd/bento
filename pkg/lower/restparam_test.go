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
