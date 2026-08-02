package build

import "testing"

// TestModuleDestructuringLeafThatHoldsABox covers a module-level destructuring leaf whose
// Go slot holds a box and which a function reads:
//
//	const raw = JSON.parse('...') as Record<string, Row>
//	const { a } = raw
//	function tagOf(): string { return a.tag }
//
// Destructuring a boxed source binds each leaf to a value.Value. The body that ran the
// pattern already knew that, since the binder records the names it assigned and every
// later read in that body dispatches through the value model, but that record is keyed by
// name and lives only while that body lowers. A top-level function reading the leaf is not
// inside it, so the read lowered against the checker's shape for the name and the Go said
// `a.Tag undefined (type value.Value has no field or method Tag)`.
//
// The answer now comes from the declaration, keyed by symbol and settled inside the
// boxed-signature fixpoint, which is what the plain path has always done and what lets a
// leaf flow into a function's parameter as a box rather than drive a coercion into a
// static struct that has none.
//
// The pattern shapes are here: an object and an array leaf, a rest of each, a rename, a
// default, and a nesting. So are the leaves that are not boxes, a number, a string and a
// boolean, which come down to their Go primitives at the bind and must keep doing so. So
// are the readers: a function declaration, a nested one, an arrow, a function expression, a
// method, a static method, a static field initializer, and a class field. So are the sinks
// a box has to reach: a static parameter, a declared return type, a typed array, JSON, and
// a reassignment of a let leaf.
//
// Held against what Node v24.18.0 prints, one program.
func TestModuleDestructuringLeafThatHoldsABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const { a } = raw;\n"+
			"const { a: alias, ...others } = raw;\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"},{\"id\":3,\"tag\":\"z\"}]') as Row[];\n"+
			"const [r0, ...restRows] = rows;\n"+
			"let [cur] = rows;\n"+
			"const empty = JSON.parse('{}') as Record<string, Row>;\n"+
			"const { a: dflt = { id: 9, tag: 'q' } } = empty;\n"+
			"const deep = JSON.parse('{\"o\":{\"n\":5,\"s\":\"w\"}}') as Record<string, { n: number; s: string }>;\n"+
			"const { o: { n, s } } = deep;\n"+
			"const prim = JSON.parse('{\"n\":3,\"s\":\"hi\",\"b\":true}') as { n: number; s: string; b: boolean };\n"+
			"const { n: pn, s: ps, b: pb } = prim;\n"+
			"const out: string[] = [];\n"+
			"function take(r: Row): string { return r.tag; }\n"+
			"function give(): Row { return a; }\n"+
			"function tagOf(): string { return a.tag; }\n"+
			"function outer(): string { function inner(): string { return alias.tag; } return inner(); }\n"+
			"const arrow = (): number => r0.id;\n"+
			"const fe = function (): string { return restRows.map((r) => r.tag).join(','); };\n"+
			"class C { static st = a.tag; fld = r0.id; m(): string { return `${n}${s}`; } static go(): string { return dflt.tag; } }\n"+
			"const acc: Row[] = [r0, a];\n"+
			"function swap(): void { cur = rows[2]; }\n"+
			"out.push(`${tagOf()} ${a.id} ${take(a)} ${give().tag}`);\n"+
			"out.push(`${outer()} ${Object.keys(others).length} ${arrow()} ${fe()}`);\n"+
			"out.push(`${C.st} ${new C().fld} ${new C().m()} ${C.go()}`);\n"+
			"out.push(`${acc.map((r) => r.tag).join('')} ${JSON.stringify(a)} ${typeof a}`);\n"+
			"out.push(`${pn + 1} ${ps.toUpperCase()} ${pb}`);\n"+
			"out.push(cur.tag);\n"+
			"swap();\n"+
			"out.push(cur.tag);\n"+
			"console.log(out.join(' / '));\n")
	want := "x 1 x x / x 1 1 y,z / x 1 5w q / xx {\"id\":1,\"tag\":\"x\"} object / 4 HI true / x / z\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
