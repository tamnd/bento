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

// TestForOfDestructureRestRuns proves a rest element gathers the tail each iteration,
// binding the fixed head and the remaining elements the same way a top-level array
// pattern does.
func TestForOfDestructureRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const rows: number[][] = [[1, 2, 3], [4, 5, 6]];\nfor (const [a, ...rest] of rows) {\n  console.log(a);\n  console.log(rest.length);\n}\n"
	if got, want := runProgramGo(t, src), "1\n2\n4\n2\n"; got != want {
		t.Fatalf("for-of rest destructure printed %q, want %q", got, want)
	}
}

// TestForOfDestructureNonArrayElementHandsBack proves destructuring over an array
// whose element is not itself an array hands back, since a positional read needs an
// array element to index. A string element is iterable, so the pattern type-checks,
// but it has no array element type to read through AtI.
func TestForOfDestructureNonArrayElementHandsBack(t *testing.T) {
	const src = "const words: string[] = [\"hi\", \"yo\"];\nfor (const [a, b] of words) {\n  console.log(a + b);\n}\n"
	renderProgramHandBack(t, src)
}

// TestForOfObjectDestructureRuns proves a for...of whose element is an object binds
// each property through the shared object binder, the case where the bound name's type
// differs from the iterable's element type.
func TestForOfObjectDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const items = [{ a: 1, b: 2 }, { a: 3, b: 4 }];
for (const { a, b } of items) {
  console.log(a + b);
}
`
	if got, want := runProgramGo(t, src), "3\n7\n"; got != want {
		t.Fatalf("for-of object destructure printed %q, want %q", got, want)
	}
}

// TestForOfObjectDestructureRenameRuns proves a renamed property in the loop pattern
// binds the target each iteration off the source field.
func TestForOfObjectDestructureRenameRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const items = [{ a: 1 }, { a: 2 }];\nfor (const { a: x } of items) {\n  console.log(x);\n}\n"
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("for-of object rename destructure printed %q, want %q", got, want)
	}
}

// TestForOfNestedDestructureRuns proves a nested array pattern in the loop binding
// reads the inner elements off the value the outer position selected each iteration.
func TestForOfNestedDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const m: number[][][] = [[[1, 2], [3, 4]]];\nfor (const [[a, b], [c, d]] of m) {\n  console.log(a + b + c + d);\n}\n"
	if got, want := runProgramGo(t, src), "10\n"; got != want {
		t.Fatalf("for-of nested destructure printed %q, want %q", got, want)
	}
}

// TestForOfTupleDestructureBindsFields proves a for...of whose element is a tuple
// binds each name off its positional struct field, for (const [k, v] of pairs)
// becoming k := e.E0; v := e.E1, the tuple counterpart of the AtI reads an
// array-of-arrays element takes (typed/05 T7, delivery slice 5b).
func TestForOfTupleDestructureBindsFields(t *testing.T) {
	const src = "const pairs: [string, number][] = [[\"a\", 1]];\nfor (const [k, v] of pairs) {\n  console.log(k + v);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "range pairs.Elems()") {
		t.Errorf("loop did not range over the array elements:\n%s", source)
	}
	if !strings.Contains(source, "k := ") || !strings.Contains(source, ".E0") {
		t.Errorf("first name did not read field E0:\n%s", source)
	}
	if !strings.Contains(source, "v := ") || !strings.Contains(source, ".E1") {
		t.Errorf("second name did not read field E1:\n%s", source)
	}
	if strings.Contains(source, ".AtI(") {
		t.Errorf("tuple element read should be a field selector, not a bounds-checked AtI:\n%s", source)
	}
}

// TestForOfTupleDestructureRuns builds and runs a for...of over a tuple array so the
// positional field reads are proven to pick the right element of each pair.
func TestForOfTupleDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const pairs: [string, number][] = [["a", 1], ["b", 2], ["c", 3]];
for (const [k, v] of pairs) {
  console.log(k + "=" + v);
}
`
	if got, want := runProgramGo(t, src), "a=1\nb=2\nc=3\n"; got != want {
		t.Fatalf("for-of tuple destructure printed %q, want %q", got, want)
	}
}

// TestForOfTupleDestructureRepeatedNameHandsBack proves a var pattern that repeats a
// bound name over a tuple source hands back rather than emit two `x :=` reads Go
// would reject as no-new-variables, the zero-fail edge the flat array path dedupes.
func TestForOfTupleDestructureRepeatedNameHandsBack(t *testing.T) {
	const src = "const pairs: [number, number][] = [[1, 2]];\nfor (var [x, x] of pairs) {\n  console.log(String(x));\n}\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "repeats a bound name") {
		t.Fatalf("repeated-name tuple for-of handback reason = %q, want a repeated-name reason", reason)
	}
}

// TestForOfObjectDestructureUnusedHandsBack proves a for...of object pattern whose
// bound name the body never reads hands back rather than emit a Go local the compiler
// rejects as declared-and-not-used, since the shared binder cannot drop it the way the
// flat array path does.
func TestForOfObjectDestructureUnusedHandsBack(t *testing.T) {
	const src = "const items = [{ a: 1, b: 2 }];\nfor (const { a, b } of items) {\n  console.log(a);\n}\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "unused bound name") {
		t.Fatalf("for-of unused handback reason = %q, want an unused-bound-name reason", reason)
	}
}

// TestForOfDynamicEntriesDestructureHandsBack proves a destructuring for...of over
// Object.entries on a dynamic receiver hands back rather than emit box.Elems(), which
// does not compile: the checker types the entries result as a tuple array, but its
// dynamic receiver lowers the whole result to a boxed value.Value with no backing
// slice for the range to walk. The fixed-shape receiver keeps its concrete fold.
func TestForOfDynamicEntriesDestructureHandsBack(t *testing.T) {
	const src = "const o: any = { a: 1, b: 2 };\nfor (const [k, v] of Object.entries(o)) {\n  console.log(k, String(v));\n}\n"
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "boxed value") {
		t.Fatalf("dynamic entries for-of handback reason = %q, want a boxed-value reason", reason)
	}
}
