package build

import "testing"

// TestArrayFromOverACollection covers what a Map or Set gives back when a program asks
// for its members as an array:
//
//	Array.from(nums).sort((a, b) => a - b)
//
// A spread of a collection into an array literal already spliced exactly this slice, so
// the two go through one reader: the runtime hands back an insertion-ordered snapshot of
// whichever slot the spelling names, and Array.from wraps it as a fresh array rather than
// walking the collection an element at a time. The snapshot is already a copy, so a later
// add to the collection does not reach an array collected before it.
//
// The pieces the same lowering has to carry along with it are all here: a Set's keys()
// and values() both yielding its members while a Map's yield each half, the pair-yielding
// spellings materializing the interned [key, value] tuple, a map callback running over
// the members and free to change their type, a collection whose member slot the boxed
// signature pass rewrote collecting into a boxed array every read off which still
// dispatches, and a generator draining its coroutine the same way.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestArrayFromOverACollection(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"type Cell = { n: number; label: string };\n"+
			"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"const nums = new Set<number>([3, 1, 2]);\n"+
			"const byName = new Map<string, number>([['a', 1], ['b', 2]]);\n"+
			"const rows = new Map<string, Cell>([['k', { n: 9, label: 'z' }]]);\n"+
			"const out: string[] = [];\n"+
			"out.push(`${Array.from(nums).join(',')} ${Array.from(nums).length}`);\n"+
			"out.push(`${Array.from(nums).sort((a, b) => a - b).join(',')} ${Array.from(nums).reduce((a, b) => a + b, 0)}`);\n"+
			"out.push(`${Array.from(nums.values()).join(',')} ${Array.from(nums.keys()).join(',')}`);\n"+
			"out.push(`${Array.from(byName.keys()).join(',')} ${Array.from(byName.values()).join(',')}`);\n"+
			"out.push(`${JSON.stringify(Array.from(byName))} ${JSON.stringify(Array.from(byName.entries()))}`);\n"+
			"out.push(`${JSON.stringify(Array.from(nums.entries()))} ${JSON.stringify(Array.from(rows))}`);\n"+
			"out.push(`${Array.from(nums, (x) => x * 2).join(',')} ${Array.from(nums, (x, i) => x + i).join(',')}`);\n"+
			"out.push(`${Array.from(nums, () => 'q').join(',')} ${Array.from(byName.keys(), (k) => k.toUpperCase()).join(',')}`);\n"+
			"out.push(`${Array.from(rows.values(), (c) => c.label).join(',')} ${JSON.stringify(Array.from(nums, (x) => ({ v: x })))}`);\n"+
			"const grow = new Set<number>([1, 2]);\n"+
			"const snapshot = Array.from(grow);\n"+
			"grow.add(3);\n"+
			"out.push(`${snapshot.join(',')} ${grow.size} ${Array.from(grow).length}`);\n"+
			"const boxed = new Set<Row>();\n"+
			"boxed.add(raw['a']);\n"+
			"boxed.add(raw['b']);\n"+
			"out.push(`${Array.from(boxed).map((r) => r.tag).join(',')} ${Array.from(boxed)[0].tag} ${Array.from(boxed).length}`);\n"+
			"out.push(`${JSON.stringify(Array.from(boxed))}`);\n"+
			"const boxedMap = new Map<string, Row>();\n"+
			"boxedMap.set('k', raw['b']);\n"+
			"out.push(`${Array.from(boxedMap.values()).map((r) => r.id).join(',')}`);\n"+
			"for (const r of Array.from(boxed)) {\n"+
			"  out.push(`loop ${r.tag}`);\n"+
			"}\n"+
			"function total(xs: Set<number>): number {\n"+
			"  return Array.from(xs).length;\n"+
			"}\n"+
			"function members(): number[] {\n"+
			"  return Array.from(nums);\n"+
			"}\n"+
			"class Holder {\n"+
			"  xs = new Set<number>([4, 5]);\n"+
			"  all(): string {\n"+
			"    return Array.from(this.xs).join(',');\n"+
			"  }\n"+
			"}\n"+
			"let stored: number[] = [];\n"+
			"stored = Array.from(nums);\n"+
			"out.push(`${total(nums)} ${members().join(',')} ${new Holder().all()} ${stored.join(',')}`);\n"+
			"out.push(`${[...Array.from(nums), 9].join(',')} ${JSON.stringify({ xs: Array.from(nums) })}`);\n"+
			"function* pair(): Generator<number> {\n"+
			"  yield 7;\n"+
			"  yield 8;\n"+
			"}\n"+
			"out.push(`${Array.from(pair()).join(',')} ${Array.from('ab').join(',')} ${Array.from([1, 2], (x) => x * 3).join(',')}`);\n"+
			"out.push(`${Array.from(nums) === Array.from(nums)} ${Array.from(new Set<string>()).length}`);\n"+
			"console.log(out.join(' / '));\n")
	want := "3,1,2 3 / 1,2,3 6 / 3,1,2 3,1,2 / a,b 1,2 / " +
		"[[\"a\",1],[\"b\",2]] [[\"a\",1],[\"b\",2]] / " +
		"[[3,3],[1,1],[2,2]] [[\"k\",{\"n\":9,\"label\":\"z\"}]] / " +
		"6,2,4 3,2,4 / q,q,q A,B / z [{\"v\":3},{\"v\":1},{\"v\":2}] / " +
		"1,2 3 3 / x,y x 2 / [{\"id\":1,\"tag\":\"x\"},{\"id\":2,\"tag\":\"y\"}] / 2 / " +
		"loop x / loop y / 3 3,1,2 4,5 3,1,2 / 3,1,2,9 {\"xs\":[3,1,2]} / " +
		"7,8 a,b 3,6 / false 0\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
