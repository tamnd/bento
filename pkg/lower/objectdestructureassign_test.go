package lower

import (
	"strings"
	"testing"
)

// An object destructuring assignment, ({ x, y } = o), assigns each already-declared
// local from the property of the same name. It is parenthesized in source since a bare
// { on the left would open a block. It lowers to a single Go parallel assignment,
// x, y = o.X, o.Y, the same struct-field selector a written-out property access lowers
// to, and the parallel form matches the assignment's evaluate-then-store order.

// TestObjectDestructureAssignBindsProperties proves each shorthand target reads the
// property of the same name off the source variable in one parallel assignment.
func TestObjectDestructureAssignBindsProperties(t *testing.T) {
	const src = "const o = { x: 1, y: 2 };\nlet x = 0;\nlet y = 0;\n({ x, y } = o);\nconsole.log(x + y);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x, y = o.X, o.Y") {
		t.Errorf("object assignment did not lower to a parallel field read:\n%s", source)
	}
}

// TestObjectDestructureAssignRuns builds and runs a shorthand assignment so the field
// reads are proven to pick the right properties.
func TestObjectDestructureAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const o = { x: 10, y: 20, z: 30 };
let x = 0;
let y = 0;
let z = 0;
({ x, y, z } = o);
console.log(x);
console.log(y);
console.log(z);
`
	if got, want := runProgramGo(t, src), "10\n20\n30\n"; got != want {
		t.Fatalf("object destructure assignment printed %q, want %q", got, want)
	}
}

// TestObjectDestructureAssignMixedTypes proves the field reads carry each property's
// type across a mixed-type object.
func TestObjectDestructureAssignMixedTypes(t *testing.T) {
	skipIfShort(t)
	const src = `
const rec = { label: "sam", age: 30, active: true };
let label = "";
let age = 0;
let active = false;
({ label, age, active } = rec);
console.log(label);
console.log(age);
console.log(active);
`
	if got, want := runProgramGo(t, src), "sam\n30\ntrue\n"; got != want {
		t.Fatalf("mixed-type object assignment printed %q, want %q", got, want)
	}
}

// TestObjectDestructureAssignRenameHandsBack proves a renamed property hands back,
// since binding a property to a different local name is a later slice.
func TestObjectDestructureAssignRenameHandsBack(t *testing.T) {
	const src = "const o = { x: 1, y: 2 };\nlet a = 0;\nlet b = 0;\n({ x: a, y: b } = o);\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureAssignDefaultRuns proves a property default lowers in an
// assignment: the missing optional property takes the default while the present one
// keeps its value.
func TestObjectDestructureAssignDefaultRuns(t *testing.T) {
	const src = "const o: { x: number; y?: number } = { x: 1 };\nlet x = 0;\nlet y = 0;\n({ x, y = 5 } = o);\nconsole.log(x + y);\n"
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("object default assignment printed %q, want %q", got, want)
	}
}

// TestObjectDestructureAssignRestHandsBack proves a rest property hands back, since
// gathering the remaining properties into an object is a later slice.
func TestObjectDestructureAssignRestHandsBack(t *testing.T) {
	const src = "const o = { x: 1, y: 2, z: 3 };\nlet x = 0;\nlet rest = { y: 0, z: 0 };\n({ x, ...rest } = o);\nconsole.log(x);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureAssignCallSourceHandsBack proves a call source hands back, since
// reading each property off the result needs a temporary to hold it.
func TestObjectDestructureAssignCallSourceHandsBack(t *testing.T) {
	const src = "function pt(): { x: number; y: number } { return { x: 1, y: 2 }; }\nlet x = 0;\nlet y = 0;\n({ x, y } = pt());\nconsole.log(x + y);\n"
	renderProgramHandBack(t, src)
}
