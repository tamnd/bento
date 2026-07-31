package lower

import (
	"strings"
	"testing"
)

// A Map and a Set are typed collections in generated code, so every crossing into a
// dynamic slot has to build the box value.Value holds. These pin the emission: the
// crossing renders as a .ToValue() on the collection itself rather than a copy into
// some other shape, and a collection whose elements have no dynamic form hands back
// with a reason instead of emitting something that would not compile.

// TestBoxingAMapEmitsTheView pins that a Map assigned into an any binding renders as
// the view over the same map. A copy would make a later typed write invisible to the
// dynamic reader, and there is nothing else for the crossing to emit.
func TestBoxingAMapEmitsTheView(t *testing.T) {
	const src = `const m = new Map<number, number>();
m.set(1, 2);
const a: any = m;
console.log(a.size);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToValue()") {
		t.Errorf("a map crossing into an any binding did not box through ToValue:\n%s", source)
	}
}

// TestBoxingASetEmitsTheView is the Set half.
func TestBoxingASetEmitsTheView(t *testing.T) {
	const src = `const s = new Set<string>();
s.add("a");
const a: any = s;
console.log(a.size);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ToValue()") {
		t.Errorf("a set crossing into an any binding did not box through ToValue:\n%s", source)
	}
}

// TestBoxingAMapOfObjectKeysHandsBack pins the gate. The box bridges an entry into a
// dynamic value through dynBox, which covers a number, a string, a boolean and an
// already-dynamic value; an object key has no dynamic form yet, so the crossing hands
// back with a reason rather than emitting a box whose reads would throw.
func TestBoxingAMapOfObjectKeysHandsBack(t *testing.T) {
	const src = `const m = new Map<{ a: number }, number>();
const a: any = m;
console.log(a.size);
`
	err := renderError(t, src)
	if !strings.Contains(err, "keys or values") {
		t.Fatalf("handback reason = %q, want the map key and value deferral", err)
	}
}

// TestBoxingASetOfArraysHandsBack is the same gate on the Set side, where the member
// rather than a key is the type with no dynamic form.
func TestBoxingASetOfArraysHandsBack(t *testing.T) {
	const src = `const s = new Set<number[]>();
const a: any = s;
console.log(a.size);
`
	err := renderError(t, src)
	if !strings.Contains(err, "members") {
		t.Fatalf("handback reason = %q, want the set member deferral", err)
	}
}

// TestObjectWalksOverACollectionRunLikeNode covers the statics that walk an object's
// own properties. A Map keeps its entries off its property table, so Node answers the
// empty array for every one of them, and the box models that: routing the collection
// through it answers what Node answers where the static path would have tried to read
// fields off a type that has none and handed the build back.
func TestObjectWalksOverACollectionRunLikeNode(t *testing.T) {
	skipIfShort(t)
	const src = `const m = new Map<string, number>();
m.set("a", 1);
console.log(Object.keys(m).length, Object.values(m).length, Object.entries(m).length);
console.log(Object.getOwnPropertyNames(m).length);
const s = new Set<number>();
s.add(1);
console.log(Object.keys(s).length, Object.entries(s).length);
`
	got := runProgramGo(t, src)
	want := "0 0 0\n0\n0 0\n"
	if got != want {
		t.Fatalf("object walk over a collection mismatch:\n got %q\nwant %q", got, want)
	}
}

// renderError renders a program that is expected not to lower and returns the reason,
// failing the test if it lowered after all.
func renderError(t *testing.T, src string) string {
	t.Helper()
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	if _, err := r.RenderProgram(entryFile(t, prog)); err != nil {
		return err.Error()
	}
	t.Fatalf("the program lowered, want a hand-back:\n%s", src)
	return ""
}

// TestABoxedMapRunsLikeNode builds and runs the crossing rather than reading the
// emitted Go, so the whole path is proven: the view is built, the member surface
// answers, and a write through the box is visible to the typed side that follows it.
// The expected output is what Node v24.18.0 prints for the same program.
func TestABoxedMapRunsLikeNode(t *testing.T) {
	skipIfShort(t)
	const src = `const m = new Map<number, number>();
m.set(1, 2);
m.set(3, 4);
console.log(m);
const a: any = m;
console.log(a.size, a.get(1), a.has(3), a.has(9));
a.set(5, 6);
console.log(m.size, m.get(5));
const s = new Set<string>();
s.add("x");
console.log(s);
const b: any = s;
b.add("y");
console.log(s.size, b.has("y"));
console.log(String(a), Object.prototype.toString.call(b));
`
	got := runProgramGo(t, src)
	want := "Map(2) { 1 => 2, 3 => 4 }\n" +
		"2 2 true false\n" +
		"3 6\n" +
		"Set(1) { 'x' }\n" +
		"2 true\n" +
		"[object Map] [object Set]\n"
	if got != want {
		t.Fatalf("boxed collection run mismatch:\n got %q\nwant %q", got, want)
	}
}
