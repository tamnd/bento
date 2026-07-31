package build

import "testing"

// TestAnObjectStringifiesLikeNode is the capability this is for. Before this a String()
// or a template substitution over anything but a primitive, a regexp or a date failed to
// build with "coercing this type to a string is a later slice", so a Map, a Set, an
// array, a plain shape and a class instance had no string form at all. This builds a
// real binary and holds its whole output against what Node v24.18.0 prints for the same
// program.
//
// The lines worth reading twice are the class ones. String(q) runs the class's own
// toString because the string hint asks toString first, while 'v' + v runs valueOf
// because + asks with the default hint, which is the whole reason the same instance
// reads two ways. An instance with neither method reads as the tag, and an inherited
// toString comes along the way any inherited method does.
func TestAnObjectStringifiesLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class Q { y = 2; toString() { return 'Q!'; } }\n"+
			"class R extends Q { toString() { return 'R!'; } }\n"+
			"class S extends Q { z = 3; }\n"+
			"class P { x = 1; }\n"+
			"class V { v = 1; valueOf() { return 7; } }\n"+
			"const q = new Q(), r = new R(), sub = new S(), p = new P(), v = new V();\n"+
			"const base: Q = r;\n"+
			"const m = new Map<string, number>();\n"+
			"m.set('a', 1);\n"+
			"const st = new Set<number>();\n"+
			"st.add(1);\n"+
			"const wm = new WeakMap<P, number>();\n"+
			"const nums: number[] = [1, 2];\n"+
			"const strs: string[] = ['a', 'b'];\n"+
			"const empty: number[] = [];\n"+
			"const nested: number[][] = [[1, 2], [3]];\n"+
			"const shape = { a: 1, b: 'z' };\n"+
			"const ps: P[] = [p, new P()];\n"+
			"console.log(String(m), String(st), String(wm));\n"+
			"console.log(String(p), `${p}`, String(shape));\n"+
			"console.log(String(nums), String(strs), String(empty), String(nested));\n"+
			"console.log(String(q), String(r), String(sub), String(base), `${q}`);\n"+
			"console.log('x' + m, 'y' + p, 'z' + nums);\n"+
			"console.log('q' + q, 'v' + v, String(v), q + '', v + '');\n"+
			"console.log(ps.join(','));\n"+
			"console.log(String(ps));\n"+
			"console.log([m].join('|'));\n"+
			"console.log(String(m).length, String(nums).length);\n"+
			"console.log(String(q).toUpperCase());\n")
	want := "[object Map] [object Set] [object WeakMap]\n" +
		"[object Object] [object Object] [object Object]\n" +
		"1,2 a,b  1,2,3\n" +
		"Q! R! Q! R! Q!\n" +
		"x[object Map] y[object Object] z1,2\n" +
		"qQ! v7 [object Object] Q! 7\n" +
		"[object Object],[object Object]\n" +
		"[object Object],[object Object]\n" +
		"[object Map]\n" +
		"12 3\n" +
		"Q!\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
