package lower

import (
	"strings"
	"testing"
)

// TestObjectEntriesEmitsPairArray pins that Object.entries on a homogeneous
// fixed-shape object folds to a value.NewArray of interned [name, value] pair
// tuples, each pairing the field-name literal with the field read in declaration
// order.
func TestObjectEntriesEmitsPairArray(t *testing.T) {
	src := "const o = { a: 1, b: 2, c: 3 };\nconsole.log(Object.entries(o).length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[Tuple_str_num](Tuple_str_num{E0: value.FromGoString(\"a\"), E1: o.A}") {
		t.Errorf("Object.entries did not fold to a NewArray of pair tuples:\n%s", source)
	}
}

// TestObjectEntriesMixedTypesHandsBack pins that a shape whose field types differ,
// whose pair value slot would be a union, is a later slice.
func TestObjectEntriesMixedTypesHandsBack(t *testing.T) {
	src := "const o = { a: 1, b: \"x\" };\nconsole.log(Object.entries(o).length);\n"
	renderProgramHandBack(t, src)
}

// TestObjectEntriesRuns builds and runs Object.entries against the Node oracle,
// covering a for-of destructure over the pairs, a stored result read by index, and
// a string-valued shape.
func TestObjectEntriesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const nums = { a: 1, b: 2, c: 3 };
for (const [k, v] of Object.entries(nums)) {
  console.log(k, v);
}
const e = Object.entries(nums);
console.log(e.length);
console.log(e[0][0], e[0][1]);
const words = { first: "hello", second: "world" };
for (const [k, v] of Object.entries(words)) {
  console.log(k, v);
}
`
	got := runProgramGo(t, src)
	want := "a 1\nb 2\nc 3\n3\na 1\nfirst hello\nsecond world\n"
	if got != want {
		t.Fatalf("Object.entries program printed %q, want %q", got, want)
	}
}
