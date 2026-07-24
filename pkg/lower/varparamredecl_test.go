package lower

import (
	"strings"
	"testing"
)

// TestVarRedeclaresParamNoSecondDecl pins that a `var x` inside a function whose
// parameter is also named x does not emit a second Go declaration: JavaScript hoists
// the var to the one function-scoped slot the parameter already occupies, so the
// bare `var x` keeps the argument value and re-declares nothing. Go binds the
// parameter as a function argument, so a second `var x value.Value` would not compile
// (x redeclared in this block). The emit must carry no such redeclaration. This is
// the function-code/S10.2.1_A5 shape.
func TestVarRedeclaresParamNoSecondDecl(t *testing.T) {
	const src = `function f1(x?: any){ var x; return typeof x; }
console.log(f1());`
	out := renderProgram(t, src)
	if strings.Contains(out, "var x value.Value") {
		t.Fatalf("var redeclaring a parameter emitted a duplicate Go declaration:\n%s", out)
	}
}

// TestVarRedeclaresParamRunsUndefined runs the no-argument call: the parameter is
// undefined and the bare `var x` does not reset it, so typeof x is "undefined",
// matching Node.
func TestVarRedeclaresParamRunsUndefined(t *testing.T) {
	skipIfShort(t)
	const src = `function f1(x?: any){ var x; return typeof x; }
console.log(f1());`
	if got, want := runProgramGo(t, src), "undefined\n"; got != want {
		t.Fatalf("var-redeclares-param typeof printed %q, want %q", got, want)
	}
}

// TestVarRedeclaresParamKeepsArgument runs a call that passes an argument: the bare
// `var x` keeps the argument value rather than resetting x to undefined, so the
// function returns the argument, matching Node's `f2(1) === 1`.
func TestVarRedeclaresParamKeepsArgument(t *testing.T) {
	skipIfShort(t)
	const src = `function f2(x: any){ var x; return x; }
console.log(String(f2(1)));`
	if got, want := runProgramGo(t, src), "1\n"; got != want {
		t.Fatalf("var-redeclares-param kept the wrong value, printed %q, want %q", got, want)
	}
}
