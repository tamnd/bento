package lower

import (
	"strings"
	"testing"
)

// A call whose callee is not a bare name but a larger expression that evaluates to
// a function value lowers to that expression applied to its arguments: an array
// element fs[0](x), the result of another call mk(5)(), a parenthesized arrow. The
// callee lowers by its own rules and the argument list is bridged the same way a
// named call's is, so these share the function-value path rather than hand the unit
// back to the interpreter.

// TestElementCalleeLowers proves calling an array element (fs[0](3)) lowers to the
// element read applied to the argument.
func TestElementCalleeLowers(t *testing.T) {
	const src = "export function f(): number { const fs: ((n: number) => number)[] = [(n: number) => n + 1]; return fs[0](3); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "fs.At(0)(3)") {
		t.Errorf("element callee did not lower to an applied element read:\n%s", source)
	}
}

// TestCallResultCalleeLowers proves calling the result of another call (mk(5)())
// lowers to the inner call applied with no arguments.
func TestCallResultCalleeLowers(t *testing.T) {
	const src = "export function mk(n: number): () => number { return () => n; }\nexport function f(): number { return mk(5)(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Mk(5)()") {
		t.Errorf("call-result callee did not lower to an applied inner call:\n%s", source)
	}
}

// TestCalleeExpressionsRun builds and runs the generated Go so each non-identifier
// callee form is proven to compute the same result as a named call, not just to
// lower.
func TestCalleeExpressionsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
const fs: ((n: number) => number)[] = [(n: number) => n + 1, (n: number) => n * 2];
console.log(fs[0](3));
console.log(fs[1](3));

function mk(n: number): () => number {
  return () => n + 1;
}
console.log(mk(5)());

function add(a: number): (b: number) => number {
  return (b: number) => a + b;
}
console.log(add(2)(3));

console.log(((n: number) => n * n)(4));
`
	if got, want := runProgramGo(t, src), "4\n6\n6\n5\n16\n"; got != want {
		t.Fatalf("callee expressions printed %q, want %q", got, want)
	}
}
