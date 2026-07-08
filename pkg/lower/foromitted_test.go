package lower

import (
	"strings"
	"testing"
)

// A for statement may omit any of its three header clauses. for(;;) drops all
// three, for(;cond;) keeps only the condition, for(init;;incr) drops the
// condition. Each shape lowers by reading the clauses off the node by role, so an
// omitted condition becomes Go's bare for and an omitted incrementor becomes no
// post clause, rather than every omission handing back.

// TestForNoConditionLowersToBareLoop proves an omitted condition lowers to Go's
// infinite for with no condition expression.
func TestForNoConditionLowersToBareLoop(t *testing.T) {
	const src = "let i = 0; for (let k = 0; ; k++) { i = i + 1; if (i >= 3) break; } console.log(i);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for k := 0.0; ; k++ {") {
		t.Errorf("for with an omitted condition did not keep an empty condition slot:\n%s", source)
	}
}

// TestForNoInitNoPostLowersToBareLoop proves for(;;) lowers to Go's bare for,
// with neither an init nor a condition nor a post clause.
func TestForNoInitNoPostLowersToBareLoop(t *testing.T) {
	const src = "let i = 0; for (;;) { i = i + 1; if (i >= 3) break; } console.log(i);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for {") {
		t.Errorf("for(;;) did not lower to a bare Go for:\n%s", source)
	}
}

// TestForConditionOnlyLowersToWhileForm proves for(;cond;) lowers to Go's
// condition-only for, the shape Go writes a while as.
func TestForConditionOnlyLowersToWhileForm(t *testing.T) {
	const src = "let j = 0; for (; j < 3; ) { j = j + 1; } console.log(j);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for j < 3 {") {
		t.Errorf("for(;cond;) did not lower to a condition-only for:\n%s", source)
	}
}

// TestOmittedClauseForLoopsRun builds and runs every omitted-clause shape and
// checks the results against the JavaScript answers, so the behavior is proven,
// not just the emitted shape: an infinite loop broken by a guard, a
// condition-only loop, a loop whose only header part is the initializer, and a
// loop missing its incrementor.
func TestOmittedClauseForLoopsRun(t *testing.T) {
	skipIfShort(t)
	const src = `
let i = 0;
for (;;) {
  i = i + 1;
  if (i >= 3) break;
}
console.log(i);

let j = 0;
for (; j < 3; ) {
  j = j + 1;
}
console.log(j);

let s = 0;
for (let k = 0; ; k++) {
  s = s + k;
  if (k >= 4) break;
}
console.log(s);

let t = 0;
for (let m = 0; m < 4; ) {
  t = t + m;
  m = m + 1;
}
console.log(t);
`
	if got, want := runProgramGo(t, src), "3\n3\n10\n6\n"; got != want {
		t.Fatalf("omitted-clause for loops printed %q, want %q", got, want)
	}
}

// TestForNoStepUnusedCounterRuns proves a loop whose counter no clause reads
// still builds and runs: the unread binding takes a blank assignment the way an
// unused let does, so Go's declared-and-unused rule does not reject it. The
// condition is a constant false, so the body never runs and the counter stays at
// its start.
func TestForNoStepUnusedCounterRuns(t *testing.T) {
	skipIfShort(t)
	const src = "let count = 0;\nfor (let i = 0; false;) {\n  count++;\n}\nconsole.log(count);\n"
	if got, want := runProgramGo(t, src), "0\n"; got != want {
		t.Fatalf("loop over a false condition printed %q, want %q", got, want)
	}
}

// TestForNoStepUnusedCounterBlanks proves the unread counter gets its blank
// assignment rather than folding into the for's := init, which has no place to
// hang one.
func TestForNoStepUnusedCounterBlanks(t *testing.T) {
	const src = "let count = 0;\nfor (let i = 0; false;) {\n  count++;\n}\nconsole.log(count);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "_ = i") {
		t.Errorf("an unread loop counter did not get a blank assignment:\n%s", source)
	}
}

// TestForDestructuringInitHandsBack proves a destructuring initializer stays a
// later slice: its binding is a pattern, not a plain counter, so the loop hands
// back rather than mangling the pattern text into one Go name.
func TestForDestructuringInitHandsBack(t *testing.T) {
	const src = "let n = 0;\nfor (const [a, b] = [3, 4]; n < 1;) {\n  n = a + b;\n}\nconsole.log(n);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "destructuring initializer") {
		t.Errorf("for with a destructuring initializer handback reason = %q, want it to name the destructuring initializer", reason)
	}
}

// TestForExpressionInitLowersToForInit proves an assignment initializer, one that
// writes an existing binding rather than declaring a new one, folds straight into
// Go's for init clause instead of a wrapping block, so for(i=0;...) reads the way a
// developer writes it.
func TestForExpressionInitLowersToForInit(t *testing.T) {
	const src = "let i = 0; for (i = 0; i < 3; i++) { } console.log(i);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "for i = 0; i < 3; i++ {") {
		t.Errorf("for with an assignment initializer did not fold into the for init clause:\n%s", source)
	}
}

// TestForExpressionInitRuns builds and runs an assignment-initialized loop and a
// comma-of-assignments initializer, whose two writes fuse into one parallel
// assignment the way a comma post clause does, and checks the results against the
// JavaScript answers.
func TestForExpressionInitRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let i = 0;
let sum = 0;
for (i = 0; i < 4; i++) {
  sum = sum + i;
}
console.log(sum);

let a = 0;
let b = 0;
let steps = 0;
for (a = 0, b = 10; a < b; a++) {
  steps = steps + 1;
}
console.log(steps);
`
	if got, want := runProgramGo(t, src), "6\n10\n"; got != want {
		t.Fatalf("assignment-initialized for loops printed %q, want %q", got, want)
	}
}
