package build

import "testing"

// TestADestructuredCallbackParameterHoldsItsBox is the everyday shape this is for. A
// program walks an object and destructures each entry as it goes:
//
//	Object.entries(m).map(([k, v]: [string, Row]) => k + v.id)
//
// The receiver is a box, so the call dispatches through the runtime and the callback is
// handed one boxed argument per parameter. A pattern is not a slot a box lands in: the
// typed binder would read a Go struct field off a value.Value, or a tuple position off
// one. So the whole pattern takes the box and its leaves read out of it through the
// dynamic protocol, which is the answer an unannotated pattern already got, reached
// through the checker's types rather than in spite of them.
//
// A static receiver is untouched: rows.map(({ id }: Row) => id) over a real Row[] still
// binds the Go field.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestADestructuredCallbackParameterHoldsItsBox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"console.log(Object.values(m).map(({ id }: Row) => id));\n"+
			"console.log(Object.values(m).map(({ id, tag }: Row) => tag + id));\n"+
			"console.log(Object.values(m).map(({ id: n }: Row) => n * 2));\n"+
			"console.log(Object.entries(m).map(([k, v]: [string, Row]) => k + v.id));\n"+
			"console.log(Object.entries(m).filter(([k]: [string, Row]) => k === 'b').length);\n"+
			"Object.entries(m).forEach(([k, v]: [string, Row]) => { console.log(k, v.tag); });\n"+
			"console.log(Object.values(m).map(({ id, tag }: Row) => { const s = tag.toUpperCase(); return s + id; }));\n"+
			"const loose = JSON.parse('[{\"id\":9,\"extra\":true}]') as any[];\n"+
			"console.log(loose.map(({ id, ...rest }: any) => id + JSON.stringify(rest)));\n"+
			"console.log(loose.map(({ id, missing = 7 }: any) => id + missing));\n"+
			"const pairs = JSON.parse('[[1,[2,3]]]') as any[];\n"+
			"console.log(pairs.map(([a, [b, c]]: any) => a + b + c));\n"+
			"console.log(pairs.map(([a, ...tail]: any) => a + JSON.stringify(tail)));\n"+
			"const rows: Row[] = [{ id: 3, tag: 'z' }];\n"+
			"console.log(rows.map(({ id }: Row) => id));\n")
	want := "[ 1, 2 ]\n[ 'x1', 'y2' ]\n[ 2, 4 ]\n[ 'a1', 'b2' ]\n1\na x\nb y\n[ 'X1', 'Y2' ]\n[ '9{\"extra\":true}' ]\n[ 16 ]\n[ 6 ]\n[ '1[[2,3]]' ]\n[ 3 ]\n"
	if got != want {
		t.Errorf("a destructured callback parameter read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestADestructuredBindComesDownToItsPrimitive is the part that would have built and
// then thrown rather than handed back. A leaf the checker types number or string or
// boolean cannot stay a box, because the runtime's Get on a boxed string answers only
// length and the indices, so tag.toUpperCase() found undefined and called it. The bind
// coerces such a leaf down to its Go primitive, which is the rule a read off a box
// already followed, asked at the other kind of site.
//
// The first line is the one that was already shipped and already wrong: a for-of over
// Object.entries of a box binds k the same way, and k.toUpperCase() threw.
//
// The declarations after it are the same binder reached from a plain const. Its gate
// asked only what the checker calls the source, which for Object.values(m)[0] is a Row,
// so it selected the Go field Id off a value.Value. That is Go that does not compile,
// so it goes here rather than waiting for the binding slice.
//
// Held against what Node v24.18.0 prints.
func TestADestructuredBindComesDownToItsPrimitive(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string; ok: boolean };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\",\"ok\":true},\"b\":{\"id\":2,\"tag\":\"y\",\"ok\":false}}') as Record<string, Row>;\n"+
			"for (const [k, v] of Object.entries(m)) { console.log(k.toUpperCase(), v.id); }\n"+
			"const { id, tag } = Object.values(m)[0];\n"+
			"console.log(id + 1, tag.toUpperCase());\n"+
			"const [k0, v0] = Object.entries(m)[0];\n"+
			"console.log(k0.toUpperCase(), v0.id);\n"+
			"console.log(Object.values(m).map(({ tag }: Row) => tag.padStart(3, '.')));\n"+
			"console.log(Object.entries(m).map(([k, v]: [string, Row], i: number) => k + v.id + i));\n"+
			"console.log(Object.entries(m).map(([k, v]: [string, Row]) => ({ k, n: v.id })));\n"+
			"console.log(Object.values(m).reduce((acc: number, { id }: Row) => acc + id, 0));\n"+
			"console.log(Object.values(m).find(({ ok }: Row) => ok === false)?.tag);\n"+
			"const opt = JSON.parse('[{\"id\":5}]') as { id: number; tag?: string }[];\n"+
			"console.log(opt.map(({ id, tag }: { id: number; tag?: string }) => id + String(tag)));\n"+
			"const deep = JSON.parse('[{\"inner\":{\"x\":3},\"list\":[4,5]}]') as any[];\n"+
			"console.log(deep.map(({ inner: { x }, list: [p, q] }: any) => x + p + q));\n"+
			"try { throw { code: 'E', n: 1 }; } catch ({ code, n }: any) { console.log(code, n); }\n")
	want := "A 1\nB 2\n2 X\nA 1\n[ '..x', '..y' ]\n[ 'a10', 'b21' ]\n" +
		"[ { k: 'a', n: 1 }, { k: 'b', n: 2 } ]\n3\ny\n[ '5undefined' ]\n[ 12 ]\nE 1\n"
	if got != want {
		t.Errorf("a destructured bind off a box read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
