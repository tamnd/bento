package lower

import (
	"strings"
	"testing"
)

// The cases here pin fresh miscompiles a whole-corpus reclassify surfaced: shapes
// that emitted Go the compiler rejects, or panicked in the front end, where the
// honest outcome is a clean lowering or a hand-back. Each one built wrong Go
// before the fix and must not again.

// TestForInNestedRedeclareNoUnusedBinding proves a for...in whose body only
// re-declares the same var name, never reading the loop key, drops the binding to
// the blank identifier. A nested `for (var x in {})` inside a loop over x is not a
// read of the outer x, so the outer loop ranges without binding a Go x that Go
// would reject as declared and not used.
func TestForInNestedRedeclareNoUnusedBinding(t *testing.T) {
	const src = "for (var x in {}) {\n  for (var x in {}) {\n    break;\n  }\n}\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("nested for...in handed back, want a lowering: %v", err)
	}
	if strings.Contains(p.Source, "for _, x := range") {
		t.Fatalf("outer for...in bound an unused x, want a blank range:\n%s", p.Source)
	}
}

// TestComputedSymbolKeyHandsBack proves a computed key whose expression the checker
// could not resolve, `{ [Symbol.nonsense]: 0 }`, hands back rather than panic on a
// zero type handle while folding the key to a constant string.
func TestComputedSymbolKeyHandsBack(t *testing.T) {
	const src = "var o = {\n  [Symbol.nonsense]: 0,\n};\n"
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	defer func() {
		if p := recover(); p != nil {
			t.Fatalf("lowering a computed symbol key panicked, want a hand-back: %v", p)
		}
	}()
	if _, err := r.RenderProgram(entryFile(t, prog)); err == nil {
		t.Fatalf("computed symbol key lowered, want a hand-back:\n%s", src)
	}
}

// TestAssertedExcessPropertyDropped proves an object literal asserted to a shape
// narrower than the literal, `<{ id: number }>{ id: 4, name: "as" }`, builds at the
// asserted shape and leaves the excess member off the struct rather than emit a
// field the struct type does not carry.
func TestAssertedExcessPropertyDropped(t *testing.T) {
	const src = "var foo = <{ id: number }>{ id: 4, name: \"as\" };\n"
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	p, err := r.RenderProgram(entryFile(t, prog))
	if err != nil {
		t.Fatalf("asserted excess-property literal handed back, want a lowering: %v", err)
	}
	if strings.Contains(p.Source, "Name:") {
		t.Fatalf("asserted literal emitted the excess Name field:\n%s", p.Source)
	}
	if !strings.Contains(p.Source, "Id:") {
		t.Fatalf("asserted literal dropped the Id field it must keep:\n%s", p.Source)
	}
}

// TestObjectLiteralAssertedToDictionaryHandsBack proves an object literal asserted
// to a pure string-index dictionary, `({ "1": "one" } as { [k: string]: string })`,
// hands back rather than build an empty struct whose members have no field to land
// in and whose keyed read would find no method.
func TestObjectLiteralAssertedToDictionaryHandsBack(t *testing.T) {
	const src = "var f = (x: string) => ({ \"1\": \"one\", \"2\": \"two\" } as { [k: string]: string })[x];\n"
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("dictionary-asserted literal lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "later slice") {
		t.Fatalf("dictionary handback reason = %q, want a later-slice deferral", err.Error())
	}
}

// TestClassValueCrossAssignmentHandsBack proves assigning one class value to a slot
// typed for another class, `let a = Foo; a = Bar`, hands back rather than emit a
// cross-type Go assignment between the two distinct static-side struct types.
func TestClassValueCrossAssignmentHandsBack(t *testing.T) {
	const src = "class Foo { constructor(public x: number) {} }\n" +
		"class Bar { constructor(public x: number) {} }\n" +
		"let a = Foo;\n" +
		"a = Bar;\n"
	prog := compileTolerant(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("class-value cross assignment lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "later slice") {
		t.Fatalf("class-value handback reason = %q, want a later-slice deferral", err.Error())
	}
}
