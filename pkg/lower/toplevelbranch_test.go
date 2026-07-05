package lower

import (
	"strings"
	"testing"
)

// A do...while, a labeled loop, and a break or continue are left unnamed by the
// frontend. The function-body statement lowering already recognizes each by its
// shape, and the module top-level now routes an unnamed statement to the main
// body the same way, so these forms lower at the top level too, not only inside a
// function.

// TestTopLevelDoWhileLowers proves a top-level do...while assembles into main as
// the Go do-while idiom, a bare for whose condition is checked at the bottom.
func TestTopLevelDoWhileLowers(t *testing.T) {
	const src = "let i = 0; let n = 0; do { n = n + 1; i = i + 1; } while (i < 3);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for {") {
		t.Errorf("top-level do...while did not lower to a bare for:\n%s", source)
	}
	if !strings.Contains(source, "break") {
		t.Errorf("top-level do...while did not emit the bottom-of-loop break:\n%s", source)
	}
}

// TestTopLevelLabeledLoopLowers proves a top-level labeled loop assembles into
// main with the Go label and a continue that names it.
func TestTopLevelLabeledLoopLowers(t *testing.T) {
	const src = "let n = 0; outer: for (let i = 0; i < 3; i++) { for (let j = 0; j < 3; j++) { if (j === 1) continue outer; n = n + 1; } }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "outer:") {
		t.Errorf("top-level labeled loop did not emit the Go label:\n%s", source)
	}
	if !strings.Contains(source, "continue outer") {
		t.Errorf("top-level labeled loop did not emit the labeled continue:\n%s", source)
	}
}

// TestTopLevelDoWhileRuns builds and runs the assembled Go so the bottom-checked
// loop is proven to run its body once before the first test and stop on the
// condition, matching the interpreter.
func TestTopLevelDoWhileRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let i = 0;
let n = 0;
do {
  n = n + 1;
  i = i + 1;
} while (i < 3);
console.log(n);

let once = 0;
do { once = once + 1; } while (false);
console.log(once);
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("top-level do...while printed %q, want %q", got, want)
	}
}

// TestTopLevelLabeledLoopRuns builds and runs a top-level labeled continue so the
// jump to the outer loop is proven, counting one increment per outer iteration.
func TestTopLevelLabeledLoopRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let n = 0;
outer: for (let i = 0; i < 3; i++) {
  for (let j = 0; j < 3; j++) {
    if (j === 1) continue outer;
    n = n + 1;
  }
}
console.log(n);
`
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("top-level labeled loop printed %q, want %q", got, want)
	}
}
