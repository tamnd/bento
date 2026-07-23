package lower

import (
	"strings"
	"testing"
)

// A function all of whose references are direct calls threads the real call-site
// arguments through a hidden trailing parameter, so arguments sees the arguments
// actually passed regardless of the declared parameter count. These cover the reads
// across the arity boundary (index past the parameters, a count under loose arity, a
// for...of over the real list, a nested arrow capturing the enclosing arguments) and
// the controls that must stay hand-backs: a reference that is not a direct call (a
// value use, a boxing into a dynamic slot) leaves no call site to pass the array, so
// the function keeps the snapshot model or hands back rather than emit a wrong result.

// TestArgumentsIndexAcrossArityThreads proves arguments[i] reads a slot past the last
// parameter under a too-many call: the hidden array carries every argument, so the
// index beyond the parameters resolves to the value actually passed.
func TestArgumentsIndexAcrossArityThreads(t *testing.T) {
	const src = `function f(a: number): unknown { return arguments[2]; }
console.log(f(10, 20, 30));
`
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "_bt0.At(2)") {
		t.Errorf("the index read did not resolve to the hidden arguments store:\n%s", source)
	}
	if !strings.Contains(source, "value.NewArray[value.Value](value.Number(10), value.Number(20), value.Number(30))") {
		t.Errorf("the call site did not pass every real argument:\n%s", source)
	}
}

// TestArgumentsLooseArityRuns builds and runs a spread of loose-arity reads (a count
// with more arguments than parameters, an index past the parameters, a for...of over
// the real list) so the threaded arguments are proven against the JavaScript result.
func TestArgumentsLooseArityRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function count(a: number): number {
  return arguments.length;
}
function beyond(a: number): unknown {
  return arguments[2];
}
function sum(): number {
  let total = 0;
  for (const x of arguments) {
    total += x as number;
  }
  return total;
}
console.log(count(1, 2, 3, 4));
console.log(beyond(10, 20, 30));
console.log(sum(1, 2, 3, 4, 5));
`
	if got, want := runProgramGoTolerant(t, src), "4\n30\n15\n"; got != want {
		t.Fatalf("loose-arity arguments printed %q, want %q", got, want)
	}
}

// TestArgumentsNestedArrowLooseArityRuns proves a nested arrow that reads the
// enclosing arguments sees the real call-site list, since it captures the hidden
// parameter the enclosing function threads rather than a snapshot of the parameters.
func TestArgumentsNestedArrowLooseArityRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(a: number): number {
  const len = () => arguments.length;
  const at = () => arguments[1] as number;
  return len() + at();
}
console.log(f(10, 20, 30));
`
	if got, want := runProgramGoTolerant(t, src), "23\n"; got != want {
		t.Fatalf("arguments in a nested arrow under loose arity printed %q, want %q", got, want)
	}
}

// TestArgumentsValueUseBlocksThreading proves a function whose name is also used as a
// value is not threaded even when a loose call exists: the value use is a reference no
// call-site rewrite would reach, so the function keeps the snapshot model and the
// loose call hands back rather than emit an arguments object decoupled from the call.
func TestArgumentsValueUseBlocksThreading(t *testing.T) {
	const src = `function f(a: number): number { return arguments.length; }
const g = f;
console.log(f(1, 2));
console.log(g(3));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "arity the parameters do not fix") {
		t.Errorf("hand-back reason %q does not name the snapshot arity guard the value use fell back to", reason)
	}
}

// TestArgumentsBoxedCalleeHandsBack proves an arguments-reading function boxed into a
// dynamic value still hands back: the boxed call convention hides the call-site count,
// so there is no call site to thread the real arguments through and emitting a
// snapshot would be wrong.
func TestArgumentsBoxedCalleeHandsBack(t *testing.T) {
	const src = `const a: any = function (x: number): number { return arguments.length; };
a(1, 2, 3);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "boxed into a dynamic value") {
		t.Errorf("hand-back reason %q does not name the boxed-callee case", reason)
	}
}

// TestArgumentsSpreadOverArgumentsHandsBack proves a spread of the arguments object
// itself still hands back: threading backs arguments.length, arguments[i], and a
// for...of, but a bare read of arguments (here spread into an array literal) is not a
// shape this slice consumes, so the whole function hands back rather than emit a
// wrong result.
func TestArgumentsSpreadOverArgumentsHandsBack(t *testing.T) {
	const src = `function f(a: number): number { return [...arguments].length; }
f(1, 2, 3);
`
	renderProgramTolerantHandBack(t, src)
}
