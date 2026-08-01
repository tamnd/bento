package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestABoxedChainBoundToANameTakesABoxedSlot pins the slot. The checker types the
// binding by the shape it projects onto the walk's element, so without this the
// declaration asked typeExpr for the Go struct that shape interns to, which the box
// cannot fill.
func TestABoxedChainBoundToANameTakesABoxedSlot(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"const first = Object.values(m)[0];\n" +
		"console.log(first.id);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "var first value.Value") {
		t.Fatalf("a boxed chain bound to a name did not take a boxed slot:\n%s", out)
	}
	if !strings.Contains(out, `first.Get(value.FromGoString("id"))`) {
		t.Fatalf("a read off the binding did not dispatch at run time:\n%s", out)
	}
}

// TestAPrimitiveBoundOffABoxKeepsItsGoValue is the boundary in the other direction. A
// binding the checker types number, string, or boolean has one Go value to come down
// to, and the ordinary coercion runs on the way in, so the name really does hold a
// float64 and every consumer of it is right about that.
func TestAPrimitiveBoundOffABoxKeepsItsGoValue(t *testing.T) {
	const src = "const nums = JSON.parse('{}') as Record<string, number>;\n" +
		"const n = Object.values(nums).reduce((a: number, b: number) => a + b, 0);\n" +
		"console.log(n + 1);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.ToNumber(") {
		t.Fatalf("a primitive bound off a box did not come down to its Go value:\n%s", out)
	}
	if strings.Contains(out, "var n value.Value") {
		t.Fatalf("a primitive bound off a box took a boxed slot:\n%s", out)
	}
}

// TestALiteralHoldingABoxBoxesWhole pins the literal. The checker interns a Go struct
// for { first: Row } and the box has no fields to fill it, so the literal is built over
// the value model instead and the box is stored as itself.
func TestALiteralHoldingABoxBoxesWhole(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"const o = { first: Object.values(m)[0] };\n" +
		"console.log(o.first.id);"
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewObject()") {
		t.Fatalf("a literal holding a box did not box whole:\n%s", out)
	}
}

// TestASpreadOfABoxAssigns pins the spread. Its answer is the source's own enumerable
// properties copied onto what the literal has built so far, which is what Assign does.
func TestASpreadOfABoxAssigns(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"const first = Object.values(m)[0];\n" +
		"const o = { ...first, extra: 3 };\n" +
		"console.log(o.extra);"
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Assign(first)") {
		t.Fatalf("a spread of a box did not assign its properties:\n%s", out)
	}
}

// TestAnOptionalLinkOffABoxStaysBoxed pins the one read that looks like it comes down
// and does not. OptionalMember answers a box whatever the checker calls the result,
// since the undefined a short circuit produces has to stay tellable from the property's
// own value, so the link after it dispatches rather than mapping an optional.
func TestAnOptionalLinkOffABoxStaysBoxed(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n" +
		"const first = Object.values(m)[0];\n" +
		"console.log(first?.tag?.length);"
	out := renderProgram(t, src)
	if strings.Count(out, "value.OptionalMember(") != 2 {
		t.Fatalf("an optional link off a box did not stay boxed down the chain:\n%s", out)
	}
}

// TestABoxPassedToADeclaredSignatureHandsBack pins the boundary this stops at. Filling a
// declared parameter or return from a box could only copy, so the honest answer until
// such a signature can take a boxed slot is a hand-back rather than the Go that does not
// compile it used to emit.
func TestABoxPassedToADeclaredSignatureHandsBack(t *testing.T) {
	const prelude = "type Row = { id: number; tag: string };\n" +
		"const m = JSON.parse('{}') as Record<string, Row>;\n"
	for name, src := range map[string]string{
		"argument": prelude + "function f(r: Row): number { return r.id; }\nconsole.log(f(Object.values(m)[0]));",
		"return":   prelude + "function g(): Row { return Object.values(m)[0]; }\nconsole.log(g().tag);",
	} {
		t.Run(name, func(t *testing.T) {
			prog := compile(t, src)
			r := NewRenderer(prog)
			_, err := r.RenderProgram(entryFile(t, prog))
			var nyl *NotYetLowerable
			if !errors.As(err, &nyl) {
				t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
			}
			if !strings.Contains(nyl.Reason, "coercing a dynamic value into this static type") {
				t.Errorf("hand-back reason = %q, want it to name the dynamic coercion", nyl.Reason)
			}
		})
	}
}
