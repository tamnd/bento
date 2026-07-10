package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestFunctionWithOwnPropertyHandsBack pins that a named function that later
// carries own data properties (foo.x = 1) hands back rather than lowering. Its
// checker type is a callable object, and the callable-object model interns a
// `type Foo struct { Call func(); X float64 }` for that shape. Emitting the
// `func Foo` declaration too puts two Foo declarations in one block, which does
// not compile, so bento used to emit Go that failed to build (Object/keys
// 15.2.3.14-3-2 hit exactly this). Handing back routes the unit to the
// interpreter until a named callable object is a modeled slice.
func TestFunctionWithOwnPropertyHandsBack(t *testing.T) {
	const src = "function foo() {}\nfoo.x = 1;\nconsole.log(String(foo.x));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "callable object") {
		t.Errorf("hand-back reason = %q, want it to mention a callable object", nyl.Reason)
	}
}

// TestPlainFunctionStillLowers pins that a function with no own properties keeps
// lowering to a bare Go func, so the callable-object handback does not swallow an
// ordinary declaration. A plain function type carries no data properties, so it
// is not a callable object and stays on the func path.
func TestPlainFunctionStillLowers(t *testing.T) {
	const src = "function twice(n: number): number { return n * 2; }\nconsole.log(String(twice(3)));\n"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func Twice(") {
		t.Fatalf("plain function did not lower to a bare func:\n%s", out)
	}
}
