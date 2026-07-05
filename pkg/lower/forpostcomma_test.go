package lower

import (
	"strings"
	"testing"
)

// A for loop's post clause holds exactly one Go statement, so a comma of updates
// (the i++, j-- a two-pointer loop walks with) cannot lower to two statements. Its
// operands fuse into one parallel assignment, i, j = i + 1, j - 1, which runs them
// together the way the comma sequence does.

// TestForPostCommaFusesToParallelAssign proves the comma post lowers to a single
// parallel assignment naming every target.
func TestForPostCommaFusesToParallelAssign(t *testing.T) {
	const src = "let s = 0; for (let i = 0, j = 5; i < j; i++, j--) { s = s + 1; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "i, j = i+1, j-1") {
		t.Errorf("for post comma did not fuse to a parallel assignment:\n%s", source)
	}
}

// TestForPostCommaCompoundStep proves a compound step in the comma fuses through
// its base operator, i += 2 becoming i + 2 in the parallel assignment.
func TestForPostCommaCompoundStep(t *testing.T) {
	const src = "let s = 0; for (let i = 0, j = 10; i < j; i += 2, j -= 1) { s = s + 1; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "i, j = i+2, j-1") {
		t.Errorf("for post comma with a compound step did not fuse correctly:\n%s", source)
	}
}

// TestForPostCommaRuns builds and runs a two-pointer loop so the parallel post is
// proven to advance both ends each iteration.
func TestForPostCommaRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let steps = 0;
for (let i = 0, j = 7; i < j; i++, j--) {
  steps = steps + 1;
}
console.log(steps);

let sum = 0;
for (let i = 0, j = 10; i < j; i += 2, j -= 2) {
  sum = sum + 1;
}
console.log(sum);
`
	if got, want := runProgramGo(t, src), "4\n3\n"; got != want {
		t.Fatalf("two-pointer loop printed %q, want %q", got, want)
	}
}

// TestForPostCommaCallHandsBack proves a comma post whose operand is a call, which
// cannot sit on the left of an assignment, hands back rather than emit wrong Go.
func TestForPostCommaCallHandsBack(t *testing.T) {
	const src = "function tick(): void {} let s = 0; for (let i = 0; i < 3; tick(), i++) { s = s + 1; }\n"
	renderProgramHandBack(t, src)
}
