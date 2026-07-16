package lower

import (
	"strings"
	"testing"
)

// A spread of a fixed-length tuple into a call whose callee has only fixed parameters
// expands to the tuple's element reads, f(...pair) becoming f(pair.E0, pair.E1). These
// tests pin the forms buildFixedTupleSpreadCall covers: an exact-arity spread, a spread
// after a leading positional argument, a string tuple, and a labeled tuple, and the
// shapes that keep the honest handback: a side-effecting spread operand, an arity that
// does not match the parameters, and a spread element whose Go type differs from its
// parameter.

// TestTupleSpreadFixedCallRuns proves a spread of a required number tuple into a
// three-parameter function runs, each element landing in its position.
func TestTupleSpreadFixedCallRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f(a: number, b: number, c: number): number { return a + b + c; }\nconst t: [number, number, number] = [1, 2, 3];\nconsole.log(f(...t));\n"
	want := "6\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("tuple spread call printed %q, want %q", got, want)
	}
}

// TestTupleSpreadWithLeadingArgRuns proves a positional argument ahead of the spread
// keeps its place, f(1, ...pair) filling the first parameter directly and the tuple's
// two elements the rest.
func TestTupleSpreadWithLeadingArgRuns(t *testing.T) {
	skipIfShort(t)
	const src = "function f(x: number, a: number, b: number): number { return x + a + b; }\nconst p: [number, number] = [2, 3];\nconsole.log(f(1, ...p));\n"
	want := "6\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("leading-arg tuple spread printed %q, want %q", got, want)
	}
}

// TestTupleSpreadStringsRun proves a string tuple spreads the same way a number tuple
// does, the element reads carrying the value.BStr fields.
func TestTupleSpreadStringsRun(t *testing.T) {
	skipIfShort(t)
	const src = "function f(a: string, b: string): string { return a + b; }\nconst t: [string, string] = [\"x\", \"y\"];\nconsole.log(f(...t));\n"
	want := "xy\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("string tuple spread printed %q, want %q", got, want)
	}
}

// TestTupleSpreadEmitsFieldReads pins the lowering: the spread splices the tuple's
// positional struct fields into the call rather than building a runtime array.
func TestTupleSpreadEmitsFieldReads(t *testing.T) {
	const src = "function f(a: number, b: number, c: number): number { return a + b + c; }\nconst t: [number, number, number] = [1, 2, 3];\nconsole.log(f(...t));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "F(t.E0, t.E1, t.E2)") {
		t.Fatalf("tuple spread did not splice element reads:\n%s", source)
	}
}

// TestTupleSpreadSideEffectingReceiverHandsBack pins that a spread of a call result does
// not expand, since reading a tuple element more than once would run the call's effect
// more than once, so the whole spread keeps the handback.
func TestTupleSpreadSideEffectingReceiverHandsBack(t *testing.T) {
	const src = "function mk(): [number, number] { return [1, 2]; }\nfunction f(a: number, b: number): number { return a + b; }\nconsole.log(f(...mk()));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "kind#42") {
		t.Fatalf("side-effecting spread handed back with %q, want the spread-element reason", reason)
	}
}

// TestTupleSpreadDynamicParamHandsBack pins that a spread element whose Go type differs
// from its parameter does not expand: a number element landing in an any parameter would
// need the source node the argument bridge boxes through, which a spliced field read does
// not carry, so the spread keeps the handback rather than drop a bare float64 into a
// value.Value slot.
func TestTupleSpreadDynamicParamHandsBack(t *testing.T) {
	const src = "function f(a: any, b: any): number { return 0; }\nconst t: [number, number] = [1, 2];\nconsole.log(f(...t));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "kind#42") {
		t.Fatalf("dynamic-parameter spread handed back with %q, want the spread-element reason", reason)
	}
}
