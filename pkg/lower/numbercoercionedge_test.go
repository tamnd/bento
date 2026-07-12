package lower

import (
	"strings"
	"testing"
)

// TestNumberNoArgEmits pins that Number() with no argument lowers to a float64
// zero, the coercion-edge count the spec fixes to +0 rather than NaN, the mirror
// of String()'s empty-string default.
func TestNumberNoArgEmits(t *testing.T) {
	const src = "export function z(): number { return Number(); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "return 0") {
		t.Errorf("Number() did not lower to a zero literal:\n%s", source)
	}
}

// TestNumberNoArgRuns compiles and runs Number() to prove it is +0 end to end, a
// value that is zero and equal to 0.
func TestNumberNoArgRuns(t *testing.T) {
	const src = "const n = Number();\nconsole.log(n);\nconsole.log(n === 0);\n"
	out := runProgramGo(t, src)
	if out != "0\ntrue\n" {
		t.Errorf("Number() = %q, want %q", out, "0\ntrue\n")
	}
}
