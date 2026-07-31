package build

import "testing"

// A boxed class instance is a view of the instance rather than a copy of its fields,
// which is what lets a Map or a Set hold one: a collection's box is itself a view, so
// the element boxing under it has to run in both directions. These build real binaries
// and hold their whole output against what Node v24.18.0 prints for the same program.

// TestACollectionOfInstancesPrintsLikeNode is the capability this is for. Before the
// view, all of these failed to build: "boxing a Map whose keys or values are not a
// number, string, boolean, date, or dynamic value into a dynamic value is a later
// slice". The mutation in the middle is the part a copy would get wrong, since the
// collection is logged after the instance it holds has changed.
func TestACollectionOfInstancesPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; y = 's'; }\n"+
			"const p = new P();\n"+
			"const m = new Map<string, P>();\n"+
			"m.set('k', p);\n"+
			"const s = new Set<P>();\n"+
			"s.add(p);\n"+
			"console.log(m);\n"+
			"console.log(s);\n"+
			"console.log(s.has(p));\n"+
			"console.log(m.has('k'));\n"+
			"p.x = 9;\n"+
			"console.log(m);\n"+
			"console.log(s);\n"+
			"s.delete(p);\n"+
			"console.log(s.size);\n"+
			"class Node2 { v = 1; next = new P(); }\n"+
			"const m2 = new Map<number, Node2>();\n"+
			"m2.set(1, new Node2());\n"+
			"console.log(m2);\n"+
			"console.log(JSON.stringify([...m2.values()]));\n")
	want := "Map(1) { 'k' => P { x: 1, y: 's' } }\n" +
		"Set(1) { P { x: 1, y: 's' } }\n" +
		"true\n" +
		"true\n" +
		"Map(1) { 'k' => P { x: 9, y: 's' } }\n" +
		"Set(1) { P { x: 9, y: 's' } }\n" +
		"0\n" +
		"Map(1) { 1 => Node2 { v: 1, next: P { x: 1, y: 's' } } }\n" +
		"[{\"v\":1,\"next\":{\"x\":1,\"y\":\"s\"}}]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestACollectionKeyedByInstancesFindsThem covers the direction a copy could not do at
// all. A key handed to has or delete arrives at the typed map as a boxed value and has
// to come back out as the instance the map was keyed by, which is the pointer the view
// carries; a bag of the same fields would find nothing. The set membership below it is
// the same test from the other side: adding one instance twice adds one member, and two
// instances with identical fields are two.
func TestACollectionKeyedByInstancesFindsThem(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const assert = require('node:assert');\n"+
			"class P { x = 1; y = 's'; }\n"+
			"const p = new P();\n"+
			"const q = new P();\n"+
			"const km = new Map<P, number>();\n"+
			"km.set(p, 10);\n"+
			"console.log(km);\n"+
			"console.log(km.has(p), km.has(q), km.size);\n"+
			"km.delete(p);\n"+
			"console.log(km.size);\n"+
			"const s = new Set<P>();\n"+
			"s.add(p); s.add(p); s.add(q);\n"+
			"console.log(s.size);\n"+
			"console.log(s);\n"+
			"const a = new Map<string, P>();\n"+
			"a.set('k', new P());\n"+
			"const b = new Map<string, P>();\n"+
			"b.set('k', new P());\n"+
			"assert.deepStrictEqual(a, b);\n"+
			"console.log('maps of instances deep equal');\n"+
			"class Q { x = 1; y = 's'; }\n"+
			"const c = new Map<string, Q>();\n"+
			"c.set('k', new Q());\n"+
			"try { assert.deepStrictEqual(a, c); console.log('bad'); }\n"+
			"catch { console.log('different classes not equal'); }\n"+
			"for (const v of s) { console.log(v.x); }\n"+
			"s.forEach((v) => { console.log(v.y); });\n")
	want := "Map(1) { P { x: 1, y: 's' } => 10 }\n" +
		"true false 1\n" +
		"0\n" +
		"2\n" +
		"Set(2) { P { x: 1, y: 's' }, P { x: 1, y: 's' } }\n" +
		"maps of instances deep equal\n" +
		"different classes not equal\n" +
		"1\n" +
		"1\n" +
		"s\n" +
		"s\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedInstanceIsAViewOfTheInstance holds the view through a dynamic alias, which
// is the shortest way a program keeps a box alive across a mutation. Every line here was
// wrong under the copying box: a write through the alias vanished, and a read through it
// answered whatever the fields held at the moment the alias was made.
func TestABoxedInstanceIsAViewOfTheInstance(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; y = 's'; }\n"+
			"const p = new P();\n"+
			"const d: any = p;\n"+
			"d.x = 5;\n"+
			"console.log(p.x);\n"+
			"console.log(d);\n"+
			"p.y = 't';\n"+
			"console.log(d);\n"+
			"d.y = 'u';\n"+
			"console.log(p.y);\n"+
			"console.log(Object.keys(d));\n"+
			"console.log(JSON.stringify(d));\n")
	want := "5\n" +
		"P { x: 5, y: 's' }\n" +
		"P { x: 5, y: 't' }\n" +
		"u\n" +
		"[ 'x', 'y' ]\n" +
		"{\"x\":5,\"y\":\"u\"}\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestACycleThroughInstancesPrintsItsMarker covers a shape only a view makes reachable.
// The copying box walked an instance's fields eagerly, so a class pointing at itself ran
// off the stack before anything could be printed; a view reads a field only when the
// field is read, which is what lets the walk get far enough to recognize the cycle.
func TestACycleThroughInstancesPrintsItsMarker(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class N { v = 1; next: N | null = null; }\n"+
			"const a = new N();\n"+
			"a.next = a;\n"+
			"console.log(a);\n"+
			"const b = new N();\n"+
			"b.v = 2;\n"+
			"a.next = b;\n"+
			"b.next = a;\n"+
			"console.log(a);\n")
	want := "<ref *1> N { v: 1, next: [Circular *1] }\n" +
		"<ref *1> N { v: 1, next: N { v: 2, next: [Circular *1] } }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAWriteAFieldCannotHoldRaises holds the one place this parts from Node. A field
// lowers to a Go field of one type, so a field declared with a number cannot be made to
// hold a string however the write is spelled; Node prints P { x: 'hello' } and bento
// raises. Raising is the honest end of it, since the alternative under a view is a box
// and an instance that disagree about the same field, and it is already better than what
// the copying box did, which was to take the write and drop it.
//
// The buffer field below it is the write that does work, and it is the interesting half:
// the field takes the very array the value boxes rather than a copy, so the bytes the
// typed side reads afterwards are the ones the writer wrote.
func TestAWriteAFieldCannotHoldRaises(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; }\n"+
			"const p = new P();\n"+
			"const d: any = p;\n"+
			"try { d.x = 'hello'; } catch (e) { console.log('caught:', e.message); }\n"+
			"console.log(p.x);\n"+
			"class B { b = new Uint8Array(2); }\n"+
			"const bb = new B();\n"+
			"const db: any = bb;\n"+
			"console.log(db);\n"+
			"db.b = new Uint8Array([1, 2]);\n"+
			"console.log(bb.b[0]);\n")
	want := "caught: bento: .x of this instance cannot hold a value of this type\n" +
		"1\n" +
		"B { b: Uint8Array(2) [ 0, 0 ] }\n" +
		"1\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
