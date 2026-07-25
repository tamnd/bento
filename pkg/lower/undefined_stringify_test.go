package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestUndefinedStringifyLowers pins that coercing an undefined-typed value to a
// string routes through value.ToString over the lowered operand, so the operand
// stays referenced rather than being folded away to a bare constant.
func TestUndefinedStringifyLowers(t *testing.T) {
	const src = "const x: undefined = undefined;\nconsole.log(x);\nconsole.log(String(undefined));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ToString(x)") {
		t.Fatalf("console.log of an undefined binding did not route through value.ToString(x):\n%s", source)
	}
}

// TestUndefinedStringifyRuns builds and runs the common undefined-to-string forms:
// console.log(undefined), String(undefined), a template substitution, an undefined
// binding, and undefined mixed with other console arguments. Each prints
// "undefined", matching Node.
func TestUndefinedStringifyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "console.log(undefined);\n" +
		"console.log(String(undefined));\n" +
		"const x = undefined;\n" +
		"console.log(`val=${x}`);\n" +
		"console.log(x);\n" +
		"console.log(undefined, \"a\", 1);\n"
	got := runProgramGo(t, src)
	want := "undefined\nundefined\nval=undefined\nundefined\nundefined a 1\n"
	if got != want {
		t.Fatalf("undefined stringify program printed %q, want %q", got, want)
	}
}

// TestUndefinedReturningCallStringifyHandsBack pins the boundary: an
// undefined-returning function call lowers to a Go statement-call with no value, so
// stringifying its result hands back rather than emit code value.ToString cannot
// take.
func TestUndefinedReturningCallStringifyHandsBack(t *testing.T) {
	const src = "function g(): undefined { return undefined; }\nconsole.log(String(g()));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("stringify of an undefined-returning call err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "coercing this type to a string") {
		t.Fatalf("hand-back reason = %q, want it to mention coercing to a string", nyl.Reason)
	}
}
