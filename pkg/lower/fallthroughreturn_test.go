package lower

import (
	"strings"
	"testing"
)

// TestFallThroughReturnsUndefined pins that a function whose declared return type
// is any and whose body can run off its end gets the trailing return
// value.Undefined. Without it the emitted Go had a value-returning function with
// no final return and did not compile. formatIdentityFreeValue in the test262
// prelude takes this shape with a switch over the value kind and no default arm.
func TestFallThroughReturnsUndefined(t *testing.T) {
	src := `function classify(x: string): any { switch (x) { case "a": return 1; } }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("fall-through any return did not emit the trailing undefined return:\n%s", out)
	}
}

// TestFallThroughReturnInFunctionExpr pins the same trailing return for a function
// expression, the other body form that can fall through, since compareArray and
// its siblings in the prelude are function expressions.
func TestFallThroughReturnInFunctionExpr(t *testing.T) {
	src := `const classify = function (x: string): any { switch (x) { case "a": return 1; } };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("fall-through any return in a function expression did not emit the trailing undefined return:\n%s", out)
	}
}

// TestTerminatingBodyKeepsNoTrailingReturn pins that a body that already returns on
// every path takes no extra trailing return, so a returning function is left as the
// developer wrote it.
func TestTerminatingBodyKeepsNoTrailingReturn(t *testing.T) {
	src := `function pick(x: string): any { if (x === "a") { return 1; } return 2; }`
	out := renderProgram(t, src)
	if strings.Contains(out, "return value.Undefined") {
		t.Fatalf("terminating body gained a spurious trailing undefined return:\n%s", out)
	}
}

// TestFallThroughRuns builds and runs the fall-through and checks the missing arm
// yields undefined the way JavaScript does.
func TestFallThroughRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function classify(x: string): any {
  switch (x) {
    case "num":
      return 1;
  }
}
console.log(String(classify("num")));
console.log(String(classify("other")));
`
	got := runProgramGo(t, src)
	want := "1\nundefined\n"
	if got != want {
		t.Fatalf("fall-through run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestFinallyReturnTerminatesBody pins that a value-returning function whose body
// ends in a try/finally that always returns compiles. The finally lowers to an
// `if done { return ret }` tail Go does not accept as terminating, so without
// recognizing the try as terminating the emit had a value function with no final
// return and did not compile. bodyTerminates now treats a finally that returns as
// terminating, so endThrowTerminatedBody plants the unreachable panic.
func TestFinallyReturnTerminatesBody(t *testing.T) {
	skipIfShort(t)
	src := `
function f(): number {
  let x = 100;
  try { throw "WAT"; }
  catch (e) { }
  finally { return x; }
}
console.log(String(f()));
`
	got := runProgramGo(t, src)
	want := "100\n"
	if got != want {
		t.Fatalf("finally-return body run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestTryCatchBothReturnTerminatesBody pins the no-finally arm: a try whose
// protected block and catch clause both return leaves no path to the code after,
// so a value function ending in it compiles without a spurious trailing return.
func TestTryCatchBothReturnTerminatesBody(t *testing.T) {
	skipIfShort(t)
	src := `
function g(y: number): number {
  try { if (y > 0) { return 1; } throw "neg"; }
  catch (e) { return 2; }
}
console.log(String(g(5)));
console.log(String(g(-5)));
`
	got := runProgramGo(t, src)
	want := "1\n2\n"
	if got != want {
		t.Fatalf("try/catch-both-return body run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestVoidReturnUndefinedDropsOperand pins that a function inferred to return void
// because its body returns undefined lowers `return undefined` to a bare return. A
// void Go function has no result, so emitting `return value.Undefined` would give
// the return one operand too many and the Go would not compile. This is the
// implicitAnyFunctionReturnNullOrUndefined shape.
func TestVoidReturnUndefinedDropsOperand(t *testing.T) {
	src := `function f() { return undefined; }`
	out := renderProgram(t, src)
	if strings.Contains(out, "return value.Undefined") {
		t.Fatalf("void function kept a returned operand instead of a bare return:\n%s", out)
	}
}

// TestVoidReturnCallEvaluatedThenBareReturn pins that a void function returning a
// call keeps the call's effect: the emit runs the call as a statement, then bare
// returns. Dropping the call outright would lose its side effect.
func TestVoidReturnCallEvaluatedThenBareReturn(t *testing.T) {
	skipIfShort(t)
	src := `
let count = 0;
function tick() { count += 1; }
function f() { return tick(); }
f();
console.log(String(count));
`
	got := runProgramGo(t, src)
	want := "1\n"
	if got != want {
		t.Fatalf("void function returning a call lost its effect:\n got %q\nwant %q", got, want)
	}
}

// TestOptionalFallThroughReturnsNone pins that a function whose inferred return type
// is T | undefined and whose body can run off its end gets a trailing
// return value.None[T]. The return lowers to a value.Opt[T] result, and a body that
// falls through in TypeScript yields undefined, which for that slot is None; without
// it the emitted Go had a value-returning function with no final return.
func TestOptionalFallThroughReturnsNone(t *testing.T) {
	src := `function f(b: boolean) { if (b) { return 1; } }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.None[float64]") {
		t.Fatalf("optional fall-through did not emit the trailing None return:\n%s", out)
	}
}

// TestOptionalFallThroughRuns builds and runs the optional fall-through shape and
// checks both arms: the returning path yields the value and the fall-through path
// yields undefined, the None the trailing return supplies.
func TestOptionalFallThroughRuns(t *testing.T) {
	skipIfShort(t)
	src := `
function f(b: boolean): number | undefined { if (b) { return 7; } }
console.log(String(f(true)));
console.log(String(f(false)));
`
	got := runProgramGo(t, src)
	want := "7\nundefined\n"
	if got != want {
		t.Fatalf("optional fall-through run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestTypeofSwitchExhaustiveTerminates pins that a switch over typeof x whose cases
// cover all eight typeof strings and each return counts as exhaustive-terminating, so
// a value function ending in it (with the checker's unreachable tail after) compiles.
// Without recognizing the switch as terminating, Go saw the tail fall through and
// reported a missing return. This is the unreachableSwitchTypeof shape.
func TestTypeofSwitchExhaustiveTerminates(t *testing.T) {
	skipIfShort(t)
	src := `
const f = (x: any): number => {
  switch (typeof x) {
    case 'string': return 1;
    case 'number': return 2;
    case 'bigint': return 3;
    case 'boolean': return 4;
    case 'symbol': return 5;
    case 'undefined': return 6;
    case 'object': return 7;
    case 'function': return 8;
  }
  x;
};
console.log(String(f("hi")));
console.log(String(f(42)));
`
	got := runProgramGo(t, src)
	want := "1\n2\n"
	if got != want {
		t.Fatalf("exhaustive typeof switch run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestTypeofSwitchMissingCaseNotExhaustive pins the negative: a typeof switch that
// leaves a case out is not exhaustive, so a function ending in it still needs its
// real fall-out and is not marked terminating. The any return type lets the body
// fall through, and the trailing return value.Undefined closes it rather than a panic.
func TestTypeofSwitchMissingCaseNotExhaustive(t *testing.T) {
	src := `
const f = (x: any): any => {
  switch (typeof x) {
    case 'string': return 1;
    case 'number': return 2;
  }
};`
	out := renderProgram(t, src)
	if !strings.Contains(out, "return value.Undefined") {
		t.Fatalf("non-exhaustive typeof switch should fall through to an undefined return:\n%s", out)
	}
	if strings.Contains(out, `panic("unreachable")`) {
		t.Fatalf("non-exhaustive typeof switch wrongly planted an unreachable panic:\n%s", out)
	}
}
