package lower

import (
	"strings"
	"testing"
)

// TestStringNoArgEmits pins that String() with no argument lowers to the empty
// string, the coercion-edge count the spec fixes to "" rather than "undefined".
func TestStringNoArgEmits(t *testing.T) {
	const src = "export function e(): string { return String(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `value.FromGoString("")`) {
		t.Errorf("String() did not lower to the empty string:\n%s", source)
	}
}

// TestStringNoArgRuns compiles and runs String() to prove it is the empty string
// end to end.
func TestStringNoArgRuns(t *testing.T) {
	const src = "const a = String();\nconsole.log(a.length);\nconsole.log(a === \"\");\n"
	out := runProgramGo(t, src)
	if out != "0\ntrue\n" {
		t.Errorf("String() = %q, want %q", out, "0\ntrue\n")
	}
}
