package lower

import (
	"strings"
	"testing"
)

// An anonymous function declaration has no name of its own. The only source form
// is a default export, which lowers to a top-level Go function under the
// synthesized Default name while the rest of the program compiles around it.
func TestAnonymousFunctionDeclarationLowers(t *testing.T) {
	src := "export default function () {\n  return 42;\n}\nconsole.log(\"ok\");\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "func Default() float64") {
		t.Fatalf("want a top-level Default function, got:\n%s", got)
	}
}

func TestAnonymousFunctionDeclarationRuns(t *testing.T) {
	skipIfShort(t)
	src := "export default function () {\n  return 42;\n}\nconsole.log(\"ok\");\n"
	if got := runProgramGo(t, src); got != "ok\n" {
		t.Fatalf("got %q, want %q", got, "ok\n")
	}
}
