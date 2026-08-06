package lower

import (
	"strings"
	"testing"
)

// TestAShapeTypedCallbackParameterTakesABoxedField pins the parameter slot. The wrapper
// around a boxed callback hands its arguments over already boxed, so a parameter the box
// cannot land in has to hold the box itself.
func TestAShapeTypedCallbackParameterTakesABoxedField(t *testing.T) {
	const src = "type Row = { id: number };\n" +
		"const rows = JSON.parse('[]') as Row[];\n" +
		"console.log(rows.map((r: Row) => r.id));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func(r value.Value)") {
		t.Fatalf("a shape-typed callback parameter did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, "r.Get(value.FromGoString(\"id\"))") {
		t.Fatalf("a read off the parameter did not dispatch dynamically:\n%s", out)
	}
}

// TestANestedCallbackKeepsTheEnclosingBoxedNames pins the capture. The set of boxed
// names was rebuilt from the inner function's own parameters, so the inner read of rows
// lowered as the static Filter on what is a value.Value, which is Go that does not
// compile rather than a hand-back.
func TestANestedCallbackKeepsTheEnclosingBoxedNames(t *testing.T) {
	const src = "type Row = { id: number };\n" +
		"const rows = JSON.parse('[]') as Row[];\n" +
		"console.log(rows.map((r: Row) => rows.filter((q: Row) => q.id >= r.id).length));"
	out := renderProgram(t, src)
	if strings.Contains(out, "rows.Filter(") {
		t.Fatalf("a nested callback lost the enclosing boxed name:\n%s", out)
	}
	if !strings.Contains(out, "value.CallMethod(rows, value.FromGoString(\"filter\")") {
		t.Fatalf("a nested read off a captured box did not dispatch dynamically:\n%s", out)
	}
}

// TestAPrimitiveLinkEndsTheBoxedChain pins where a chain stops being a box. r.tag is a
// read off a box, but the checker types it string and the read coerces down to a
// value.BStr at the read itself, so the method on it is the static string one.
func TestAPrimitiveLinkEndsTheBoxedChain(t *testing.T) {
	const src = "type Row = { tag: string };\n" +
		"const rows = JSON.parse('[]') as Row[];\n" +
		"console.log(rows.map((r: Row) => r.tag.toUpperCase()));"
	out := renderProgram(t, src)
	if !strings.Contains(out, ".ToUpperCase()") {
		t.Fatalf("a string read off a box did not take the static string method:\n%s", out)
	}
}

// TestAPrimitiveCallbackParameterKeepsItsStaticSlot is the boundary in the other
// direction. A number parameter takes the box through ToNumber, so it stays static and
// its lowering does not move.
func TestAPrimitiveCallbackParameterKeepsItsStaticSlot(t *testing.T) {
	const src = "const xs = JSON.parse('[]') as number[];\n" +
		"console.log(xs.map((x: number) => x * 2));"
	out := renderProgram(t, src)
	if !strings.Contains(out, "func(x float64)") {
		t.Fatalf("a number callback parameter did not keep its static slot:\n%s", out)
	}
}

// TestABoxedCallbackResultBoxesByType pins the return side. Only the primitives had a
// box picked by type flags alone; a shape and an array each have one that needs the
// type to name it.
func TestABoxedCallbackResultBoxesByType(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"shape": {"console.log((JSON.parse('[]') as number[]).map((n: number) => ({ v: n })));", "value.ObjectFromStruct("},
		"array": {"console.log((JSON.parse('[]') as number[]).flatMap((n: number) => [n, n]));", "value.ArrayValueOf("},
	} {
		t.Run(name, func(t *testing.T) {
			out := renderProgram(t, tc.src)
			if !strings.Contains(out, tc.want) {
				t.Fatalf("a boxed callback's result did not box through %s:\n%s", tc.want, out)
			}
		})
	}
}
