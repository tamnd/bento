package build

import "testing"

// TestModuleBindingThatHoldsABox covers a module binding whose Go slot holds a box and
// which a function reads:
//
//	const ns = JSON.parse('[3,1,2]') as number[]
//	function total(): number { let t = 0; for (const n of ns) t += n; return t }
//
// Being read from a function is what moves the binding out of main and onto a
// package-level var. That move used to change what the compiler thought was in the slot:
// the reads dispatched through the value model and the initializer lowered as a box,
// while the package var was still spelled with the array type the checker gave the name,
// so the generated Go did not compile.
//
// Where a variable lives has nothing to do with what is in it, so the package var is now
// declared off the same rule the in-main declaration and every read of the name already
// used.
//
// The shapes that reach it are all here: an array of numbers, of strings, of shapes, a
// record, and a binding a later assignment boxes. So are the readers that make it hoist:
// a function declaration, a nested one, a concise arrow of each primitive result, a
// function expression, a method, a static method, a static field initializer, and a
// class field.
//
// Held against what Node v24.18.0 prints, one program.
func TestModuleBindingThatHoldsABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"}]') as Row[];\n"+
			"const ns = JSON.parse('[3,1,2]') as number[];\n"+
			"const ss = JSON.parse('[\"a\",\"b\"]') as string[];\n"+
			"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"let cur: Row = { id: 0, tag: 'z' };\n"+
			"cur = raw['a'];\n"+
			"const out: string[] = [];\n"+
			"function tags(): string { return rows.map((r) => r.tag).join(','); }\n"+
			"function total(): number { let t = 0; for (const n of ns) t += n; return t; }\n"+
			"function nth(i: number): string { return rows[i].tag; }\n"+
			"function tagOf(k: string): string { return raw[k].tag; }\n"+
			"function count(): number { return Object.keys(raw).length; }\n"+
			"function curTag(): string { return cur.tag; }\n"+
			"function each(): string { const acc: string[] = []; for (const r of rows) acc.push(r.tag); return acc.join(','); }\n"+
			"function filt(): number { return rows.filter((r) => r.id > 1).length; }\n"+
			"function json(): string { return JSON.stringify(rows); }\n"+
			"function outer(): string { function inner(): number { return ns.length; } return `${inner()} ${ns[0]}`; }\n"+
			"const j = (): string => ss.join('|');\n"+
			"const l = (): number => ns.length;\n"+
			"const bool = (): boolean => ns.length > 0;\n"+
			"const g = function (): string { return ns.join(','); };\n"+
			"class C { static n = ns.length; static go(): number { return ns[0]; } }\n"+
			"class D { xs = ns; len(): number { return this.xs.length; } }\n"+
			"out.push(`${tags()} ${total()} ${nth(1)}`);\n"+
			"out.push(`${tagOf('a')}${tagOf('b')} ${count()} ${curTag()}`);\n"+
			"out.push(`${each()} ${filt()} ${json()}`);\n"+
			"out.push(`${outer()} ${j()} ${l()} ${bool()} ${g()}`);\n"+
			"out.push(`${C.n} ${C.go()} ${new D().len()}`);\n"+
			"out.push(`${ns.length} ${rows.length} ${cur.id}`);\n"+
			"out.push(`${[...ns].length} ${rows.map((r) => r.id).join(',')}`);\n"+
			"console.log(out.join(' / '));\n")
	want := "x,y 6 y / xy 2 x / x,y 1 [{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"}] / " +
		"3 3 a|b 3 true 3,1,2 / 3 3 3 / 3 2 1 / 3 1,2\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
