package build

import "testing"

// TestPairOfABoxedCollection covers the [key, value] entry a Map or Set gives back once
// the boxed-signature pass has rewritten one of its slots:
//
//	for (const e of mm) { out.push(`${e[0]}${e[1].tag}`) }
//
// A collection whose key, value or member type reaches the pass gets a value.Value slot,
// and the entry such a collection yields therefore holds a box in at least one half. The
// interned tuple an entry otherwise materializes into is a Go struct, and a struct field
// has no room for a box, so the pair gives way rather than the value: it becomes the
// two-element array JavaScript says an entry actually is, and every read of it goes
// through the value model.
//
// That is also the closer reading of the source. The entry answers its length, its index
// reads, Array.isArray, typeof, join, spread and JSON.stringify with no tuple struct in
// the picture at all, which is what the middle of this program pins.
//
// The spellings that yield a pair are all here: a Map ranged directly, either kind's
// entries(), a boxed key with a static value, and a Set whose entry is its member twice.
// So are the places a pair lands: a for...of binding read by index, a destructuring
// for...of head, an assignment destructuring onto already-declared locals, a declaration
// destructure off a collected entry, a closure that outlives the turn, a splice beside
// another element, and a nested loop over the same collection.
//
// Held against what Node v24.18.0 prints, one program so the insertion ordering is
// pinned too.
func TestPairOfABoxedCollection(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const mm = new Map<string, Row>();\n"+
			"mm.set('a', raw['a']);\n"+
			"mm.set('b', raw['b']);\n"+
			"const bs = new Set<Row>();\n"+
			"bs.add(raw['a']);\n"+
			"bs.add(raw['b']);\n"+
			"const km = new Map<Row, number>();\n"+
			"km.set(raw['a'], 7);\n"+
			"const empty = new Map<string, Row>();\n"+
			"const out: string[] = [];\n"+
			"for (const e of mm) { out.push(`m ${e[0]}${e[1].tag}`); }\n"+
			"for (const e of mm.entries()) { out.push(`n ${e[0]}${e[1].id}`); }\n"+
			"for (const e of bs.entries()) { out.push(`s ${e[0].tag}${e[1].id}`); }\n"+
			"for (const e of km) { out.push(`k ${e[0].tag}=${e[1] + 1}`); }\n"+
			"for (const [k, v] of mm) { out.push(`d ${k}${v.tag}`); }\n"+
			"for (const e of mm) {\n"+
			"  let k = '';\n"+
			"  let v: Row = raw['a'];\n"+
			"  [k, v] = e;\n"+
			"  out.push(`a ${k}${v.tag}`);\n"+
			"}\n"+
			"out.push(JSON.stringify([...mm]));\n"+
			"out.push(JSON.stringify(Array.from(mm.entries())));\n"+
			"out.push(JSON.stringify([...bs.entries()]));\n"+
			"out.push(JSON.stringify(Array.from(km)));\n"+
			"const es = [...mm];\n"+
			"out.push(`${es.length} ${es[0].length} ${Array.isArray(es[0])} ${typeof es[0]}`);\n"+
			"out.push(`${es[0].join('-')} ${es[0] === es[0]} ${es[0] === es[1]}`);\n"+
			"out.push(`${[...bs].length} ${[...bs][1].tag} ${[...bs.values()][0].id}`);\n"+
			"const mixed = [raw['a'], ...mm];\n"+
			"out.push(`${mixed.length} ${JSON.stringify(mixed[2])}`);\n"+
			"let n = 0;\n"+
			"for (const e of empty) { n += e.length; }\n"+
			"out.push(`${n} ${[...empty].length} ${Array.from(empty).length}`);\n"+
			"const [fk, fv] = [...mm][0];\n"+
			"out.push(`c ${fk}${fv.tag}`);\n"+
			"const fns: (() => string)[] = [];\n"+
			"for (const e of mm) { fns.push(() => `${e[0]}${e[1].id}`); }\n"+
			"out.push(fns.map((f) => f()).join(','));\n"+
			"for (const e of mm) { for (const f of mm) { out.push(`x ${e[0]}${f[0]}`); } }\n"+
			"console.log(out.join(' / '));\n")
	want := "m ax / m by / n a1 / n b2 / s x1 / s y2 / k x=8 / d ax / d by / a ax / a by / " +
		"[[\"a\",{\"id\":1,\"tag\":\"x\"}],[\"b\",{\"id\":2,\"tag\":\"y\"}]] / " +
		"[[\"a\",{\"id\":1,\"tag\":\"x\"}],[\"b\",{\"id\":2,\"tag\":\"y\"}]] / " +
		"[[{\"id\":1,\"tag\":\"x\"},{\"id\":1,\"tag\":\"x\"}],[{\"id\":2,\"tag\":\"y\"},{\"id\":2,\"tag\":\"y\"}]] / " +
		"[[{\"id\":1,\"tag\":\"x\"},7]] / 2 2 true object / a-[object Object] true false / 2 y 1 / " +
		"3 [\"b\",{\"id\":2,\"tag\":\"y\"}] / 0 0 0 / c ax / a1,b2 / x aa / x ab / x ba / x bb\n"
	if got != want {
		t.Fatalf("pair of a boxed collection:\ngot  %q\nwant %q", got, want)
	}
}
