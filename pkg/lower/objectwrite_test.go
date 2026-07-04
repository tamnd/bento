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
