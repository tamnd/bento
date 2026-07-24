package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestVoidLiteralLowersToUndefined pins that void 0 lowers to the value.Undefined
// singleton, the same value the undefined literal reads, with the operand dropped.
func TestVoidLiteralLowersToUndefined(t *testing.T) {
	const src = "const x = void 0;\nvoid (1 + 2);\nconsole.log(\"done\");\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x := value.Undefined") {
		t.Fatalf("void 0 did not lower to the undefined singleton:\n%s", source)
	}
	if !strings.Contains(source, "_ = value.Undefined") {
		t.Fatalf("void as a discarded statement did not lower to the undefined singleton:\n%s", source)
	}
	// The side-effect-free operand is folded away, so neither operand appears.
	if strings.Contains(source, "1 + 2") {
		t.Fatalf("void dropped neither operand; the arithmetic operand survived:\n%s", source)
	}
}

// TestVoidSideEffectHandsBack pins that void over an operand that could run a side
// effect hands back rather than fold to undefined and silently drop that effect.
func TestVoidSideEffectHandsBack(t *testing.T) {
	const src = "function f(): number { return 1; }\nconst x = void f();\nconsole.log(x === undefined);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("void over a call err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "void over an operand with a side effect") {
		t.Fatalf("void over a call handed back with %q, want the side-effect reason", nyl.Reason)
	}
}

// TestVoidStatementRuns builds and runs void as a discarded expression statement,
// the canonical use: the statements evaluate to nothing and the program continues.
func TestVoidStatementRuns(t *testing.T) {
	skipIfShort(t)
	const src = "void 0;\nvoid (1 + 2);\nvoid \"side\";\nconsole.log(\"done\");\n"
	got := runProgramGo(t, src)
	want := "done\n"
	if got != want {
		t.Fatalf("void statement program printed %q, want %q", got, want)
	}
}
