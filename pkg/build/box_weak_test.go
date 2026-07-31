package build

import "testing"

// The four weakly-holding kinds box now. Before this a console.log of a WeakMap, a
// WeakSet, a WeakRef or a FinalizationRegistry failed to build with "boxing this static
// type into a dynamic value is a later slice", since the boxing wall knew a Map and a
// Set and nothing weakly held. These build real binaries and hold their whole output
// against what Node v24.18.0 prints for the same program.

// TestAWeakCollectionPrintsLikeNode is the capability this is for. What a weak
// collection shows is a name and, for the two that hold entries, the placeholder node
// prints where a Map prints entries: a program that could count what a WeakMap holds
// could watch the garbage collector run, so there is nothing to walk and that is the
// point rather than a gap.
func TestAWeakCollectionPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class O { a = 1; }\n"+
			"const o = new O();\n"+
			"const wm = new WeakMap<O, number>();\n"+
			"wm.set(o, 1);\n"+
			"console.log(wm);\n"+
			"console.log(wm.get(o), wm.has(o));\n"+
			"const ws = new WeakSet<O>();\n"+
			"ws.add(o);\n"+
			"console.log(ws);\n"+
			"const wr = new WeakRef<O>(o);\n"+
			"console.log(wr);\n"+
			"console.log(wr.deref());\n"+
			"const fr = new FinalizationRegistry<string>((h: string) => { void h; });\n"+
			"fr.register(o, 'held');\n"+
			"console.log(fr);\n"+
			"console.log(typeof wm, typeof wr);\n"+
			"console.log(JSON.stringify(wm), JSON.stringify(wr));\n"+
			"console.log(Object.keys(wm));\n"+
			"const m = new Map<string, WeakMap<O, number>>();\n"+
			"m.set('k', wm);\n"+
			"console.log(m);\n"+
			"const d: any = wm;\n"+
			"console.log(d.get(o), d.has(o), d.delete(o), d.has(o));\n"+
			"const ds: any = ws;\n"+
			"console.log(ds.has(o), ds.delete(o), ds.has(o));\n"+
			"const dr: any = wr;\n"+
			"console.log(dr.deref());\n"+
			"console.log([wr]);\n"+
			"const s2 = new Set<WeakSet<O>>();\n"+
			"s2.add(ws);\n"+
			"console.log(s2, s2.has(ws));\n")
	want := "WeakMap { <items unknown> }\n" +
		"1 true\n" +
		"WeakSet { <items unknown> }\n" +
		"WeakRef {}\n" +
		"O { a: 1 }\n" +
		"FinalizationRegistry {}\n" +
		"object object\n" +
		"{} {}\n" +
		"[]\n" +
		"Map(1) { 'k' => WeakMap { <items unknown> } }\n" +
		"1 true true false\n" +
		"true true false\n" +
		"O { a: 1 }\n" +
		"[ WeakRef {} ]\n" +
		"Set(1) { WeakSet { <items unknown> } } true\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAWeakCollectionIsFoundByReference holds the identity half. The box is the view
// kept on the collection, so a WeakMap stored as a Map's key is found again by the same
// WeakMap, a write made through a dynamic alias lands in the typed one, and the deep
// comparison answers what node answers: a weak collection is compared by reference and
// by nothing else, since its contents are unreadable, so two distinct empty WeakMaps
// are not equal however alike they print.
func TestAWeakCollectionIsFoundByReference(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const assert = require('node:assert');\n"+
			"class P { x = 1; }\n"+
			"const a = new P(), b = new P();\n"+
			"const wm = new WeakMap<P, string>();\n"+
			"wm.set(a, 'A');\n"+
			"const km = new Map<WeakMap<P, string>, string>();\n"+
			"km.set(wm, 'v');\n"+
			"console.log(km, km.has(wm), km.get(wm));\n"+
			"km.delete(wm);\n"+
			"console.log(km.size);\n"+
			"const dm: any = wm;\n"+
			"dm.set(b, 'B');\n"+
			"console.log(wm.get(b), wm.get(a));\n"+
			"assert.deepStrictEqual(wm, wm);\n"+
			"console.log('same weakmap equal');\n"+
			"try { assert.deepStrictEqual(new WeakMap<P, string>(), new WeakMap<P, string>()); console.log('two weakmaps equal'); }\n"+
			"catch { console.log('two weakmaps not equal'); }\n"+
			"try { assert.deepStrictEqual(new WeakSet<P>(), new WeakMap<P, string>()); console.log('ws==wm'); }\n"+
			"catch { console.log('ws!=wm'); }\n"+
			"try { assert.deepStrictEqual(new WeakMap<P, string>(), {}); console.log('wm=={}'); }\n"+
			"catch { console.log('wm!={}'); }\n"+
			"console.log(!!wm, wm === dm);\n"+
			"const nested = new Map<string, Map<string, WeakSet<P>>>();\n"+
			"const inner = new Map<string, WeakSet<P>>();\n"+
			"inner.set('w', new WeakSet<P>());\n"+
			"nested.set('k', inner);\n"+
			"console.log(nested);\n"+
			"const wr = new WeakRef<P>(a);\n"+
			"const wrs = new Set<WeakRef<P>>();\n"+
			"wrs.add(wr);\n"+
			"console.log(wrs.has(wr), wrs);\n"+
			"console.log(wr.deref()!.x);\n"+
			"console.log(require('node:util').inspect(nested, { depth: 0 }));\n")
	want := "Map(1) { WeakMap { <items unknown> } => 'v' } true v\n" +
		"0\n" +
		"B A\n" +
		"same weakmap equal\n" +
		"two weakmaps not equal\n" +
		"ws!=wm\n" +
		"wm!={}\n" +
		"true true\n" +
		"Map(1) { 'k' => Map(1) { 'w' => WeakSet { <items unknown> } } }\n" +
		"true Set(1) { WeakRef {} }\n" +
		"1\n" +
		"Map(1) { 'k' => [Map] }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
