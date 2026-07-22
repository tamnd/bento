package lower

import (
	"strings"
	"testing"
)

// TestVoidFunctionIntoAnyReturnAdapts pins that a void function assigned into an any
// returning slot is wrapped in an adapter that drives the call and returns undefined.
// Go's func types are invariant, so func() is not assignable to func() value.Value even
// though the JavaScript call yields undefined, so the adapter bridges the return.
func TestVoidFunctionIntoAnyReturnAdapts(t *testing.T) {
	const src = `var fa = function(): any { return 3; };
fa = function() { };
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("void function into an any slot did not adapt to return undefined:\n%s", out)
	}
}

// TestValueFunctionIntoVoidDiscards pins the other direction: a value returning
// function assigned into a void slot is wrapped in an adapter that calls it and
// discards the result, so func() float64 fills a func() slot.
func TestValueFunctionIntoVoidDiscards(t *testing.T) {
	const src = `var fv = function(): void {};
fv = function() { return 0; };
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "fv = func() {") {
		t.Fatalf("value function into a void slot did not adapt to a void signature:\n%s", out)
	}
}

// TestFuncValueCoercionRuns builds and runs both directions through call arguments: a
// void callback passed where an any returning callback is wanted, and a value returning
// callback passed where a void callback is wanted. Each adapter drives the call, so the
// program runs and prints what the callbacks observed.
func TestFuncValueCoercionRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function execAny(cb: (v: any) => any): any { return cb(1); }
function execVoid(cb: (v: any) => void) { cb(2); }
let seen = 0;
const r = execAny(function (v: any) { seen = v; });
console.log(seen, r === undefined);
execVoid(function (v: any) { seen = v; return 9; });
console.log(seen);
`
	if got, want := runProgramGo(t, src), "1 true\n2\n"; got != want {
		t.Fatalf("func value coercion run mismatch:\n got %q\nwant %q", got, want)
	}
}
