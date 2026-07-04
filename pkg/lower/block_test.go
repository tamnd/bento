package lower

import (
	"strings"
	"testing"
)

// TestStandaloneBlockScopesLocals pins that a bare block outside a function body
// lowers to a Go block, so a binding it introduces stays scoped to the braces and
// does not leak to the enclosing function. Go spells the lexical scope the same way,
// so the block maps straight across.
func TestStandaloneBlockScopesLocals(t *testing.T) {
	const src = "export function f(): number { let s = 0; { let x = 2; s = s + x; } return s; }\n"
	source := renderProgram(t, src)
	// The block keeps its braces and the inner binding is a short decl inside them.
	if !strings.Contains(source, "{\n\t\tx := 2") {
		t.Errorf("standalone block did not lower to a scoped Go block:\n%s", source)
	}
}

// TestStandaloneBlockRuns builds and runs the generated Go, proving the inner
// binding is scoped to the block and a same-named binding after the block is a
// separate variable.
func TestStandaloneBlockRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  let s = 0;
  {
    let x = 10;
    s = s + x;
  }
  {
    let x = 20;
    s = s + x;
  }
  console.log(s);
}
run();
`
	if got, want := runProgramGo(t, src), "30\n"; got != want {
		t.Fatalf("standalone block printed %q, want %q", got, want)
	}
}
