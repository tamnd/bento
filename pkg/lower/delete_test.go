package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestDeleteDynamicMemberLowers pins that delete o.k on a dynamic receiver lowers
// to the runtime property removal, value.Value.Delete keyed by the source name.
func TestDeleteDynamicMemberLowers(t *testing.T) {
	const src = "const obj: any = { a: 1 };\nconst gone: boolean = delete obj.a;\nconsole.log(gone);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, `.Delete(value.FromGoString("a"))`) {
		t.Fatalf("delete of a dynamic member did not lower to a runtime removal:\n%s", source)
	}
}

// TestDeleteDynamicMemberRuns builds and runs delete o.k on a dynamic object: the
// removal yields true, the deleted property reads back undefined, and a sibling
// property is untouched.
func TestDeleteDynamicMemberRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const obj: any = { a: 1, b: 2 };\n" +
		"const gone: boolean = delete obj.a;\n" +
		"console.log(gone);\n" +
		"console.log(obj.a);\n" +
		"console.log(obj.b);\n"
	got := runProgramGo(t, src)
	want := "true\nundefined\n2\n"
	if got != want {
		t.Fatalf("delete dynamic member program printed %q, want %q", got, want)
	}
}

// TestDeleteDynamicElementLowers pins that delete o[k] with a number key on a
// dynamic receiver lowers to DeleteIndex, the numeric-key runtime removal.
func TestDeleteDynamicElementLowers(t *testing.T) {
	const src = "const arr: any = [1, 2];\nconst gone: boolean = delete arr[0];\nconsole.log(gone);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".DeleteIndex(") {
		t.Fatalf("delete of a dynamic number-keyed element did not lower to DeleteIndex:\n%s", source)
	}
}

// TestDeleteDynamicElementRuns builds and runs the computed-key form: a string
// key removes an object property, and a number key clears an array element to a
// hole without changing length.
func TestDeleteDynamicElementRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const obj: any = { a: 1, b: 2 };\n" +
		"const g1: boolean = delete obj[\"a\"];\n" +
		"console.log(g1);\n" +
		"console.log(obj.a);\n" +
		"console.log(obj.b);\n" +
		"const arr: any = [10, 20, 30];\n" +
		"const g2: boolean = delete arr[1];\n" +
		"console.log(g2);\n" +
		"console.log(arr[1]);\n" +
		"console.log(arr.length);\n" +
		"console.log(arr[2]);\n"
	got := runProgramGo(t, src)
	want := "true\nundefined\n2\ntrue\nundefined\n3\n30\n"
	if got != want {
		t.Fatalf("delete dynamic element program printed %q, want %q", got, want)
	}
}

// TestDeleteMissingPropertyRuns pins the missing-property result: delete of a key
// the object never carried yields true, the boolean JavaScript gives for an absent
// property, and the object's real properties are untouched.
func TestDeleteMissingPropertyRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const obj: any = { a: 1 };\n" +
		"const gone: boolean = delete obj.b;\n" +
		"console.log(gone);\n" +
		"console.log(obj.a);\n"
	got := runProgramGo(t, src)
	want := "true\n1\n"
	if got != want {
		t.Fatalf("delete of a missing property printed %q, want %q", got, want)
	}
}

// TestDeleteNonReferenceFolds pins that delete over a non-reference operand, which
// the checker flags 2703 and the front door admits, folds to the constant true:
// the operand is side-effect free, so JavaScript's evaluate-then-yield-true drops
// it and delete becomes true.
func TestDeleteNonReferenceFolds(t *testing.T) {
	const src = "const b: boolean = delete 0;\nconsole.log(b);\n"
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "b := true") {
		t.Fatalf("delete of a non-reference operand did not fold to true:\n%s", source)
	}
}

// TestDeleteNonReferenceRuns builds and runs a non-reference delete: it prints
// true, the boolean JavaScript gives when the operand is not a property reference.
func TestDeleteNonReferenceRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const b: boolean = delete (1 + 2);\nconsole.log(b);\n"
	got := runProgramGoTolerant(t, src)
	if got != "true\n" {
		t.Fatalf("delete of a non-reference operand printed %q, want %q", got, "true\n")
	}
}

// TestDeleteSideEffectingNonReferenceHandsBack pins the boundary: a non-reference
// operand with a side effect cannot fold to true without dropping that effect, so
// it hands back until the sequencing slice lands.
func TestDeleteSideEffectingNonReferenceHandsBack(t *testing.T) {
	const src = "function eff(): number { console.log(\"ran\"); return 1; }\nfunction f(): boolean { return delete eff(); }\n"
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "side effect") {
		t.Errorf("hand-back reason = %q, want it to mention a side effect", nyl.Reason)
	}
}

// TestDeleteStaticMemberHandsBack pins the boundary: a property whose fixed shape
// makes it a Go struct field has no runtime slot to remove, so delete over it
// hands back for the object descriptor model a later phase builds.
func TestDeleteStaticMemberHandsBack(t *testing.T) {
	const src = "interface P { a?: number; }\nfunction f(p: P): boolean { return delete p.a; }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "object descriptor model") {
		t.Errorf("hand-back reason = %q, want it to mention the object descriptor model", nyl.Reason)
	}
}
