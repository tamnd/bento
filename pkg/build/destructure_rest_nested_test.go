package build

import "testing"

// TestObjectRestInsideANestedPattern covers an object rest wherever a sub-pattern binds:
//
//	const { o: { a, ...inner } } = deep
//	for (const { a, ...r } of rows)
//
// Both bind a pattern against a receiver that already holds the value, through
// bindSubObject, which had no rest arm. So a rest element there fell to
// classifyObjectElem and handed back with a reason that was true of the whole capability
// once and is no longer: a top-level `const { a, ...rest } = o` already gathers a
// fixed-shape rest into its own interned struct and an empty one into the runtime object.
// The array sibling was already there, since a nested array pattern splits its trailing
// rest and copies the tail with Slice.
//
// The gather now lives in one helper both levels call, and the nested object paths, the
// declaration and the assignment, have their rest arm.
//
// The positions are here: a rest one level down, two levels down, inside an array
// pattern's element, in a for...of head, in a for...of head under a further nesting, and
// in the nested assignment form. So are the neighbors: a rename, a property of the outer
// object beside the nesting, and a nested array rest that still copies its tail. So are
// the kinds, const and let with a reassignment, and the scopes: the module, a function
// body, a destructured parameter, a class method, and a leaf a function reads, which is
// the shape that hoists. A rest with nothing left in it is here in both the declaration
// and the loop, since the nested gather answers the empty object type the same way the
// top-level one does. So is a rest nothing reads.
//
// Held against what Node v24.18.0 prints, one program.
func TestObjectRestInsideANestedPattern(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const deep = { o: { a: 1, b: 2, c: 3 } };\n"+
			"const lone = { o: { a: 1 } };\n"+
			"const three = { p: { q: { a: 1, b: 2, c: 3 } } };\n"+
			"const beside = { o: { a: 1, b: 2 }, z: 5 };\n"+
			"const arrs = { o: { xs: [1, 2, 3] } };\n"+
			"const rows = [{ a: 1, b: 2, c: 3 }, { a: 4, b: 5, c: 6 }];\n"+
			"const bare = [{ a: 1 }, { a: 2 }];\n"+
			"const wrapped = [{ o: { a: 1, b: 2 } }, { o: { a: 3, b: 4 } }];\n"+
			"const inArr = [{ a: 7, b: 8, c: 9 }];\n"+
			"\n"+
			"const { o: { a: da, ...drest } } = deep;\n"+
			"const { o: { a: la, ...lrest } } = lone;\n"+
			"const { p: { q: { a: ta, ...trest } } } = three;\n"+
			"const { z: bz, o: { a: ba, ...brest } } = beside;\n"+
			"const { o: { xs: [xh, ...xt] } } = arrs;\n"+
			"const [{ a: ia, ...irest }] = inArr;\n"+
			"let { o: { a: ma, ...mrest } } = deep;\n"+
			"ma = 9;\n"+
			"\n"+
			"let sa = 0;\n"+
			"let srest: { b: number; c: number } = { b: 0, c: 0 };\n"+
			"({ o: { a: sa, ...srest } } = deep);\n"+
			"\n"+
			"function param({ o: { a, ...rest } }: { o: { a: number; b: number; c: number } }): string {\n"+
			"  return `${a}${JSON.stringify(rest)}`;\n"+
			"}\n"+
			"function hoisted(): string { return `${da}${drest.b + drest.c}`; }\n"+
			"function dropped(): number { const { o: { a, ...unread } } = deep; return a; }\n"+
			"class C {\n"+
			"  m(d: { o: { a: number; b: number } }): string { const { o: { a, ...r } } = d; return `${a}${JSON.stringify(r)}`; }\n"+
			"}\n"+
			"\n"+
			"console.log(da, JSON.stringify(drest), la, JSON.stringify(lrest), Object.keys(lrest).length);\n"+
			"console.log(ta, JSON.stringify(trest), bz, ba, JSON.stringify(brest));\n"+
			"console.log(xh, xt.join(','), ia, JSON.stringify(irest), ma, mrest.b + mrest.c);\n"+
			"console.log(sa, JSON.stringify(srest), param({ o: { a: 1, b: 2, c: 3 } }), hoisted(), dropped(), new C().m({ o: { a: 1, b: 2 } }));\n"+
			"for (const { a, ...r } of rows) { console.log(a, JSON.stringify(r), r.b + r.c); }\n"+
			"for (const { a, ...r } of bare) { console.log(a, JSON.stringify(r), Object.keys(r).length); }\n"+
			"for (const { o: { a, ...r } } of wrapped) { console.log(a, JSON.stringify(r)); }\n")
	want := "1 {\"b\":2,\"c\":3} 1 {} 0\n" +
		"1 {\"b\":2,\"c\":3} 5 1 {\"b\":2}\n" +
		"1 2,3 7 {\"b\":8,\"c\":9} 9 5\n" +
		"1 {\"b\":2,\"c\":3} 1{\"b\":2,\"c\":3} 15 1 1{\"b\":2}\n" +
		"1 {\"b\":2,\"c\":3} 5\n" +
		"4 {\"b\":5,\"c\":6} 11\n" +
		"1 {} 0\n" +
		"2 {} 0\n" +
		"1 {\"b\":2}\n" +
		"3 {\"b\":4}\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
