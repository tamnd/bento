package lower

import (
	"strings"
	"testing"
)

// TestObjectFieldWriteEmits pins that o.k = v on a plain object lowers to the Go
// struct field assignment, the store half of the o.k read.
func TestObjectFieldWriteEmits(t *testing.T) {
	const src = "export function bump(o: { x: number }): void { o.x = 5; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "o.X = 5") {
		t.Errorf("object field write did not lower to a struct field assignment:\n%s", source)
	}
}

// TestObjectFieldWriteCoercesValue pins that a string write into a string field
// still routes through the field store, so the value reaches the field type rather
// than handing back.
func TestObjectFieldWriteCoercesValue(t *testing.T) {
	const src = "export function set(o: { name: string }): void { o.name = \"hi\"; }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "o.Name = ") {
		t.Errorf("string field write did not lower to a struct field assignment:\n%s", source)
	}
}

// TestObjectFieldWriteThroughParameterMutates proves the write goes through the
// pointer a plain object lowers to, so a function that writes a caller's object
// field builds and runs against the Node oracle.
func TestObjectFieldWriteThroughParameterMutates(t *testing.T) {
	skipIfShort(t)
	const src = `
function setX(o: { x: number }): void {
  o.x = 42;
}
function run(): void {
  const o = { x: 1 };
  setX(o);
  console.log(o.x);
}
run();
`
	runProgramGo(t, src)
}

// TestObjectCompoundFieldWriteMutatesThroughParameter proves a compound object
// field write o.k += v on a fixed-shape parameter lowers to the Go compound
// assignment on the field selector and mutates through the object's pointer, so a
// caller sees the updated field.
func TestObjectCompoundFieldWriteMutatesThroughParameter(t *testing.T) {
	skipIfShort(t)
	const src = `function add(o: { x: number }, v: number): void { o.x += v; }
const o = { x: 10 };
add(o, 5);
console.log(String(o.x));
`
	if got, want := runProgramGo(t, src), "15\n"; got != want {
		t.Fatalf("compound field write through a parameter printed %q, want %q", got, want)
	}
}

// TestObjectUndeclaredFieldWriteBoxes proves a write to a property the fixed shape
// never declared boxes the binding to a dynamic bag from its literal rather than
// emitting an assignment to the value.MissingProperty read fallback, which is not
// addressable and would fail the go build. The binding grows a key the checker did
// not fold into its type, the JavaScript expando, so it lives as a value.Object: the
// literal builds via NewObject, the undeclared write lands through Set, and both the
// declared and the grown property read back through Get. The write draws the 2339
// "property does not exist" diagnostic the AOT front door tolerates, so the test
// reaches the renderer through the same tolerant path build.Compile uses.
func TestObjectUndeclaredFieldWriteBoxes(t *testing.T) {
	const src = "const o = { x: 1 };\no.y = 5;\n"
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, `o.Set(value.FromGoString("y"), value.Number(5))`) {
		t.Fatalf("undeclared field write did not land through Set on the box:\n%s", source)
	}
}

// TestObjectUndeclaredFieldWriteRuns proves the boxed binding runs against the Node
// oracle: the grown key reads back the value written and the declared key survives the
// growth, so const o = { x: 1 }; o.y = 5 prints the new and the old property in order.
func TestObjectUndeclaredFieldWriteRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o = { x: 1 };\no.y = 5;\nconsole.log(o.y);\nconsole.log(o.x);\n"
	if got, want := runProgramGoTolerant(t, src), "5\n1\n"; got != want {
		t.Fatalf("undeclared field write run = %q, want %q", got, want)
	}
}

// TestObjectFieldWriteToEmptyShapeEmits pins the case the test262 compareArray harness
// hit: an empty-shape object o = {} boxes into a dynamic value.Object, so a write to any
// property lands through Set on the box rather than a non-addressable struct field, now
// that the empty object top type boxes.
func TestObjectFieldWriteToEmptyShapeEmits(t *testing.T) {
	const src = "const o = {};\no.prop = 42;\n"
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, `o.Set(value.FromGoString("prop"), value.Number(42))`) {
		t.Fatalf("empty-shape object write did not land through Set on the box:\n%s", source)
	}
}
