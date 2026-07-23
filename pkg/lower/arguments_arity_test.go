package lower

import (
	"strings"
	"testing"
)

// The parameter snapshot an arguments-reading function materializes stands in for
// the passed arguments only when the call passes exactly one argument per
// parameter. The tolerant call lowering drops an extra argument and fills a missing
// one, so a call whose fixed argument count differs from the parameter count would
// read the wrong length and the wrong slots off the snapshot. These pin that both
// arity directions hand back, that a write to a named parameter alongside a read of
// arguments hands back, and that an exact-arity read with no parameter write still
// lowers so the guards are not over-broad.

// TestArgumentsExtraArgHandsBack proves a call passing more arguments than the
// arguments-reading callee declares hands back: the extras are dropped before the
// snapshot, so arguments.length would read the parameter count, not the call count.
func TestArgumentsExtraArgHandsBack(t *testing.T) {
	const src = `function ref(): number { return arguments.length; }
console.log(ref(42));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "arity the parameters do not fix") {
		t.Errorf("hand-back reason %q does not name the arguments arity case", reason)
	}
}

// TestArgumentsTooFewArgsHandsBack proves a call passing fewer arguments than the
// arguments-reading callee declares hands back: the missing slots are filled before
// the snapshot, so arguments.length would read the parameter count, not the shorter
// call count.
func TestArgumentsTooFewArgsHandsBack(t *testing.T) {
	const src = `function testcase(a: number, b: number, c: number): number { return arguments.length; }
console.log(testcase());
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "arity the parameters do not fix") {
		t.Errorf("hand-back reason %q does not name the arguments arity case", reason)
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
