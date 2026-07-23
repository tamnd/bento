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

// TestArgumentsForOfRangesStore proves a for...of over arguments ranges the backing
// store's elements.
func TestArgumentsForOfRangesStore(t *testing.T) {
	const src = "function f(a: number, b: number): number {\n" +
		"  let n = 0;\n" +
		"  for (const x of arguments) { n++; }\n" +
		"  return n;\n" +
		"}\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Elems()") {
		t.Errorf("for...of over arguments did not range the store:\n%s", source)
	}
}

// TestArgumentsForOfRuns builds and runs a counting for...of and an element-printing
// for...of over arguments so the ranged store is proven against the JavaScript
// result.
func TestArgumentsForOfRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function count(a: number, b: number, c: number): number {
  let n = 0;
  for (const x of arguments) {
    n++;
  }
  return n;
}
function each(a: number, b: number): void {
  for (const x of arguments) {
    console.log(x);
  }
}
console.log(count(1, 2, 3));
each(7, 8);
`
	if got, want := runProgramGo(t, src), "3\n7\n8\n"; got != want {
		t.Fatalf("for...of over arguments printed %q, want %q", got, want)
	}
}

// TestArgumentsInNestedArrowCapturesStore proves a nested arrow that reads
// arguments makes the enclosing function materialize the store and closes over it,
// since an arrow has no arguments of its own.
func TestArgumentsInNestedArrowCapturesStore(t *testing.T) {
	const src = "function f(a: number, b: number): number {\n" +
		"  const get = () => arguments.length;\n" +
		"  return get();\n" +
		"}\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(a), value.Number(b))") {
		t.Errorf("an arrow's arguments read did not materialize the enclosing store:\n%s", source)
	}
	if !strings.Contains(source, ".Len()") {
		t.Errorf("the arrow did not read the captured store:\n%s", source)
	}
}

// TestArgumentsInNestedArrowRuns builds and runs a function whose arrow reads the
// enclosing arguments, both length and index, so the captured store is proven
// against the JavaScript result.
func TestArgumentsInNestedArrowRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function count(a: number, b: number, c: number): number {
  const get = () => arguments.length;
  return get();
}
function first(a: number, b: number): unknown {
  const get = () => arguments[0];
  return get();
}
console.log(count(1, 2, 3));
console.log(first(9, 8));
`
	if got, want := runProgramGo(t, src), "3\n9\n"; got != want {
		t.Fatalf("arguments in a nested arrow printed %q, want %q", got, want)
	}
}

// TestArgumentsInFunctionExpressionMaterializesStore proves a function expression
// that reads arguments materializes its own store from its parameters inside the
// closure, since a function expression carries an arguments object of its own.
func TestArgumentsInFunctionExpressionMaterializesStore(t *testing.T) {
	const src = "const f = function (a: number, b: number): number { return arguments.length; };\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(a), value.Number(b))") {
		t.Errorf("the function expression did not materialize its arguments store:\n%s", source)
	}
	if !strings.Contains(source, ".Len()") {
		t.Errorf("arguments.length in the function expression did not read the store:\n%s", source)
	}
}

// TestArgumentsInFunctionExpressionRuns builds and runs a function expression that
// reads arguments by length and by index, so its own materialized store is proven
// against the JavaScript result.
func TestArgumentsInFunctionExpressionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const len = function (a: number, b: number): number {
  return arguments.length;
};
const first = function (a: number, b: number): unknown {
  return arguments[0];
};
console.log(len(1, 2));
console.log(first(9, 8));
`
	if got, want := runProgramGo(t, src), "2\n9\n"; got != want {
		t.Fatalf("arguments in a function expression printed %q, want %q", got, want)
	}
}

// TestArgumentsWriteReadsStore proves a write to arguments[i] lowers to the backing
// store's Set when no parameter is read by name, so the write and a following read
// go through the same snapshot.
func TestArgumentsWriteReadsStore(t *testing.T) {
	const src = "function f(a: number, b: number): unknown {\n" +
		"  arguments[0] = 9;\n" +
		"  return arguments[0];\n" +
		"}\n" +
		"f(1, 2);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Set(0, value.Number(9))") {
		t.Errorf("the write to arguments did not store into the backing array:\n%s", source)
	}
}

// TestArgumentsWriteRuns builds and runs a body that writes arguments[i] then reads
// it back, so the store write is proven against the JavaScript result.
func TestArgumentsWriteRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function set(a: number, b: number): unknown {
  arguments[0] = 42;
  return arguments[0];
}
console.log(set(1, 2));
`
	if got, want := runProgramGo(t, src), "42\n"; got != want {
		t.Fatalf("a write to arguments printed %q, want %q", got, want)
	}
}

// TestArgumentsWriteWithNamedParameterHandsBack proves a write to arguments[i] hands
// back when the body also reads a parameter by name: the snapshot store cannot
// mirror the mapped rule where the write would change that parameter too, so the
// whole function hands back rather than emit an unfaithful write.
func TestArgumentsWriteWithNamedParameterHandsBack(t *testing.T) {
	const src = "function f(a: number, b: number): number {\n" +
		"  arguments[0] = 9;\n" +
		"  return a;\n" +
		"}\n" +
		"f(1, 2);\n"
	renderProgramHandBack(t, src)
}

// TestArgumentsWithRestParameterThreads proves a body that reads arguments while its
// signature carries a rest parameter threads the real call-site arguments: the rest
// gathers its own tail as before, and the hidden array carries every argument, so
// arguments.length reads the true count regardless of the rest split.
func TestArgumentsWithRestParameterThreads(t *testing.T) {
	const src = "function f(a: number, ...rest: number[]): number { return arguments.length; }\n" +
		"f(1, 2, 3);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "rest *value.Array[float64], _bt0 *value.Array[value.Value]") {
		t.Errorf("the rest-parameter callee did not take the hidden arguments parameter after the rest:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(1), value.Number(2), value.Number(3))") {
		t.Errorf("the call site did not pass every real argument in the hidden array:\n%s", source)
	}
}

// TestArgumentsWithRestParameterRuns builds and runs an arguments-reading rest
// function so the real count and the rest tail are both proven against the
// JavaScript result.
func TestArgumentsWithRestParameterRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, ...rest: number[]): number {
  return arguments.length + rest.length;
}
console.log(f(1, 2, 3, 4));
`
	if got, want := runProgramGo(t, src), "7\n"; got != want {
		t.Fatalf("arguments in a rest function printed %q, want %q", got, want)
	}
}

// TestArgumentsWithOptionalParameterThreads proves a body that reads arguments while
// a parameter is omittable threads the real arguments: the omitted parameter still
// fills its default at the call site, while the hidden array holds only the arguments
// actually passed, so arguments.length is the call count, not the parameter count.
func TestArgumentsWithOptionalParameterThreads(t *testing.T) {
	const src = "function f(a: number, b: number = 5): number { return arguments.length; }\n" +
		"f(1);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "*value.Array[value.Value]") {
		t.Errorf("the optional-parameter callee did not take the hidden arguments parameter:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(1))") {
		t.Errorf("the call site did not pass only the argument actually supplied:\n%s", source)
	}
}

// TestArgumentsWithOptionalParameterRuns builds and runs an arguments-reading
// function with a defaulted parameter at two arities so the real count is proven
// against the JavaScript result.
func TestArgumentsWithOptionalParameterRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number, b: number = 5): number {
  return arguments.length;
}
console.log(f(1));
console.log(f(7, 8));
`
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("arguments with an optional parameter printed %q, want %q", got, want)
	}
}

// TestArgumentsUsedAsValueHandsBack proves a bare read of arguments that no backed
// shape consumes hands back, since passing the arity object around is a later slice.
func TestArgumentsUsedAsValueHandsBack(t *testing.T) {
	const src = "function f(a: number): unknown { return arguments; }\n" +
		"f(1);\n"
	renderProgramHandBack(t, src)
}
