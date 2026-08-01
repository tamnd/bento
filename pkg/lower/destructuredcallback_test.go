package lower

import (
	"strings"
	"testing"
)

// TestADestructuredParameterOverABoxReadsAtRunTime pins the routing. The callback is
// handed a box, so the pattern reads its leaves through the runtime rather than
// selecting the Go fields the checker's type names.
func TestADestructuredParameterOverABoxReadsAtRunTime(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"console.log(Object.values(m).map(({ id }: Row) => id));"
	out := renderProgram(t, src)
	if !strings.Contains(out, `Get(value.FromGoString("id"))`) {
		t.Fatalf("a destructured parameter over a box did not read at run time:\n%s", out)
	}
}

// TestADestructuredParameterOverAnArrayKeepsItsFields is the boundary in the other
// direction. A receiver that really is a Go slice of structs hands the callback a
// struct, so nothing moves and the bind stays a field select.
func TestADestructuredParameterOverAnArrayKeepsItsFields(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const rows: Row[] = [{ id: 1, tag: 'a' }];\n" +
		"console.log(rows.map(({ id }: Row) => id));"
	out := renderProgram(t, src)
	if strings.Contains(out, `Get(value.FromGoString("id"))`) {
		t.Fatalf("a destructured parameter over an array read at run time:\n%s", out)
	}
}

// TestADestructuredLeafComesDownToItsPrimitive pins the coercion. A leaf the checker
// types string holds no box, because the runtime's Get on a boxed string answers only
// length and the indices, so a method call on one would have found undefined and called
// it. This is a build-then-throw rather than a hand-back, which is why the bind asks the
// same question a read off a box asks.
func TestADestructuredLeafComesDownToItsPrimitive(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"console.log(Object.values(m).map(({ tag }: Row) => tag.toUpperCase()));"
	out := renderProgram(t, src)
	if strings.Contains(out, `FromGoString("toUpperCase")`) {
		t.Fatalf("a string leaf kept its box and dispatched its method at run time:\n%s", out)
	}
	if !strings.Contains(out, "value.ToString(") {
		t.Fatalf("a string leaf did not come down to its primitive:\n%s", out)
	}
}

// TestADeclarationDestructuringABoxReadsAtRunTime is the same binder reached from a
// plain const. Its gate asked what the checker calls the source, which for
// Object.values(m)[0] is a Row, so it selected the Go field off a value.Value.
func TestADeclarationDestructuringABoxReadsAtRunTime(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"const { id } = Object.values(m)[0];\n" +
		"console.log(id);"
	out := renderProgram(t, src)
	if !strings.Contains(out, `Get(value.FromGoString("id"))`) {
		t.Fatalf("a declaration destructuring a box did not read at run time:\n%s", out)
	}
}
