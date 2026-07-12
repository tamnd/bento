package lower

import (
	"strings"
	"testing"
)

// TestStringSpreadEmits pins that a spread of a string in an array literal splices
// its code points through value.BStr.CodePoints, the same code-point walk for...of
// over a string takes.
func TestStringSpreadEmits(t *testing.T) {
	const src = "export function b(s: string): string[] { return [...s]; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".CodePoints()...") {
		t.Errorf("string spread did not splice CodePoints:\n%s", source)
	}
}

// TestStringSpreadRuns compiles and runs a spread of a string, proving the code
// points splice in order and an astral character stays one element.
func TestStringSpreadRuns(t *testing.T) {
	const src = "const a = [...\"a\\u{1F600}b\"];\nconsole.log(a.length);\nconsole.log(a[0] + a[2]);\n"
	out := runProgramGo(t, src)
	if out != "3\nab\n" {
		t.Errorf("string spread = %q, want %q", out, "3\nab\n")
	}
}
