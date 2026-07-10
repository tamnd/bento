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
