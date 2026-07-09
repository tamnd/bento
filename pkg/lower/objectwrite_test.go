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

// TestObjectCompoundFieldWriteHandsBack proves a compound object field write
// o.k += v hands back rather than dropping the read half the compound needs.
func TestObjectCompoundFieldWriteHandsBack(t *testing.T) {
	const src = "export function add(o: { x: number }, v: number): void { o.x += v; }\n"
	reason := renderProgramHandBack(t, src)
	if reason == "" {
		t.Fatal("expected a compound object field write to hand back")
	}
}

// TestObjectUndeclaredFieldWriteHandsBack proves a write to a property the fixed
// shape never declared hands back rather than emitting an assignment to the
// value.MissingProperty read fallback, which is not addressable and would fail
// the go build. The shape interns to a struct with only its declared fields, so
// there is no lvalue for a property it never declared; adding one is a runtime
// shape mutation this path does not model. The write draws the 2339 "property
// does not exist" diagnostic the AOT front door tolerates, so the test reaches
// the renderer through the same tolerant path build.Compile uses.
func TestObjectUndeclaredFieldWriteHandsBack(t *testing.T) {
	const src = "const o = { x: 1 };\no.y = 5;\n"
	reason := renderProgramTolerantHandBack(t, src)
	if reason == "" {
		t.Fatal("expected a write to an undeclared property to hand back")
	}
}

// TestObjectFieldWriteToEmptyShapeHandsBack pins the case the test262 compareArray
// harness hit: a write to any property of an empty-shape object o = {} has no
// declared field to land in, so it hands back instead of emitting the
// non-addressable value.MissingProperty(o) = v.
func TestObjectFieldWriteToEmptyShapeHandsBack(t *testing.T) {
	const src = "const o = {};\no.prop = 42;\n"
	reason := renderProgramTolerantHandBack(t, src)
	if reason == "" {
		t.Fatal("expected a write to a property of an empty-shape object to hand back")
	}
}
