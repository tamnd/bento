package lower

import (
	"strings"
	"testing"
)

// A destructuring for...of over an array of arrays, for (const [a, b] of pairs),
// lowers to a range loop whose element is bound to a generated temporary and
// destructured at the top of the body. The range value is fresh each iteration, so
// the positional reads see that iteration's element with no reset.

// TestForOfDestructureBindsElements proves each name lowers to a positional AtI read
// off the generated range temporary.
func TestForOfDestructureBindsElements(t *testing.T) {
	const src = "const pairs: number[][] = [[1, 2]];\nfor (const [a, b] of pairs) {\n  console.log(a + b);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "range pairs.Elems()") {
		t.Errorf("loop did not range over the array elements:\n%s", source)
	}
	if !strings.Contains(source, "a := ") || !strings.Contains(source, ".AtI(0)") {
		t.Errorf("first name did not read position 0:\n%s", source)
	}
	if !strings.Contains(source, "b := ") || !strings.Contains(source, ".AtI(1)") {
		t.Errorf("second name did not read position 1:\n%s", source)
	}
}

// TestForOfDestructureRuns builds and runs a destructuring loop so the positional
// reads are proven to pick the right elements each iteration.
func TestForOfDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const pairs: number[][] = [[1, 2], [3, 4], [5, 6]];
for (const [a, b] of pairs) {
  console.log(a + " + " + b + " = " + (a + b));
}
`
	if got, want := runProgramGo(t, src), "1 + 2 = 3\n3 + 4 = 7\n5 + 6 = 11\n"; got != want {
		t.Fatalf("for-of destructure printed %q, want %q", got, want)
	}
}

// TestForOfDestructureDropsUnusedName proves a name the body never reads is dropped
// rather than bound, so the Go loop does not carry an unused local.
func TestForOfDestructureDropsUnusedName(t *testing.T) {
	const src = "const pairs: number[][] = [[1, 2]];\nfor (const [a, b] of pairs) {\n  console.log(b);\n}\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "a := ") {
		t.Errorf("unused first name should have been dropped:\n%s", source)
	}
	if !strings.Contains(source, ".AtI(1)") {
		t.Errorf("used second name should still read position 1:\n%s", source)
	}
}

// TestForOfDestructureRunsWithUnused builds and runs a loop that reads only the second
// element, so the dropped binding is proven not to break the loop.
func TestForOfDestructureRunsWithUnused(t *testing.T) {
	skipIfShort(t)
	const src = `
const pairs: number[][] = [[1, 10], [2, 20]];
for (const [a, b] of pairs) {
  console.log(b);
}
`
	if got, want := runProgramGo(t, src), "10\n20\n"; got != want {
		t.Fatalf("for-of destructure with unused name printed %q, want %q", got, want)
	}
}

// TestForOfDestructureDuplicateNameBindsLast proves a pattern that repeats a name,
// for (var [x, x] of pairs), binds x once at the last position rather than emitting
// two `x :=` lines. JavaScript binds the repeated name a single time with the last
// element winning, and the positional reads are pure AtI lookups, so the earlier read
// is a dead store. Emitting both would also make Go reject the second `x :=` as no new
// variables on the left. for-of/head-var-bound-names-dup exercises this.
func TestForOfDestructureDuplicateNameBindsLast(t *testing.T) {
	const src = "for (var [x, x] of [[1, 2]]) {\n  console.log(String(x));\n}\n"
	source := renderProgram(t, src)
	if got := strings.Count(source, "x := "); got != 1 {
		t.Errorf("repeated name should bind once, saw %d `x :=` lines:\n%s", got, source)
	}
	if !strings.Contains(source, ".AtI(1)") || strings.Contains(source, ".AtI(0)") {
		t.Errorf("repeated name should read the last position 1, not 0:\n%s", source)
	}
}

// TestForOfDestructureDuplicateNameRuns builds and runs the repeated-name loop so the
// last-wins binding is proven end to end: x holds the second element each iteration.
func TestForOfDestructureDuplicateNameRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
var iterCount = 0;
for (var [x, x] of [[1, 2]]) {
  console.log(String(x));
  iterCount += 1;
}
console.log(String(iterCount));
`
	if got, want := runProgramGo(t, src), "2\n1\n"; got != want {
		t.Fatalf("for-of duplicate destructure printed %q, want %q", got, want)
	}
}

// TestForOfDestructureHoleHandsBack proves a pattern with a hole hands back.
func TestForOfDestructureHoleHandsBack(t *testing.T) {
	const src = "const pairs: number[][] = [[1, 2]];\nfor (const [, b] of pairs) {\n  console.log(b);\n}\n"
	renderProgramHandBack(t, src)
}

// TestForOfDestructureRestHandsBack proves a rest element hands back.
func TestForOfDestructureRestHandsBack(t *testing.T) {
	const src = "const rows: number[][] = [[1, 2, 3]];\nfor (const [a, ...rest] of rows) {\n  console.log(a);\n  console.log(rest.length);\n}\n"
	renderProgramHandBack(t, src)
}

// TestForOfDestructureNonArrayElementHandsBack proves destructuring over an array
// whose element is not itself an array hands back, since a positional read needs an
// array element to index. A string element is iterable, so the pattern type-checks,
// but it has no array element type to read through AtI.
func TestForOfDestructureNonArrayElementHandsBack(t *testing.T) {
	const src = "const words: string[] = [\"hi\", \"yo\"];\nfor (const [a, b] of words) {\n  console.log(a + b);\n}\n"
	renderProgramHandBack(t, src)
}
