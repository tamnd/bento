package lower

import (
	"strings"
	"testing"
)

// An object destructuring binding, const {x, y} = src, binds each shorthand name to
// the property of the same name. Go has no destructuring, so it lowers to one short
// declaration per property reading through the struct-field selector, the same read a
// written-out property access lowers to. The source must be a plain variable of a
// fixed-shape object, and the pattern must be shorthand names only.

// TestObjectDestructureBindsProperties proves each shorthand name lowers to the
// struct-field selector off the source variable.
func TestObjectDestructureBindsProperties(t *testing.T) {
	const src = "const pt = { x: 10, y: 20 };\nconst { x, y } = pt;\nconsole.log(x + y);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "x := pt.X") {
		t.Errorf("property x did not lower to pt.X:\n%s", source)
	}
	if !strings.Contains(source, "y := pt.Y") {
		t.Errorf("property y did not lower to pt.Y:\n%s", source)
	}
}

// TestObjectDestructureRuns builds and runs a mixed-type destructure so the field
// reads are proven to pick the right properties across types.
func TestObjectDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const rec = { label: "sam", age: 30, active: true };
const { label, age, active } = rec;
console.log(label);
console.log(age);
console.log(active);
`
	if got, want := runProgramGo(t, src), "sam\n30\ntrue\n"; got != want {
		t.Fatalf("object destructure printed %q, want %q", got, want)
	}
}

// TestObjectDestructureSingle proves a one-property pattern binds that property.
func TestObjectDestructureSingle(t *testing.T) {
	skipIfShort(t)
	const src = `
const box = { width: 42, height: 7 };
const { width } = box;
console.log(width);
`
	if got, want := runProgramGo(t, src), "42\n"; got != want {
		t.Fatalf("single object destructure printed %q, want %q", got, want)
	}
}

// TestObjectDestructureRenameHandsBack proves a renamed property hands back, since
// binding a property to a different local name is a later slice.
func TestObjectDestructureRenameHandsBack(t *testing.T) {
	const src = "const pt = { x: 1, y: 2 };\nconst { x: a, y: b } = pt;\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureDefaultHandsBack proves a property default hands back, since
// filling a missing property with the default is a later slice.
func TestObjectDestructureDefaultHandsBack(t *testing.T) {
	const src = "const pt: { x: number; y?: number } = { x: 1 };\nconst { x, y = 5 } = pt;\nconsole.log(x + y);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureRestHandsBack proves a rest property hands back, since
// gathering the remaining properties into an object is a later slice.
func TestObjectDestructureRestHandsBack(t *testing.T) {
	const src = "const pt = { x: 1, y: 2, z: 3 };\nconst { x, ...rest } = pt;\nconsole.log(x);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureCallSourceLowersToTemp proves a non-variable object source, a
// call returning a fixed-shape object, lowers by holding the source in a generated
// temporary read once, then reading each property off that temporary, so the source is
// evaluated once.
func TestObjectDestructureCallSourceLowersToTemp(t *testing.T) {
	const src = "function pt(): { x: number; y: number } { return { x: 10, y: 20 }; }\nconst { x, y } = pt();\nconsole.log(x + y);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ":= Pt()") {
		t.Errorf("call source was not held in a temporary:\n%s", source)
	}
	if !strings.Contains(source, "x := ") || !strings.Contains(source, ".X") {
		t.Errorf("property x did not read off the temporary:\n%s", source)
	}
	if !strings.Contains(source, "y := ") || !strings.Contains(source, ".Y") {
		t.Errorf("property y did not read off the temporary:\n%s", source)
	}
}

// TestObjectDestructureCallSourceRuns builds and runs a call-source destructure so the
// evaluate-once temporary is proven to feed the property reads.
func TestObjectDestructureCallSourceRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function pt(): { x: number; y: number } { return { x: 10, y: 20 }; }
const { x, y } = pt();
console.log(x);
console.log(y);
`
	if got, want := runProgramGo(t, src), "10\n20\n"; got != want {
		t.Fatalf("call-source object destructure printed %q, want %q", got, want)
	}
}
