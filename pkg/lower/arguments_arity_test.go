package lower

import (
	"strings"
	"testing"
)

// An arguments-reading function all of whose references are direct calls threads the
// real call-site arguments through a hidden trailing parameter, so a loose-arity call
// reads the true argument count and slots rather than a snapshot of the parameters.
// These pin that a too-many call now threads and lowers to the real count, that a
// too-few call whose omitted slot is a typed parameter still hands back (no undefined
// fits the Go slot), that a write to a named parameter alongside a read of arguments
// still hands back, and that an exact-arity read with no parameter write keeps the
// simpler snapshot so the change is confined to the loose cases.

// TestArgumentsExtraArgThreads proves a call passing more arguments than the
// arguments-reading callee declares threads the real arguments through the hidden
// parameter, so arguments.length reads the call count, not the parameter count.
func TestArgumentsExtraArgThreads(t *testing.T) {
	const src = `function ref(): number { return arguments.length; }
console.log(ref(42));
`
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "*value.Array[value.Value]") {
		t.Errorf("the arguments-reading callee did not take the hidden arguments parameter:\n%s", source)
	}
	if !strings.Contains(source, "Ref(value.NewArray[value.Value](value.Number(42)))") {
		t.Errorf("the call site did not pass the real arguments array:\n%s", source)
	}
}

// TestArgumentsTooFewArgsHandsBack proves a call passing fewer arguments than the
// callee declares still hands back when an omitted slot is a typed parameter: the
// hidden array would carry the shorter argument list faithfully, but the Go parameter
// has no undefined to bind, so the call cannot lower and the whole unit hands back.
func TestArgumentsTooFewArgsHandsBack(t *testing.T) {
	const src = `function testcase(a: number, b: number, c: number): number { return arguments.length; }
console.log(testcase());
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "omits an argument the callee does not default") {
		t.Errorf("hand-back reason %q does not name the unfilled-parameter case", reason)
	}
}

// TestArgumentsParamWriteHandsBack proves a body that reads arguments and also
// assigns to a named parameter hands back: the entry snapshot boxes each
// parameter's original value once, so it cannot mirror the mapped rule where the
// later store to the parameter also changes arguments[i].
func TestArgumentsParamWriteHandsBack(t *testing.T) {
	const src = `function foo(a: number, b: number, c: number): unknown {
  a = 1;
  return arguments[0];
}
foo(10, 20, 30);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "write to a named parameter") {
		t.Errorf("hand-back reason %q does not name the parameter-write case", reason)
	}
}

// TestArgumentsParamUpdateHandsBack proves the same for a ++ on a named parameter:
// an update is a store, so it makes the mapped aliasing observable the snapshot
// cannot reflect.
func TestArgumentsParamUpdateHandsBack(t *testing.T) {
	const src = `function foo(a: number): unknown {
  a++;
  return arguments[0];
}
foo(10);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "write to a named parameter") {
		t.Errorf("hand-back reason %q does not name the parameter-write case", reason)
	}
}

// TestArgumentsExactArityStillLowers is the control: an arguments-reading function
// called with exactly one argument per parameter and no parameter write still
// lowers to the materialized snapshot, so the arity and parameter-write guards are
// not over-broad.
func TestArgumentsExactArityStillLowers(t *testing.T) {
	const src = `function f(a: number, b: number): number { return arguments.length; }
console.log(f(1, 2));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(a), value.Number(b))") {
		t.Errorf("an exact-arity arguments read did not materialize the snapshot:\n%s", source)
	}
}

// TestArgumentsLengthOnlyParamWriteLowers proves a body whose only arguments read is
// arguments.length lowers even alongside a write to a named parameter: length is
// fixed by the call arity and does not track the parameter in either the mapped or
// the unmapped object, so the entry snapshot stays faithful and the mapped-store
// handback does not fire. It is the shape node:assert's fail() reaches.
func TestArgumentsLengthOnlyParamWriteLowers(t *testing.T) {
	const src = `function fail(actual: any, expected: any, message: any): void {
  if (arguments.length === 1) { message = actual; actual = undefined; }
  console.log(String(message));
}
fail("a", "b", "c");
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[value.Value](actual, expected, message)") {
		t.Errorf("a length-only arguments read with a parameter write did not materialize the snapshot:\n%s", source)
	}
}

// TestArgumentsLengthOnlyParamWriteRuns builds the length-guarded parameter-rewrite
// shape and reads the length and the rewritten parameter. Called with the full arity
// the length test is false, so the parameter keeps its passed value, the snapshot
// length is the parameter count, and the write is invisible to the length read.
func TestArgumentsLengthOnlyParamWriteRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function pick(a: any, b: any): void {
  if (arguments.length === 1) { b = a; a = undefined; }
  console.log(String(arguments.length) + ":" + String(a) + ":" + String(b));
}
pick("x", "y");
`
	got := runProgramGo(t, src)
	want := "2:x:y\n"
	if got != want {
		t.Fatalf("length-only arguments run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestArgumentsElementParamWriteStillHandsBack is the control: the same parameter
// write alongside an element read of arguments still hands back, since arguments[i]
// does track the parameter in the mapped object and the frozen snapshot cannot
// mirror the later store. Only the length-only read is relaxed.
func TestArgumentsElementParamWriteStillHandsBack(t *testing.T) {
	const src = `function foo(a: any, b: any): unknown {
  if (arguments.length === 1) { a = b; }
  return arguments[0];
}
foo(10, 20);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "element read alongside a write to a named parameter") {
		t.Errorf("hand-back reason %q does not name the element-read parameter-write case", reason)
	}
}
