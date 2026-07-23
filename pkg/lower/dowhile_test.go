package lower

import (
	"strings"
	"testing"
)

// TestDoWhileEmitsForWithConditionPost pins that a do...while lowers to a Go for
// whose body runs once before the test, with the condition riding the post clause
// behind a flag seeded true. The condition renders as itself, s < 3, not a negation,
// because the post re-tests it each turn rather than breaking on its opposite.
func TestDoWhileEmitsForWithConditionPost(t *testing.T) {
	const src = "export function f(): number { let s = 0; do { s = s + 1; } while (s < 3); return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ":= true;") {
		t.Errorf("do...while did not seed a flag true in the for init:\n%s", source)
	}
	if !strings.Contains(source, "= s < 3 {") {
		t.Errorf("do...while did not put the condition in the for post clause:\n%s", source)
	}
}

// TestDoWhileTruthyConditionFoldsTruthiness proves a do...while whose condition is a
// bare number rides JavaScript truthiness the way a while with the same condition
// does, so a nonzero, non-NaN number keeps the loop running. The condition folds the
// same in the post clause as a while folds it in its test.
func TestDoWhileTruthyConditionFoldsTruthiness(t *testing.T) {
	const src = "export function f(): number { let s = 3; do { s = s - 1; } while (s); return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "= s != 0 && s == s {") {
		t.Errorf("do...while over a number did not fold truthiness in the post clause:\n%s", source)
	}
}

// TestDoWhileRuns builds and runs the generated Go, proving the body runs once even
// when the condition is false at entry, which is the whole point of do...while.
func TestDoWhileRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  let s = 0;
  do {
    s = s + 1;
  } while (s < 3);
  console.log(s);

  let once = 0;
  do {
    once = once + 1;
  } while (once > 5);
  console.log(once);
}
run();
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("do...while printed %q, want %q", got, want)
	}
}

// TestDoWhileContinueTerminates guards the reason the condition rides the post
// clause: a continue inside the body must jump to the condition re-test, so a
// do...while(false) with a continue in its body runs once and exits. The old
// trailing-break shape sat the test past the continue, which never ran and spun
// forever, the block-scope/leave/x-before-continue test262 timeout.
func TestDoWhileContinueTerminates(t *testing.T) {
	skipIfShort(t)
	const src = `
function run(): void {
  let hits = 0;
  do {
    hits = hits + 1;
    {
      continue;
    }
  } while (false);
  console.log(hits);
}
run();
`
	if got, want := runProgramGo(t, src), "1\n"; got != want {
		t.Fatalf("do...while with a continue printed %q, want %q", got, want)
	}
}
