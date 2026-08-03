package lower

import (
	"strings"
	"testing"
)

// TestAHoleSkipsThePositionItHolds pins what this slice makes true. The name after a hole
// reads the slot the hole pushed it onto, so the read is the second element and no read at
// all is emitted for the first. Before this the whole pattern handed back.
func TestAHoleSkipsThePositionItHolds(t *testing.T) {
	const src = "const arr = [1, 2, 3];\n" +
		"const [, b] = arr;\n" +
		"console.log(b);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "b := arr.AtI(1)") {
		t.Errorf("the name after the hole did not read the second slot:\n%s", got)
	}
	if strings.Contains(got, "AtI(0)") {
		t.Errorf("the hole emitted a read of its own position:\n%s", got)
	}
}

// TestAHoleBetweenTwoNamesKeepsBothIndices pins the middle position. The hole is what makes
// the name after it select index two rather than index one, so both neighbors have to land
// on the indices the source text gives them.
func TestAHoleBetweenTwoNamesKeepsBothIndices(t *testing.T) {
	const src = "const arr = [1, 2, 3];\n" +
		"const [p, , q] = arr;\n" +
		"console.log(p, q);\n"
	got := renderProgram(t, src)
	for _, want := range []string{"p := arr.AtI(0)", "q := arr.AtI(2)"} {
		if !strings.Contains(got, want) {
			t.Errorf("want %q in the lowering:\n%s", want, got)
		}
	}
}

// TestAHoleBeforeARestOffsetsTheTail pins that a hole counts toward the fixed positions a
// rest starts after, so the tail copy begins one element in even though nothing before it
// binds a name.
func TestAHoleBeforeARestOffsetsTheTail(t *testing.T) {
	const src = "const arr = [1, 2, 3];\n" +
		"const [, ...tail] = arr;\n" +
		"console.log(tail.length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "tail := arr.Slice(1)") {
		t.Errorf("the rest did not start after the hole:\n%s", got)
	}
}

// TestATupleHoleReadsTheFieldAfterIt pins the tuple source, which reads named struct fields
// rather than indices. The hole selects no field, and the name after it takes E1.
func TestATupleHoleReadsTheFieldAfterIt(t *testing.T) {
	const src = "const tup: [number, string, boolean] = [1, \"two\", true];\n" +
		"const [, s] = tup;\n" +
		"console.log(s);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "s := tup.E1") {
		t.Errorf("the tuple hole did not push the name onto the second field:\n%s", got)
	}
	if strings.Contains(got, "tup.E0") {
		t.Errorf("the tuple hole read its own field:\n%s", got)
	}
}

// TestAnAssignmentHoleTakesTheBlank pins the assignment form, which stores into targets that
// already exist through one parallel Go assignment. The positions have to stay aligned, so a
// hole keeps its slot on both sides and its read goes to the blank.
func TestAnAssignmentHoleTakesTheBlank(t *testing.T) {
	const src = "const arr = [1, 2, 3];\n" +
		"let b = 0;\n" +
		"[, b] = arr;\n" +
		"console.log(b);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "_, b = arr.AtI(0), arr.AtI(1)") {
		t.Errorf("the assignment hole did not take the blank:\n%s", got)
	}
}

// TestAPatternOfNothingButHolesStillReadsItsSource pins the degenerate shape. Such a pattern
// binds no name at all, yet JavaScript still reads the source, so a plain variable source is
// evaluated to the blank rather than left untouched, which Go would call unused.
func TestAPatternOfNothingButHolesStillReadsItsSource(t *testing.T) {
	const src = "const arr = [1, 2, 3];\n" +
		"const [,] = arr;\n" +
		"console.log(\"ok\");\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "_ = arr") {
		t.Errorf("the all-hole pattern did not read its source:\n%s", got)
	}
}

// TestAHoleInALoopHeadHoldsItsPositionEachIteration pins the loop head, which binds its
// pattern against the element the range holds. The hole is the same hole there, so the name
// after it reads the second element of every row.
func TestAHoleInALoopHeadHoldsItsPositionEachIteration(t *testing.T) {
	const src = "const rows = [[1, 2], [3, 4]];\n" +
		"for (const [, y] of rows) { console.log(y); }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "y := ") || !strings.Contains(got, "AtI(1)") {
		t.Errorf("the loop head hole did not read the second element:\n%s", got)
	}
	if !strings.Contains(got, "range") {
		t.Fatalf("the loop did not lower to a range:\n%s", got)
	}
}

// TestAHoleInAParameterPatternBindsOffTheArgument pins the destructured parameter, which
// classifies its elements through the same core. A hole in a parameter names nothing, so
// the function's own binding is the element after it.
func TestAHoleInAParameterPatternBindsOffTheArgument(t *testing.T) {
	const src = "function skip([, y]: number[]): number { return y; }\n" +
		"console.log(skip([1, 2]));\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "y := ") || !strings.Contains(got, "AtI(1)") {
		t.Errorf("the parameter hole did not bind off the second element:\n%s", got)
	}
}
