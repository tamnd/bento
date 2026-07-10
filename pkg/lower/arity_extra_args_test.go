package lower

import (
	"strings"
	"testing"
)

// renderTolerant compiles src through the tolerant front door and assembles it to
// Go source, the path a call with an arity the checker rejects takes: the strict
// compile helper would fail on the 2554/2555 diagnostic before the renderer ran.
func renderTolerant(t *testing.T, src string) string {
	t.Helper()
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	r.SetGoConstants(testGoConstants())
	r.SetGoErrorVars(testGoErrorVars())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("RenderProgram handed back: %v", err)
	}
	return p.Source
}

// TestExtraLiteralArgDropped pins that a call passing more arguments than the
// callee accepts drops the extra literal: JavaScript evaluates it and ignores it,
// and the extra has no side effect, so the emitted Go calls with the fixed
// argument only.
func TestExtraLiteralArgDropped(t *testing.T) {
	const src = `function g(a: number): number { return a; }
console.log(g(7, 99, "x"));
`
	source := renderTolerant(t, src)
	if strings.Contains(source, "99") || strings.Contains(source, `"x"`) {
		t.Errorf("extra literal arguments were not dropped:\n%s", source)
	}
	if !strings.Contains(source, "G(7)") {
		t.Errorf("call did not lower to the fixed argument only:\n%s", source)
	}
}

// TestExtraIdentifierArgHandsBack pins that a plain variable read passed as an
// extra argument is not dropped: removing it would leave the local unread, which
// Go rejects as a declared-and-unused binding, so the call hands back rather than
// emit Go that fails to compile.
func TestExtraIdentifierArgHandsBack(t *testing.T) {
	const src = `function g(a: number): number { return a; }
let extra = 5;
console.log(g(1, extra));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "side-effecting extra argument") {
		t.Errorf("hand-back reason %q does not name the extra argument case", reason)
	}
}

// TestSideEffectingExtraArgHandsBack pins the zero-fail guard: an extra argument
// that could mutate or throw is not dropped, because JavaScript still evaluates
// it before ignoring it. The call hands back to a later slice rather than emit Go
// that silently skips the effect.
func TestSideEffectingExtraArgHandsBack(t *testing.T) {
	const src = `function g(a: number): number { return a; }
let log = 0;
function bump(): number { log = log + 1; return log; }
console.log(g(1, bump()));
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "side-effecting extra argument") {
		t.Errorf("hand-back reason %q does not name the side-effecting extra argument", reason)
	}
}
