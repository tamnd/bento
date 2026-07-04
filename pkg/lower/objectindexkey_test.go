package lower

import (
	"strings"
	"testing"
)

// TestObjectIndexKeyEmitsField pins that o["k"] with a string-literal key on a
// fixed-shape object lowers to the exported struct field selector, the same field
// the dotted read o.k selects, rather than the dynamic Get the any/unknown path
// takes.
func TestObjectIndexKeyEmitsField(t *testing.T) {
	const src = `const o = { a: 1, name: "hi" };
console.log(o["name"]);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Name") {
		t.Errorf("o[\"name\"] did not lower to the .Name struct field:\n%s", source)
	}
	if strings.Contains(source, "Get(") {
		t.Errorf("o[\"name\"] took the dynamic Get path, want a static field selector:\n%s", source)
	}
}

// TestObjectIndexKeyDynamicHandsBack pins that a computed key that is not a string
// literal, a variable index, still hands back: there is no static field to select,
// so it is its own later slice rather than a wrong field guess.
func TestObjectIndexKeyDynamicHandsBack(t *testing.T) {
	const src = `const o = { a: 1 };
const k = "a";
console.log(o[k]);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "non-array receiver") {
		t.Errorf("hand-back reason = %q, want it to name the non-array element access", reason)
	}
}

// TestObjectIndexKeyRuns builds and runs the emitted Go against the Node oracle:
// a string-literal index reads the same field the dotted access reads, for both a
// number-typed and a string-typed field, and a bracket read assigned to a binding
// carries the field value.
func TestObjectIndexKeyRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const o = { a: 1, name: "hi" };
console.log(o["a"]);
console.log(o["name"]);
const bracket = o["a"];
console.log(bracket);
`
	got := runProgramGo(t, src)
	const want = "1\nhi\n1\n"
	if got != want {
		t.Errorf("object index key program printed %q, want %q", got, want)
	}
}
