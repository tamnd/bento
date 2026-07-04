package lower

import (
	"strings"
	"testing"
)

// TestDoWhileEmitsForWithTrailingBreak pins that a do...while lowers to a bare Go
// for whose body runs once before the test, with the test spelled as the negated
// condition breaking out. The comparison flips rather than taking a leading not, so
// s < 3 breaks on s >= 3.
func TestDoWhileEmitsForWithTrailingBreak(t *testing.T) {
	const src = "export function f(): number { let s = 0; do { s = s + 1; } while (s < 3); return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for {") {
		t.Errorf("do...while did not lower to a bare for:\n%s", source)
	}
	if !strings.Contains(source, "if s >= 3 {") || !strings.Contains(source, "break") {
		t.Errorf("do...while did not lower the test to a trailing negated break:\n%s", source)
	}
}

// TestDoWhileTruthyConditionFoldsTruthiness proves a do...while whose condition is a
// bare number rides JavaScript truthiness the way a while with the same condition
// does, so a nonzero, non-NaN number keeps the loop running and the negation of that
// test breaks out. The two loops agree on how they lower a truthy condition.
func TestDoWhileTruthyConditionFoldsTruthiness(t *testing.T) {
	const src = "export function f(): number { let s = 3; do { s = s - 1; } while (s); return s; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "if !(s != 0 && s == s) {") {
		t.Errorf("do...while over a number did not fold to a negated truthiness break:\n%s", source)
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
