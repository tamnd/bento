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

// TestObjectDestructureRenameLowers proves a renamed property reads the source
// property of its own name and binds it to the renamed target: {x: a} reads o.X into
// a, so the emitted Go selects the source field and defines the renamed local.
func TestObjectDestructureRenameLowers(t *testing.T) {
	const src = "const pt = { x: 1, y: 2 };\nconst { x: a, y: b } = pt;\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a := pt.X") || !strings.Contains(source, "b := pt.Y") {
		t.Errorf("object rename did not read the source field into the renamed target:\n%s", source)
	}
}

// TestObjectDestructureRenameRuns builds and runs a renamed destructuring so the
// renamed locals are proven to carry the right source properties.
func TestObjectDestructureRenameRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const pt = { x: 1, y: 2 };
const { x: a, y: b } = pt;
console.log(a);
console.log(b);
`
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("object rename destructure printed %q, want %q", got, want)
	}
}

// TestObjectDestructureDefaultRuns proves a property default lowers: the missing
// optional property takes the default while the present one keeps its value.
func TestObjectDestructureDefaultRuns(t *testing.T) {
	const src = "const pt: { x: number; y?: number } = { x: 1 };\nconst { x, y = 5 } = pt;\nconsole.log(x + y);\n"
	if got, want := runProgramGo(t, src), "6\n"; got != want {
		t.Fatalf("object default destructure printed %q, want %q", got, want)
	}
}

// TestObjectDestructureRenameDefaultRuns proves a renamed target carrying a default
// applies the rename to the target and the default to the undefined case together: the
// present property feeds the renamed local, and the missing optional property takes the
// default under the renamed name.
func TestObjectDestructureRenameDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const o: { x: number; y?: number } = { x: 1 };
const { x: a, y: b = 9 } = o;
console.log(a);
console.log(b);
`
	if got, want := runProgramGo(t, src), "1\n9\n"; got != want {
		t.Fatalf("object rename-default destructure printed %q, want %q", got, want)
	}
}

// TestObjectDestructureComputedKeyHandsBack proves a computed key hands back, since
// reading the source by a key computed at run time needs the dynamic object model of
// phase 7 rather than a static field selector.
func TestObjectDestructureComputedKeyHandsBack(t *testing.T) {
	const src = "const k = \"x\";\nconst o = { x: 1 };\nconst { [k]: v } = o;\nconsole.log(v);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureComputedKeySideEffectHandsBack proves a computed key whose
// expression has a side effect hands back rather than emit a partial read: getting the
// evaluate-exactly-once order right against the other elements needs the dynamic object
// model of phase 7, so the decline is honest rather than a half-evaluated key.
func TestObjectDestructureComputedKeySideEffectHandsBack(t *testing.T) {
	const src = "let count = 0;\nconst bump = (): \"x\" => { count++; return \"x\"; };\nconst o = { x: 1 };\nconst { [bump()]: v } = o;\nconsole.log(v);\n"
	renderProgramHandBack(t, src)
}

// TestObjectDestructureRestHandsBack proves a rest property hands back, since
// gathering the remaining properties into an object needs the object model of phase 7.
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
