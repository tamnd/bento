package build

import "testing"

// TestAHoleInAnArrayPattern covers the position an array pattern skips by writing nothing
// between two commas:
//
//	const [, b] = arr
//	const [a, , c] = arr
//
// A hole names nothing and reads nothing. It exists only to hold its place, which is what
// makes the elements after it select the indices they do. Every array pattern classified
// its elements one at a time and refused an element that held no binding, so a hole
// anywhere in a pattern handed the whole unit back. The frontend already keeps a hole as
// an element of its own, sitting in the position it skips, so the indices were right all
// along; what was missing was the classification saying so and each caller stepping over
// it.
//
// The positions are here: leading, in the middle, two in a row, and before a rest, which
// starts after the hole counts toward the fixed slots. So are the sources: an array, a
// tuple reading named fields rather than indices, a nested pattern, and a pattern inside
// an object pattern's property. So are the neighbors: a default after a hole, both the
// present and the missing slot. So are the forms, the declaration and the assignment,
// which stores through one parallel Go assignment and so keeps a slot on both sides with
// the read going to the blank, with and without a rest beside it. So are the scopes: the
// module, a function body, a destructured parameter, an arrow, a class method, a for...of
// head over arrays, over tuples and under a further nesting, and a leaf a function reads,
// which is the shape that hoists. A pattern of nothing but holes is here too, since it
// binds no name yet still reads its source.
//
// Held against what Node v24.18.0 prints, one program.
func TestAHoleInAnArrayPattern(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const arr = [1, 2, 3, 4];\n"+
			"const t: [number, string, boolean] = [1, \"two\", true];\n"+
			"const rows: number[][] = [[1, 2, 3], [4, 5, 6]];\n"+
			"const pairs: [string, number][] = [[\"a\", 1], [\"b\", 2]];\n"+
			"const grid: number[][] = [[1, 2], [3, 4]];\n"+
			"const obj = { v: [1, 2, 3] };\n"+
			"const objs: Array<{ k: string; v: number[] }> = [{ k: \"x\", v: [5, 6] }];\n"+
			"\n"+
			"const [, one] = arr;\n"+
			"const [two, , four] = arr;\n"+
			"const [, , , last] = arr;\n"+
			"const [, ...tail] = arr;\n"+
			"const [, , ...shortTail] = arr;\n"+
			"const [, def = 9] = arr;\n"+
			"const [, , , , missing = 7] = arr;\n"+
			"const [, ts] = t;\n"+
			"const [tn, , tb] = t;\n"+
			"const [, [, deep]] = grid;\n"+
			"const { v: [, inObj] } = obj;\n"+
			"const [,] = arr;\n"+
			"let mut = 0;\n"+
			"[, mut] = arr;\n"+
			"let mrest: number[] = [];\n"+
			"let mfirst = 0;\n"+
			"[, mfirst, ...mrest] = arr;\n"+
			"const ptup: [number, number] = [10, 20];\n"+
			"let ptb = 0;\n"+
			"[, ptb] = ptup;\n"+
			"\n"+
			"function second(p: number[]): number { const [, y] = p; return y; }\n"+
			"function skipTwo([, , z]: number[]): number { return z; }\n"+
			"function hoisted(): number { return one; }\n"+
			"class C {\n"+
			"  head(p: number[]): number { const [, y] = p; return y; }\n"+
			"}\n"+
			"const mapped = grid.map(([, y]) => y);\n"+
			"\n"+
			"console.log(one, two, four, last, tail.join(\",\"), shortTail.join(\",\"));\n"+
			"console.log(def, missing, ts, tn, tb, deep, inObj);\n"+
			"console.log(mut, mfirst, mrest.join(\",\"), ptb);\n"+
			"console.log(second([1, 2]), skipTwo([1, 2, 3]), hoisted(), new C().head([1, 2, 3]), mapped.join(\",\"));\n"+
			"for (const [, y] of rows) { console.log(y); }\n"+
			"for (const [x, , z] of rows) { console.log(x, z); }\n"+
			"for (const [, n] of pairs) { console.log(n); }\n"+
			"for (const { k, v: [, sec] } of objs) { console.log(k, sec); }\n")
	want := "2 1 3 4 2,3,4 3,4\n" +
		"2 7 two 1 true 4 2\n" +
		"2 2 3,4 20\n" +
		"2 3 2 2 2,4\n" +
		"2\n" +
		"5\n" +
		"1 3\n" +
		"4 6\n" +
		"1\n" +
		"2\n" +
		"x 6\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
