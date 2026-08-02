package lower

import (
	"strings"
	"testing"
)

// TestNestedObjectRestGathers pins what this slice makes true. A rest inside a nested
// pattern gathers off the temporary the outer level selected, the same struct copy the
// top-level pattern builds off its own source. Before this it fell to classifyObjectElem,
// which refuses a rest outright.
func TestNestedObjectRestGathers(t *testing.T) {
	const src = "const deep = { o: { a: 1, b: 2, c: 3 } };\n" +
		"const { o: { a, ...inner } } = deep;\n" +
		"console.log(a, inner.b + inner.c);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "inner := &ObjBC{B: ") {
		t.Errorf("the nested rest did not gather its struct:\n%s", got)
	}
}

// TestNestedObjectRestWithNothingLeftGathersARuntimeObject pins that the nested gather is
// the same one, so it answers the empty object type the same way: a rest with nothing left
// in it holds the structural top type and builds the runtime object, not an empty struct.
func TestNestedObjectRestWithNothingLeftGathersARuntimeObject(t *testing.T) {
	const src = "const deep = { o: { a: 1 } };\n" +
		"const { o: { a, ...inner } } = deep;\n" +
		"console.log(a, Object.keys(inner).length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "inner := value.NewObject()") {
		t.Errorf("the empty nested rest did not gather a runtime object:\n%s", got)
	}
}

// TestForOfHeadObjectRestGathers pins the loop head, which is the same nesting: the head
// binds its pattern against the element the range holds, through bindSubObject. So a rest
// in the head gathers off that element once per iteration.
func TestForOfHeadObjectRestGathers(t *testing.T) {
	const src = "const rows = [{ a: 1, b: 2 }, { a: 3, b: 4 }];\n" +
		"for (const { a, ...r } of rows) { console.log(a, r.b); }\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "r := &ObjB{B: ") {
		t.Errorf("the for...of head rest did not gather its struct:\n%s", got)
	}
	if !strings.Contains(got, "range") {
		t.Fatalf("the loop did not lower to a range:\n%s", got)
	}
}

// TestNestedObjectAssignRestGathers pins the assignment sibling. The target already
// exists, so the gather assigns rather than declares, and the gather itself is the
// declaration form's.
func TestNestedObjectAssignRestGathers(t *testing.T) {
	const src = "const deep = { o: { a: 1, b: 2, c: 3 } };\n" +
		"let oa = 0;\n" +
		"let orest: { b: number; c: number } = { b: 0, c: 0 };\n" +
		"({ o: { a: oa, ...orest } } = deep);\n" +
		"console.log(oa, orest.b + orest.c);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "orest = &ObjBC{B: ") {
		t.Errorf("the nested assignment rest did not gather into its target:\n%s", got)
	}
	// The one `:=` on the name is its own `let`, so the gather added none.
	if n := strings.Count(got, "orest := "); n != 1 {
		t.Errorf("the nested assignment rest declared a new binding (%d declarations):\n%s", n, got)
	}
}

// TestNestedObjectRestNothingReadsTakesTheBlank pins that a rest gathered only to be
// discarded still compiles. Gathering a rest to name everything but one property and then
// dropping it is ordinary, and a `:=` nothing reads is a Go error, so the gather takes the
// same blank an unread leaf takes.
func TestNestedObjectRestNothingReadsTakesTheBlank(t *testing.T) {
	const src = "const deep = { o: { a: 1, b: 2 } };\n" +
		"function f(): number { const { o: { a, ...unread } } = deep; return a; }\n" +
		"console.log(f());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "_ = unread") {
		t.Errorf("the unread nested rest took no blank:\n%s", got)
	}
}

// TestNestedArrayRestStillGathersItsTail pins the boundary that was already held. The
// array sibling of this arm has been in bindSubArray all along, so a nested array rest
// still copies the tail with Slice rather than routing through the object gather.
func TestNestedArrayRestStillGathersItsTail(t *testing.T) {
	const src = "const deep = { o: { xs: [1, 2, 3] } };\n" +
		"const { o: { xs: [head, ...tail] } } = deep;\n" +
		"console.log(head, tail.length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "tail := ") || !strings.Contains(got, ".Slice(1)") {
		t.Errorf("the nested array rest did not copy its tail:\n%s", got)
	}
}
