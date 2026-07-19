package lower

import (
	"strings"
	"testing"
)

// A method whose body can fall off its end with a dynamic return type runs the
// implicit return undefined a JavaScript function does. The top-level function path
// already appended it, but the method path did not, so an empty-bodied any-returning
// method emitted a Go method with no return and did not compile. The emit now closes
// with the same value.Undefined return.
func TestMethodImplicitUndefinedReturnLowers(t *testing.T) {
	src := "class C {\n  m(): any {}\n}\nconsole.log(typeof new C().m());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "return value.Undefined") {
		t.Fatalf("want a trailing value.Undefined return in the method, got:\n%s", got)
	}
}

func TestMethodImplicitUndefinedReturnRuns(t *testing.T) {
	skipIfShort(t)
	src := "class C {\n  m(): any {}\n}\nconsole.log(typeof new C().m());\n"
	if got := runProgramGo(t, src); got != "undefined\n" {
		t.Fatalf("got %q, want %q", got, "undefined\n")
	}
}

// A value-returning function whose only statement is a throw never returns, but the
// throw lowers to a value.Throw call Go reads as an ordinary one, so the function
// compiled to a missing return. The emit now plants the panic marker bento already
// uses under an exhaustive switch, on the genuinely unreachable ground past the throw.
func TestThrowOnlyFunctionLowers(t *testing.T) {
	src := "function f(): number {\n  throw new Error(\"nyi\");\n}\ntry {\n  f();\n} catch (e) {\n  console.log(\"caught\");\n}\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "panic(\"unreachable\")") {
		t.Fatalf("want an unreachable panic past the throw, got:\n%s", got)
	}
}

func TestThrowOnlyFunctionRuns(t *testing.T) {
	skipIfShort(t)
	src := "function f(): number {\n  throw new Error(\"nyi\");\n}\ntry {\n  f();\n} catch (e) {\n  console.log(\"caught\");\n}\n"
	if got := runProgramGo(t, src); got != "caught\n" {
		t.Fatalf("got %q, want %q", got, "caught\n")
	}
}

// A throw with dead code after it, a hoisted var TypeScript keeps though it never
// runs, still terminates the function: the unconditional throw ends control flow, so
// looking only at the last statement missed it and the function fell through to a
// missing return. The body-terminates scan now sees the throw and the unreachable
// marker lands after the dead tail.
func TestThrowThenDeadCodeFunctionRuns(t *testing.T) {
	skipIfShort(t)
	src := "function g(): number {\n  throw new Error(\"x\");\n  var t;\n}\ntry {\n  g();\n} catch (e) {\n  console.log(\"ok\");\n}\n"
	if got := runProgramGo(t, src); got != "ok\n" {
		t.Fatalf("got %q, want %q", got, "ok\n")
	}
}
