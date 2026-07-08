package lower

import (
	"strings"
	"testing"
)

// TestLabeledLoopEmitsLabelAndTargetedBranch pins that a labeled loop lowers to a Go
// labeled statement and that a labeled continue inside the body names the same
// label, so the branch targets the outer loop the way the source reads it.
func TestLabeledLoopEmitsLabelAndTargetedBranch(t *testing.T) {
	const src = "export function f(): number {\n" +
		"  let n = 0;\n" +
		"  outer: for (let i = 0; i < 3; i = i + 1) {\n" +
		"    for (let j = 0; j < 3; j = j + 1) {\n" +
		"      if (j === 1) { continue outer; }\n" +
		"      n = n + 1;\n" +
		"    }\n" +
		"  }\n" +
		"  return n;\n" +
		"}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "outer:") {
		t.Errorf("labeled loop did not emit the Go label:\n%s", source)
	}
	if !strings.Contains(source, "continue outer") {
		t.Errorf("labeled continue did not target the label:\n%s", source)
	}
}

// TestLabeledBreakTargetsLabel pins that a labeled break lowers to a Go break that
// names its label, the break counterpart of the labeled continue.
func TestLabeledBreakTargetsLabel(t *testing.T) {
	const src = "export function f(): number {\n" +
		"  let n = 0;\n" +
		"  outer: for (let i = 0; i < 3; i = i + 1) {\n" +
		"    for (let j = 0; j < 3; j = j + 1) {\n" +
		"      if (j === 1) { break outer; }\n" +
		"      n = n + 1;\n" +
		"    }\n" +
		"  }\n" +
		"  return n;\n" +
		"}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "break outer") {
		t.Errorf("labeled break did not target the label:\n%s", source)
	}
}

// TestLabeledLoopWithoutTargetDropsLabel proves an unused label is dropped rather
// than emitted, since Go rejects a label the body never targets while JavaScript
// accepts it, so the lowering must not leave a dead label behind.
func TestLabeledLoopWithoutTargetDropsLabel(t *testing.T) {
	const src = "export function f(): number {\n" +
		"  let n = 0;\n" +
		"  loop: for (let i = 0; i < 3; i = i + 1) { n = n + 1; }\n" +
		"  return n;\n" +
		"}\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "loop:") {
		t.Errorf("unused label should be dropped, Go rejects a label the body never targets:\n%s", source)
	}
}

// TestLabeledLoopsRun builds and runs the generated Go, proving a labeled continue
// skips to the next outer iteration and a labeled break leaves the whole nest.
func TestLabeledLoopsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  let cont = 0;
  outer: for (let i = 0; i < 3; i = i + 1) {
    for (let j = 0; j < 3; j = j + 1) {
      if (j === 1) { continue outer; }
      cont = cont + 1;
    }
  }
  console.log(cont);

  let brk = 0;
  top: for (let i = 0; i < 3; i = i + 1) {
    for (let j = 0; j < 3; j = j + 1) {
      if (i === 1 && j === 1) { break top; }
      brk = brk + 1;
    }
  }
  console.log(brk);
}
run();
`
	if got, want := runProgramGo(t, src), "3\n4\n"; got != want {
		t.Fatalf("labeled loops printed %q, want %q", got, want)
	}
}

// TestLabeledBlockBreakInnerLoop pins that a break naming a labeled block jumps out
// of both an inner loop and the block. JavaScript lets a break name a labeled block,
// and Go accepts a labeled break only on a for, switch, or select, so the block
// lowers to a one-shot for loop the label sits on. The break past ten proves the
// jump leaves the whole block, not just the inner while.
func TestLabeledBlockBreakInnerLoop(t *testing.T) {
	skipIfShort(t)
	const src = "let i = 0;\n" +
		"woohoo: {\n" +
		"  while (true) {\n" +
		"    i = i + 1;\n" +
		"    if (i === 10) { break woohoo; }\n" +
		"  }\n" +
		"  i = 99;\n" +
		"}\n" +
		"console.log(i);\n"
	if got, want := runProgramGo(t, src), "10\n"; got != want {
		t.Fatalf("labeled block break past an inner loop printed %q, want %q", got, want)
	}
}
