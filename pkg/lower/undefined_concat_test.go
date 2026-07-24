package lower

import (
	"strings"
	"testing"
)

// TestUndefinedConcatLowers pins that concatenating an undefined-typed operand with
// a string routes through value.ToString over the lowered operand, so the operand
// stays referenced and never reaches the boxOperand handback.
func TestUndefinedConcatLowers(t *testing.T) {
	const src = "const x = undefined;\nconsole.log(\"v=\" + x);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ToString(x)") {
		t.Fatalf("string concat with an undefined operand did not route through value.ToString(x):\n%s", source)
	}
}

// TestUndefinedConcatRuns builds and runs the undefined-in-concatenation forms: an
// undefined binding on the right, the undefined literal, an undefined operand on the
// left, and a chained concat. Each renders the undefined operand as "undefined",
// matching Node.
func TestUndefinedConcatRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const x = undefined;\n" +
		"console.log(\"v=\" + x);\n" +
		"console.log(\"v=\" + undefined);\n" +
		"console.log(x + \"=v\");\n" +
		"console.log(\"a\" + x + \"b\");\n"
	got := runProgramGo(t, src)
	want := "v=undefined\nv=undefined\nundefined=v\naundefinedb\n"
	if got != want {
		t.Fatalf("undefined concat program printed %q, want %q", got, want)
	}
}
