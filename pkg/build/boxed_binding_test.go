package build

import "testing"

// TestABoxedChainBoundToANameHoldsItsBox is the everyday shape this is for. A program
// walks parsed JSON and keeps a piece of it:
//
//	const first = Object.values(m)[0];
//	first.id;
//
// The walk hands back boxes and the checker calls the element a Row, so the binding
// asked for a Go struct and was handed a value.Value. There is no way to fill that
// struct from a box short of copying its properties, which would alias nothing where
// JavaScript aliases everything, so the binding holds the box and every read off the
// name dispatches at run time.
//
// A binding the checker types a primitive is not this. const t = first.tag comes down to
// a Go string at the read, the same rule every read off a box follows, and t.toUpperCase()
// is the static string method.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestABoxedChainBoundToANameHoldsItsBox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const first = Object.values(m)[0];\n"+
			"console.log(first.id, first.tag);\n"+
			"const all = Object.values(m);\n"+
			"console.log(all.length, all[1].id);\n"+
			"const e = Object.entries(m)[0];\n"+
			"console.log(e[0], e[1].id);\n"+
			"const t = first.tag;\n"+
			"console.log(t.toUpperCase(), first.id + 1);\n"+
			"console.log(all.map((r: Row) => r.id), all.filter((r: Row) => r.id > 1).length);\n"+
			"const [p, q] = Object.values(m);\n"+
			"console.log(p.id, q.tag);\n"+
			"const { id, tag } = first;\n"+
			"console.log(id, tag);\n"+
			"for (const r of all) { console.log(r.tag); }\n"+
			"console.log(JSON.stringify(first));\n"+
			"const again = first;\n"+
			"console.log(again.id);\n"+
			"let cur = Object.values(m)[0];\n"+
			"cur = Object.values(m)[1];\n"+
			"console.log(cur.id);\n"+
			"cur = { id: 9, tag: 'z' };\n"+
			"console.log(cur.id, cur.tag);\n")
	want := "1 x\n2 2\na 1\nX 2\n[ 1, 2 ] 1\n1 y\n1 x\nx\ny\n{\"id\":1,\"tag\":\"x\"}\n1\n2\n9 z\n"
	if got != want {
		t.Errorf("a boxed chain bound to a name read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestALiteralHoldingABoxIsBoxedWhole covers where the box goes next. A literal built
// around one is the case that would have been Go that does not compile rather than a
// hand-back: the checker types { first: Object.values(m)[0] } as { first: Row } and
// interns a Go struct for it, and the box has no fields to fill that struct's with.
//
// The literal boxes whole instead, so what it holds is stored as itself and the object
// stays the one object every other reference to it sees. A struct copy would have lost
// exactly that.
//
// A spread of a box is the same question about the source rather than the member, and
// its answer is Assign, which copies the source's own enumerable properties onto what
// the literal has built so far. Order carries the override rule for free, which the
// second spread below reads: a key written after a spread wins over the one it brought.
// A key written before the spread is the other half of that rule and is not here,
// because TypeScript rejects the literal outright under 2783.
//
// The tail is the reads that come after, an optional link among them. That one answers
// OptionalMember, which is a box whatever the checker calls the result, so first?.tag is
// still a box and the ?.length after it dispatches rather than mapping an optional that
// is not there.
//
// Held against what Node v24.18.0 prints.
func TestALiteralHoldingABoxIsBoxedWhole(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const first = Object.values(m)[0];\n"+
			"const o = { first: Object.values(m)[0] };\n"+
			"console.log(o.first.id);\n"+
			"const xs = [Object.values(m)[0]];\n"+
			"console.log(xs[0].tag);\n"+
			"const nested = { rows: Object.values(m), n: 1 };\n"+
			"console.log(nested.rows.length, nested.n, nested.rows[0].tag);\n"+
			"console.log(JSON.stringify({ ...first, extra: 3 }));\n"+
			"console.log(JSON.stringify({ ...first, id: 5 }));\n"+
			"console.log(`id=${first.id} tag=${first.tag}`);\n"+
			"console.log(first.id > 1 ? first.tag : 'no');\n"+
			"console.log(typeof first, first === undefined, first != null);\n"+
			"console.log(first?.id, first?.tag?.length);\n"+
			"console.log(first);\n"+
			"console.log(Object.values(m));\n")
	want := "1\nx\n2 1 x\n{\"id\":1,\"tag\":\"x\",\"extra\":3}\n{\"id\":5,\"tag\":\"x\"}\n" +
		"id=1 tag=x\nno\nobject false true\n1 1\n{ id: 1, tag: 'x' }\n[ { id: 1, tag: 'x' }, { id: 2, tag: 'y' } ]\n"
	if got != want {
		t.Errorf("a literal holding a box read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
