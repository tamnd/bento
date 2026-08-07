package lower

import (
	"strings"
	"testing"
)

// TestTupleElementWriteAssignsTheField pins the shape of this slice: a tuple's Go form
// is a positional struct, so the write to a position is the field assignment its read
// already selects.
func TestTupleElementWriteAssignsTheField(t *testing.T) {
	src := "const t: [string, number] = [\"a\", 2];\nt[0] = \"b\";\nt[1] = 5;\nconsole.log(t[0], t[1]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "t.E0 = value.FromGoString(\"b\")") {
		t.Errorf("the write to position 0 did not assign E0:\n%s", source)
	}
	if !strings.Contains(source, "t.E1 = 5") {
		t.Errorf("the write to position 1 did not assign E1:\n%s", source)
	}
}

// TestTupleElementCompoundWriteReadsTheSameField pins the compound form, which is the
// desugaring t[i] = t[i] <op> v with both halves naming the one field.
func TestTupleElementCompoundWriteReadsTheSameField(t *testing.T) {
	src := "const t: [string, number] = [\"a\", 2];\nt[1] += 3;\nt[0] += \"b\";\nconsole.log(t[0], t[1]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "t.E1 = t.E1 + 3") {
		t.Errorf("the compound number write did not read and store the field:\n%s", source)
	}
	if !strings.Contains(source, "t.E0 = ") || !strings.Contains(source, "Concat") {
		t.Errorf("the compound string write did not concatenate:\n%s", source)
	}
}

// TestTupleElementStepIsAGoStep pins that ++ and -- on a number position are the Go
// step on that field, since a statement discards the result and the prefix and postfix
// forms are then the same store.
func TestTupleElementStepIsAGoStep(t *testing.T) {
	src := "const t: [number, number] = [1, 2];\nt[0]++;\n--t[1];\nconsole.log(t[0], t[1]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "t.E0++") {
		t.Errorf("the postfix step did not lower to a Go step:\n%s", source)
	}
	if !strings.Contains(source, "t.E1--") {
		t.Errorf("the prefix step did not lower to a Go step:\n%s", source)
	}
}

// TestTupleElementWriteThroughAFactoryBinding pins that a binding initialized from a
// function whose every return is an array literal is holding a fresh array, which is
// how the array a program writes to usually arrives.
func TestTupleElementWriteThroughAFactoryBinding(t *testing.T) {
	src := "function mk(n: number): [number, string] {\n  if (n > 0) { return [n, \"pos\"]; }\n  return [0, \"zero\"];\n}\n" +
		"const p = mk(5);\np[0] = 9;\nconsole.log(p[0], p[1]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "p.E0 = 9") {
		t.Errorf("the write through a factory binding did not assign the field:\n%s", source)
	}
}

// TestTupleElementWriteThroughASecondNameHandsBack pins the guard this slice exists
// for. A tuple is a struct held by value, so a second name holds a copy; the source
// shares one array and would see the write through either name.
func TestTupleElementWriteThroughASecondNameHandsBack(t *testing.T) {
	src := "const t: [number, number] = [1, 2];\nconst u = t;\nt[0] = 9;\nconsole.log(u[0]);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "holds or rebinds elsewhere") {
		t.Errorf("the refusal did not name the second holder: %s", reason)
	}
}

// TestTupleElementWriteThroughAParameterHandsBack pins the same rule on the shape it
// most often takes. A tuple parameter is a copy the caller cannot see, so a write
// through it would change nothing the source expects it to change.
func TestTupleElementWriteThroughAParameterHandsBack(t *testing.T) {
	src := "function bump(t: [number, number]): void { t[0] = 9; }\n" +
		"const a: [number, number] = [1, 2];\nbump(a);\nconsole.log(a[0]);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "holds or rebinds elsewhere") {
		t.Errorf("the refusal did not name the parameter's copy: %s", reason)
	}
}

// TestTupleElementWriteOnASharedInitializerHandsBack pins the other half of the guard.
// The binding here is never handed out, but it was handed in an array somebody else
// already holds, which makes it a second holder from the start.
func TestTupleElementWriteOnASharedInitializerHandsBack(t *testing.T) {
	src := "const shared: [number, number] = [1, 2];\nfunction get(): [number, number] { return shared; }\n" +
		"const a = get();\na[0] = 9;\nconsole.log(shared[0], a[0]);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "fresh array") {
		t.Errorf("the refusal did not name the shared initializer: %s", reason)
	}
}

// TestTupleElementWriteAtAComputedIndexHandsBack pins that the index has to be a
// literal inside the arity, since the field a write lands on is chosen at compile time
// and the positions do not share a type.
func TestTupleElementWriteAtAComputedIndexHandsBack(t *testing.T) {
	src := "const t: [number, number] = [1, 2];\nconst i = 1;\nt[i] = 9;\nconsole.log(t[0]);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "not a literal inside the arity") {
		t.Errorf("the refusal did not name the computed index: %s", reason)
	}
}

// TestTupleElementWriteAtAnOptionalPositionHandsBack pins the refusal for a position
// whose field internTuple does not emit at all.
func TestTupleElementWriteAtAnOptionalPositionHandsBack(t *testing.T) {
	src := "const t: [number, number?] = [1, 2];\nt[1] = 9;\nconsole.log(t[0]);\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional position") && !strings.Contains(reason, "not a literal inside the arity") {
		t.Errorf("the refusal did not name the optional position: %s", reason)
	}
}
