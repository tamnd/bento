package build

import "testing"

// A collection can hold a collection. Before this, a Map<string, Map<string, number>>,
// a Set<Set<number>>, a Map<string, number[]> and a number[][] all failed to build: the
// element gate knew the primitives, a date, the byte buffers, a typed array and a class
// instance, and a container was not on the list. These build real binaries and hold
// their whole output against what Node v24.18.0 prints for the same program.

// TestANestedCollectionPrintsLikeNode is the capability this is for. The mutation in the
// middle is the part that says the inner Map is a view rather than a copy: the outer map
// is logged again after the inner one has grown.
func TestANestedCollectionPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const assert = require('node:assert');\n"+
			"const xs: number[][] = [[1, 2], [3]];\n"+
			"console.log(xs);\n"+
			"const m = new Map<string, Map<string, number>>();\n"+
			"const inner = new Map<string, number>();\n"+
			"inner.set('a', 1);\n"+
			"m.set('k', inner);\n"+
			"console.log(m);\n"+
			"console.log(m.get('k'));\n"+
			"inner.set('b', 2);\n"+
			"console.log(m);\n"+
			"const s = new Set<Set<number>>();\n"+
			"const si = new Set<number>();\n"+
			"si.add(1);\n"+
			"s.add(si);\n"+
			"console.log(s);\n"+
			"console.log(s.has(si));\n"+
			"const ma = new Map<string, number[]>();\n"+
			"ma.set('k', [1, 2]);\n"+
			"console.log(ma);\n"+
			"console.log(ma.get('k'));\n"+
			"const arr: Map<string, number>[] = [inner];\n"+
			"console.log(arr);\n"+
			"const ms = new Map<string, Set<string>>();\n"+
			"ms.set('k', new Set<string>(['a', 'b']));\n"+
			"console.log(ms);\n"+
			"const km = new Map<Map<string, number>, string>();\n"+
			"km.set(inner, 'v');\n"+
			"console.log(km);\n"+
			"console.log(km.has(inner), km.size);\n"+
			"km.delete(inner);\n"+
			"console.log(km.size);\n"+
			"console.log(JSON.stringify([...m.values()]));\n"+
			"assert.deepStrictEqual(m, m);\n"+
			"console.log('nested deep equal');\n")
	want := "[ [ 1, 2 ], [ 3 ] ]\n" +
		"Map(1) { 'k' => Map(1) { 'a' => 1 } }\n" +
		"Map(1) { 'a' => 1 }\n" +
		"Map(1) { 'k' => Map(2) { 'a' => 1, 'b' => 2 } }\n" +
		"Set(1) { Set(1) { 1 } }\n" +
		"true\n" +
		"Map(1) { 'k' => [ 1, 2 ] }\n" +
		"[ 1, 2 ]\n" +
		"[ Map(2) { 'a' => 1, 'b' => 2 } ]\n" +
		"Map(1) { 'k' => Set(2) { 'a', 'b' } }\n" +
		"Map(1) { Map(2) { 'a' => 1, 'b' => 2 } => 'v' }\n" +
		"true 1\n" +
		"0\n" +
		"[{}]\n" +
		"nested deep equal\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestANestedCollectionReadsAndWritesThrough holds the view all the way down. Reaching
// three levels in through a dynamic alias and writing there lands in the innermost typed
// collection, and a set added through the alias is a member the typed side sees, because
// every level of the box is the collection itself rather than a snapshot of it.
func TestANestedCollectionReadsAndWritesThrough(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; }\n"+
			"const deep = new Map<string, Map<string, P>>();\n"+
			"const mid = new Map<string, P>();\n"+
			"mid.set('p', new P());\n"+
			"deep.set('k', mid);\n"+
			"console.log(deep);\n"+
			"const d: any = deep;\n"+
			"console.log(d.get('k').get('p').x);\n"+
			"d.get('k').get('p').x = 9;\n"+
			"console.log(mid.get('p')!.x);\n"+
			"d.get('k').set('q', new P());\n"+
			"console.log(mid.size, deep);\n"+
			"const grid: Date[][] = [[new Date(0)]];\n"+
			"console.log(grid);\n"+
			"const ss = new Set<Map<string, number>>();\n"+
			"const im = new Map<string, number>();\n"+
			"ss.add(im);\n"+
			"console.log(ss.has(im), ss);\n"+
			"const three = new Map<string, Map<string, Map<string, number>>>();\n"+
			"const l2 = new Map<string, Map<string, number>>();\n"+
			"const l3 = new Map<string, number>();\n"+
			"l3.set('z', 1);\n"+
			"l2.set('y', l3);\n"+
			"three.set('x', l2);\n"+
			"console.log(three);\n")
	want := "Map(1) { 'k' => Map(1) { 'p' => P { x: 1 } } }\n" +
		"1\n" +
		"9\n" +
		"2 Map(1) { 'k' => Map(2) { 'p' => P { x: 9 }, 'q' => P { x: 1 } } }\n" +
		"[ [ 1970-01-01T00:00:00.000Z ] ]\n" +
		"true Set(1) { Map(0) {} }\n" +
		"Map(1) { 'x' => Map(1) { 'y' => Map(1) { 'z' => 1 } } }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestANestedArrayIsACopyOfTheTypedOne pins the one place a nested container parts from
// Node, and it is not new here: an array's box holds its own elements, so a write made
// through it does not reach the typed array. The two lines below print [ 1, 2, 3 ] in
// Node. The same is already true of a top-level array box, which is why this slice lets
// an array be a nested value rather than holding it out; when the array grows a live
// view, this test is what says so by failing.
//
// It is also why an array is not allowed as a Map key or a Set member: there the copy
// would make has and delete answer false about a collection that does hold the array,
// which is a wrong answer rather than a lost write, so those hand back at the boxing site.
func TestANestedArrayIsACopyOfTheTypedOne(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const av = new Map<string, number[]>();\n"+
			"av.set('k', [1, 2]);\n"+
			"const dv: any = av;\n"+
			"dv.get('k').push(3);\n"+
			"console.log(av.get('k'));\n"+
			"const a: number[] = [1, 2];\n"+
			"const d: any = a;\n"+
			"d.push(3);\n"+
			"console.log(a);\n")
	want := "[ 1, 2 ]\n" +
		"[ 1, 2 ]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
