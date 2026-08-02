package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestBoxedArraySpreadDrainsAtRunTime pins the choice this slice makes. A binding the
// checker still calls a number[] can hold a box, and the splice the array paths pick is
// chosen off that checker type, so it used to read an Elems field a value.Value has not
// got. What the expression lowers to wins: a boxed operand is drained by the same
// value.Iterate a for...of over it drives, and each drained box comes down to the element
// type the target names, once, at the splice.
func TestBoxedArraySpreadDrainsAtRunTime(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want string
	}{
		{
			"number",
			"const ns = JSON.parse('[]') as number[];\nconst c = [...ns];\nconsole.log(c.length);\n",
			"value.ToNumber(",
		},
		{
			"string",
			"const ss = JSON.parse('[]') as string[];\nconst c = [...ss];\nconsole.log(c.length);\n",
			"value.ToString(",
		},
		{
			"boolean",
			"const bs = JSON.parse('[]') as boolean[];\nconst c = [...bs];\nconsole.log(c.length);\n",
			"value.ToBoolean(",
		},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			got := renderProgram(t, c.src)
			if !strings.Contains(got, "value.IterateToSlice(") {
				t.Errorf("the spread of a boxed array was not drained at run time:\n%s", got)
			}
			if !strings.Contains(got, c.want) {
				t.Errorf("the drained elements did not come down to the element type, want %s:\n%s", c.want, got)
			}
			if strings.Contains(got, ".Elems") {
				t.Errorf("the spread still read Elems off a box:\n%s", got)
			}
		})
	}
}

// TestBoxedArraySpreadCoercesThroughOnePass pins the shape of that drain: the run-time
// slice is read once into a temp, a slice of the target's element type is made at its
// length, and one range fills it. Doing the conversion once at the splice is what leaves
// the copy an ordinary Go slice, so no later reader of it has to know it came from a box.
func TestBoxedArraySpreadCoercesThroughOnePass(t *testing.T) {
	const src = "const ns = JSON.parse('[]') as number[];\nconst c = [...ns];\nconsole.log(c.length);\n"
	got := renderProgram(t, src)
	for _, want := range []string{
		"_bt0 := value.IterateToSlice(ns, \"ns\")",
		"_bt1 := make([]float64, len(_bt0))",
		"for _bt2, _bt3 := range _bt0 {",
		"_bt1[_bt2] = value.ToNumber(_bt3)",
		"return _bt1",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestBoxedArraySpreadOfShapesBoxesTheLiteral pins the other half of the rule. A shape
// element type has no Go value to bring a box down to, since there is nothing to put in a
// struct that is the same object the box already is. So the literal around the spread
// gives way instead and becomes a boxed array, which is the answer a spread of a boxed
// collection already takes. The drained boxes are then value.Value themselves, so they
// splice with nothing to convert.
func TestBoxedArraySpreadOfShapesBoxesTheLiteral(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const rows = JSON.parse('[]') as Row[];\n" +
		"const c = [...rows];\n" +
		"console.log(c.length);\n"
	got := renderProgram(t, src)
	for _, want := range []string{
		"var c value.Value = value.NewArrayValue(",
		"value.IterateToSlice(rows, \"rows\")...",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestBoxedArraySpreadSplicesBesideOtherElements pins that the drain is one member of the
// same append the ordinary spread path builds, so a boxed operand mixes with plain
// elements in source order rather than needing a literal of its own.
func TestBoxedArraySpreadSplicesBesideOtherElements(t *testing.T) {
	const src = "const ns = JSON.parse('[]') as number[];\n" +
		"const c = [0, ...ns, 9];\n" +
		"console.log(c.length);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "value.IterateToSlice(") {
		t.Errorf("the boxed operand was not drained beside the plain elements:\n%s", got)
	}
	for _, want := range []string{"append([]float64{0}, func() []float64 {", "}()...), 9)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing the plain element %q in:\n%s", want, got)
		}
	}
}

// TestBoxedArraySpreadIntoATypedRest pins that a call argument takes the same rule as a
// literal. f(...ns) gathers into the rest parameter's own slice, so the drained boxes have
// to come down to that parameter's element type exactly as they do at a literal, and the
// callee sees the ordinary Go slice its signature declares.
func TestBoxedArraySpreadIntoATypedRest(t *testing.T) {
	const src = "const ns = JSON.parse('[]') as number[];\n" +
		"function sum(...xs: number[]): number { return xs.length; }\n" +
		"console.log(sum(...ns));\n"
	got := renderProgram(t, src)
	for _, want := range []string{
		"value.IterateToSlice(ns, \"ns\")",
		"make([]float64, len(",
		"value.ToNumber(",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

// TestBoxedArraySpreadHandsBack pins the boundary the rule leaves. A rest parameter of a
// class type is neither a slot of boxes nor a primitive to come down to, and unlike an
// array literal a parameter's slot cannot give way and become boxed, since its signature
// is what the callee was compiled against. So the call hands back with a named reason.
func TestBoxedArraySpreadHandsBack(t *testing.T) {
	const src = "class Row { id = 0; }\n" +
		"const rows = JSON.parse('[]') as Row[];\n" +
		"function count(...xs: Row[]): number { return xs.length; }\n" +
		"console.log(count(...rows));\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("want a NotYetLowerable, got %v", err)
	}
	if !strings.Contains(nyl.Reason, "spread of a boxed value into this element type") {
		t.Fatalf("want the element-type reason, got %q", nyl.Reason)
	}
}
