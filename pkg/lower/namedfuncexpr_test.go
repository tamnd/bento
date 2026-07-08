package lower

import (
	"strings"
	"testing"
)

// A named function expression carries a name that is in scope only inside its own
// body, where a recursive call reads it. Go has no self-referential function
// literal, so the closure binds to a declared local first and the literal is
// assigned second, which lets the body call the local by name. A name the body
// never reads needs no such two-step and lowers as a plain closure.

// TestNamedFunctionExprSelfReferenceLowers proves a recursive named function
// expression lowers to the two-step (var then assign) so the body's recursive call
// resolves to the bound local rather than to a top-level function name.
func TestNamedFunctionExprSelfReferenceLowers(t *testing.T) {
	const src = "const fac = function f(n: number): number { return n <= 1 ? 1 : n * f(n - 1); };\nconsole.log(fac(5));\n"
	source := renderProgram(t, src)
	for _, want := range []string{"var f func(", "f = func(", "n * f(n-1)", "return f\n"} {
		if !strings.Contains(source, want) {
			t.Errorf("named function expression did not print %q:\n%s", want, source)
		}
	}
}

// TestNamedFunctionExprNoSelfReferenceLowersPlainly proves a named function
// expression whose body never reads its name skips the two-step and lowers as a
// plain closure, so the emitted Go stays readable.
func TestNamedFunctionExprNoSelfReferenceLowersPlainly(t *testing.T) {
	const src = "const g = function h(n: number): number { return n + 1; };\nconsole.log(g(4));\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "var h func(") {
		t.Errorf("a non-recursive named function expression should not take the two-step:\n%s", source)
	}
	if !strings.Contains(source, "g := func(n float64) float64") {
		t.Errorf("a non-recursive named function expression did not lower as a plain closure:\n%s", source)
	}
}

// TestNamedFunctionExprRuns builds and runs a recursive factorial and a two-call
// Fibonacci so the self-reference is proven to resolve against the JavaScript result
// rather than just the emitted shape.
func TestNamedFunctionExprRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const fac = function f(n: number): number {
  return n <= 1 ? 1 : n * f(n - 1);
};
const fib = function f(n: number): number {
  return n < 2 ? n : f(n - 1) + f(n - 2);
};
console.log(fac(5));
console.log(fib(10));
`
	if got, want := runProgramGo(t, src), "120\n55\n"; got != want {
		t.Fatalf("named function expression program printed %q, want %q", got, want)
	}
}
