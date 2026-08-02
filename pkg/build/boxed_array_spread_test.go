package build

import "testing"

// TestSpreadOfABoxedArray covers `[...xs]` and `f(...xs)` where xs holds a box:
//
//	const ns = JSON.parse('[3,1,2]') as number[]
//	const copy = [...ns]
//
// The checker still calls ns a number[], so the splice used to reach for an Elems field
// the value.Value in its slot has not got, which broke the build. What an expression
// lowers to has to win over what the checker named it, so a boxed operand is drained at
// run time by the same value.Iterate a for...of over it drives.
//
// Where the drained boxes land then depends on the target. A slot of boxes takes them as
// they stand. A slot of numbers, strings or booleans brings each one down at the splice,
// so the copy is the ordinary Go slice its type names and no later reader has to know it
// came from a box. A slot of shapes has no Go value to land a box in, so the literal
// around the spread gives way and becomes a boxed array instead.
//
// All three land here, along with the places a spread appears: alone, beside other
// elements, twice over, nested, inside an object literal, into a Set, into a typed rest
// parameter, into a destructuring, and straight into console.log. Array.from over the
// same boxed sources rides along because it collects through the same rule.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestSpreadOfABoxedArray(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const ns = JSON.parse('[3,1,2]') as number[];\n"+
			"const ss = JSON.parse('[\"a\",\"b\"]') as string[];\n"+
			"const bs = JSON.parse('[true,false]') as boolean[];\n"+
			"const em = JSON.parse('[]') as number[];\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"}]') as Row[];\n"+
			"const v = JSON.parse('[5,6]') as unknown;\n"+
			"function sum(...xs: number[]): number { let t = 0; for (const x of xs) t += x; return t; }\n"+
			"function cat(sep: string, ...xs: string[]): string { return xs.join(sep); }\n"+
			"const out: string[] = [];\n"+
			"const c = [...ns];\n"+
			"c.push(9);\n"+
			"out.push(`${c.join(',')} ${ns.length}`);\n"+
			"out.push(`${[0, ...ns, 9].join(',')} ${[...ns, ...ns].join(',')}`);\n"+
			"out.push(`${['z', ...ss].join(',')} ${[...bs].join(',')} ${[...em].length}`);\n"+
			"out.push(`${[...ns].sort().join(',')} ${[...ns].reverse().join(',')} ${[...ns].map((x) => x * 2).join(',')}`);\n"+
			"out.push(`${[...ns].reduce((p, q) => p + q, 0)} ${[...ns].every((x) => x > 0)} ${[...ns].indexOf(2)}`);\n"+
			"out.push(`${[...ns][0] + 1} ${[...ns].at(-1)} ${JSON.stringify([...ns])}`);\n"+
			"out.push(`${[...(v as number[])].join(',')}`);\n"+
			"const copy = [...rows];\n"+
			"out.push(`${copy.length} ${copy[0].tag} ${JSON.stringify(copy)}`);\n"+
			"out.push(`${[...rows].map((r) => r.id).join(',')}`);\n"+
			"for (const r of [...rows]) out.push(`r ${r.tag}`);\n"+
			"const anys: unknown[] = [...ns];\n"+
			"out.push(`${anys.length}`);\n"+
			"const nested: number[][] = [[...ns], [...ns].slice(1)];\n"+
			"out.push(`${JSON.stringify(nested)} ${[...[...ns]].join(',')} ${JSON.stringify({ xs: [...ns] })}`);\n"+
			"out.push(`${new Set([...ns, ...ns]).size} ${[...new Set([...ns])].join(',')}`);\n"+
			"out.push(`${sum(...ns)} ${sum(1, ...ns, 5)} ${cat('-', 'z', ...ss)}`);\n"+
			"const [p, q] = [...ns];\n"+
			"out.push(`${p} ${q}`);\n"+
			"out.push(`${Array.from(ns).join(',')} ${Array.from(ns, (x) => x * 2).join(',')} ${Array.from(rows, (r) => r.tag).join(',')}`);\n"+
			"console.log(out.join(' / '));\n"+
			"console.log([...ns]);\n"+
			"console.log([...rows]);\n")
	want := "3,1,2,9 3 / 0,3,1,2,9 3,1,2,3,1,2 / z,a,b true,false 0 / 1,2,3 2,1,3 6,2,4 / " +
		"6 true 2 / 4 2 [3,1,2] / 5,6 / 2 x [{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"}] / 1,2 / " +
		"r x / r y / 3 / [[3,1,2],[1,2]] 3,1,2 {\"xs\":[3,1,2]} / 3 3,1,2 / 6 12 z-a-b / 3 1 / " +
		"3,1,2 6,2,4 x,y\n" +
		"[ 3, 1, 2 ]\n" +
		"[ { id: 1, tag: 'x' }, { id: 2, tag: 'y' } ]\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
