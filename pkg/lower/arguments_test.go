package lower

import (
	"strings"
	"testing"
)

// A function body that reads arguments materializes a *value.Array[value.Value]
// store from its parameters at body entry, and arguments.length reads that store's
// count. The store stands in for the passed arguments because the checker forces
// every call to an all-required, rest-free signature to pass exactly one argument
// per parameter, so the parameter count is the call arity and the i-th parameter is
// the i-th argument at every call.

// TestArgumentsLengthMaterializesStore proves a body reading arguments.length
// materializes the backing store from its parameters and reads the count off it.
func TestArgumentsLengthMaterializesStore(t *testing.T) {
	const src = "function f(a: number, b: number): number { return arguments.length; }\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(a), value.Number(b))") {
		t.Errorf("the arguments store was not materialized from the parameters:\n%s", source)
	}
	if !strings.Contains(source, ".Len()") {
		t.Errorf("arguments.length did not read the store count:\n%s", source)
	}
}

// TestArgumentsLengthRuns builds and runs a function that returns arguments.length
// so the parameter-backed count is proven against the JavaScript result.
func TestArgumentsLengthRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number): number {
  return arguments.length;
}
function g(x: number): number {
  return arguments.length;
}
console.log(f(1, 2));
console.log(g(7));
`
	if got, want := runProgramGo(t, src), "2\n1\n"; got != want {
		t.Fatalf("arguments.length printed %q, want %q", got, want)
	}
}

// TestArgumentsIndexReadsStore proves arguments[i] lowers to a read of the backing
// store, and the materialization is still present.
func TestArgumentsIndexReadsStore(t *testing.T) {
	const src = "function f(a: number, b: number): unknown { return arguments[0]; }\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(a), value.Number(b))") {
		t.Errorf("the arguments store was not materialized from the parameters:\n%s", source)
	}
	if !strings.Contains(source, ".At(0)") {
		t.Errorf("arguments[0] did not read the store:\n%s", source)
	}
}

// TestArgumentsIndexRuns builds and runs a body that reads arguments by index, at a
// literal and at a variable index, so the store read is proven against the
// JavaScript result.
func TestArgumentsIndexRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function first(a: number, b: number): unknown {
  return arguments[0];
}
function pick(a: number, b: number, c: number): unknown {
  const i = 2;
  return arguments[i];
}
console.log(first(10, 20));
console.log(pick(1, 2, 3));
`
	if got, want := runProgramGo(t, src), "10\n3\n"; got != want {
		t.Fatalf("arguments index printed %q, want %q", got, want)
	}
}

// TestArgumentsWithRestParameterHandsBack proves a body that reads arguments while
// its signature carries a rest parameter hands back: the rest gathers a call-varying
// tail, so the parameter count is not the call arity and the store cannot stand in.
func TestArgumentsWithRestParameterHandsBack(t *testing.T) {
	const src = "function f(a: number, ...rest: number[]): number { return arguments.length; }\n" +
		"f(1, 2, 3);\n"
	renderProgramHandBack(t, src)
}

// TestArgumentsWithOptionalParameterHandsBack proves a body that reads arguments
// while a parameter is omittable hands back: a call may omit the slot, so the count
// depends on the call site the body cannot see.
func TestArgumentsWithOptionalParameterHandsBack(t *testing.T) {
	const src = "function f(a: number, b: number = 5): number { return arguments.length; }\n" +
		"f(1);\n"
	renderProgramHandBack(t, src)
}

// TestArgumentsUsedAsValueHandsBack proves a bare read of arguments that no backed
// shape consumes hands back, since passing the arity object around is a later slice.
func TestArgumentsUsedAsValueHandsBack(t *testing.T) {
	const src = "function f(a: number): unknown { return arguments; }\n" +
		"f(1);\n"
	renderProgramHandBack(t, src)
}
