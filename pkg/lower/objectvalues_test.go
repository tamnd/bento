package lower

import (
	"strings"
	"testing"
)

// TestObjectValuesEmitsArray pins that Object.values on a homogeneous fixed-shape
// object folds to a value.NewArray of the field reads in declaration order.
func TestObjectValuesEmitsArray(t *testing.T) {
	src := "const o = { a: 1, b: 2, c: 3 };\nconsole.log(Object.values(o).join(\",\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.NewArray[float64](o.A, o.B, o.C)") {
		t.Errorf("Object.values did not fold to a NewArray of field reads:\n%s", source)
	}
}

// TestObjectValuesMixedTypesHandsBack pins that a shape whose field types differ,
// which would need a mixed-element array, is a later slice.
func TestObjectValuesMixedTypesHandsBack(t *testing.T) {
	src := "const o = { a: 1, b: \"x\" };\nconsole.log(Object.values(o).length);\n"
	renderProgramHandBack(t, src)
}

// TestObjectValuesRuns builds and runs Object.values against the Node oracle for
// a number shape and a string shape.
func TestObjectValuesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const nums = { a: 10, b: 20, c: 30 };
console.log(Object.values(nums).join(","));
const words = { first: "hello", second: "world" };
console.log(Object.values(words).join(" "));
`
	got := runProgramGo(t, src)
	want := "10,20,30\nhello world\n"
	if got != want {
		t.Fatalf("Object.values program printed %q, want %q", got, want)
	}
}
