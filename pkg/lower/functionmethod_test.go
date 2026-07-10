package lower

import (
	"strings"
	"testing"
)

// Function.prototype.call invokes a function with an explicit this and the remaining
// positional arguments. bento's plain functions take no this, since a body that
// reads this hands back when the function is lowered, so f.call(thisArg, a, b) lowers
// to the direct call F(a, b) with the this argument dropped once its evaluation is
// pure. A this argument that could have a side effect keeps the handback rather than
// drop an observable evaluation.

// TestFunctionCallLowersToDirectCall proves f.call(thisArg, a, b) lowers to the
// direct Go call with the this argument dropped, so the call reads exactly as a bare
// call of the same function.
func TestFunctionCallLowersToDirectCall(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"console.log(add.call(null, 2, 3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Add(2, 3)") {
		t.Errorf("call did not lower to the direct call with the this argument dropped:\n%s", source)
	}
	if strings.Contains(source, "Add(nil") || strings.Contains(source, "value.Undefined") {
		t.Errorf("the this argument was not dropped from the call:\n%s", source)
	}
}

// TestFunctionCallRuns builds and runs a function invoked through call so the lowered
// direct call is proven against the JavaScript result, for both a null and an
// undefined this argument and a no-argument call.
func TestFunctionCallRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"function pi(): number { return 3; }\n" +
		"console.log(add.call(null, 2, 3));\n" +
		"console.log(add.call(undefined, 4, 5));\n" +
		"console.log(pi.call(null));\n"
	if got, want := runProgramGo(t, src), "5\n9\n3\n"; got != want {
		t.Fatalf("call printed %q, want %q", got, want)
	}
}

// TestFunctionCallSideEffectingThisHandsBack proves a this argument that could have a
// side effect hands back, since bento drops the this a plain function never reads and
// dropping an observable evaluation would change what the program runs.
func TestFunctionCallSideEffectingThisHandsBack(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"function mk(): number { return 0; }\n" +
		"console.log(add.call(mk(), 2, 3));\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "this argument") {
		t.Fatalf("side-effecting this hand-back reason = %q, want a this-argument reason", reason)
	}
}

// Function.prototype.apply invokes a function the same way call does, but gathers the
// positional arguments in an array rather than spelling them inline. bento reads the
// elements of a plain array literal as the positional arguments and lowers the whole
// invocation to the direct call, so apply(null, [a, b]) is F(a, b).

// TestFunctionApplyLowersToDirectCall proves f.apply(thisArg, [a, b]) lowers to the
// direct Go call with the array literal's elements spread as positional arguments.
func TestFunctionApplyLowersToDirectCall(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"console.log(add.apply(null, [2, 3]));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Add(2, 3)") {
		t.Errorf("apply did not lower to the direct call over the array literal's elements:\n%s", source)
	}
}

// TestFunctionApplyRuns builds and runs a function invoked through apply so the
// lowered direct call is proven against the JavaScript result, including a
// no-argument apply that passes only a this argument.
func TestFunctionApplyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"function pi(): number { return 3; }\n" +
		"console.log(add.apply(null, [2, 3]));\n" +
		"console.log(add.apply(undefined, [4, 5]));\n" +
		"console.log(pi.apply(null));\n"
	if got, want := runProgramGo(t, src), "5\n9\n3\n"; got != want {
		t.Fatalf("apply printed %q, want %q", got, want)
	}
}

// Function.prototype.bind fixes this and any leading arguments and yields a new
// function. The checker types that new function as (...args: [tuple]) => R, a rest
// parameter whose element is the tuple of the remaining parameters, and bento does not
// yet render a tuple-typed rest parameter, so the bound value is unrenderable wherever
// it is used. bind is therefore a clean handback today, blocked on rendering a
// rest-over-tuple function type.

// TestFunctionBindHandsBack proves f.bind(thisArg, arg) on a plain function whose
// result is discarded reaches the bind recognizer and hands back with the reason that
// names the real blocker, the rest-over-tuple type of the bound value.
func TestFunctionBindHandsBack(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"add.bind(null, 2);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "rest-over-tuple") {
		t.Fatalf("bind hand-back reason = %q, want the rest-over-tuple reason", reason)
	}
}

// TestFunctionBoundValueHandsBack proves that binding the result to a name and calling
// it hands back too: rendering the bound value's rest-over-tuple function type is the
// first thing that cannot lower, so the whole unit routes to the engine. This pins
// that a consumed bound value is not silently mislowered while the rest-over-tuple
// function type stays a later slice.
func TestFunctionBoundValueHandsBack(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"const g = add.bind(null, 2);\n" +
		"console.log(g(3));\n"
	renderProgramHandBack(t, src)
}

// A function's .length is its arity: the count of parameters before the first
// defaulted or rest one, a compile-time constant. bento models a function as a bare Go
// func with no struct, so without a reflective path the read would fold to undefined;
// it lowers instead to the numeric constant of the signature's MinArgs.

// TestFunctionLengthLowersToConstant proves add.length lowers to the numeric constant 2
// rather than the missing-property fold that would answer undefined.
func TestFunctionLengthLowersToConstant(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"console.log(add.length);\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "MissingProperty") {
		t.Errorf(".length folded to the missing-property path instead of a constant:\n%s", source)
	}
	if !strings.Contains(source, "NumberToString(2)") {
		t.Errorf(".length did not lower to the constant 2:\n%s", source)
	}
}

// TestFunctionLengthRuns builds and runs .length reads so the lowered constants are
// proven against the JavaScript arity, for a required-only function, a function with a
// defaulted tail, and a function with a rest parameter, none of which count toward the
// arity past the first optional one.
func TestFunctionLengthRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"function greet(name: string, greeting = \"hi\"): string { return greeting + name; }\n" +
		"function sum(first: number, ...rest: number[]): number { return first; }\n" +
		"console.log(add.length);\n" +
		"console.log(greet.length);\n" +
		"console.log(sum.length);\n"
	if got, want := runProgramGo(t, src), "2\n1\n1\n"; got != want {
		t.Fatalf(".length printed %q, want %q", got, want)
	}
}

// TestFunctionLengthOffVariableHandsBack proves .length off a function value held in a
// variable, which has no named declaration to count at compile time, hands back rather
// than answer a wrong constant or fold to undefined.
func TestFunctionLengthOffVariableHandsBack(t *testing.T) {
	const src = "function add(a: number, b: number): number { return a + b; }\n" +
		"const f = add;\n" +
		"console.log(f.length);\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "reflective .length") {
		t.Fatalf(".length off a variable hand-back reason = %q, want a reflective-length reason", reason)
	}
}
