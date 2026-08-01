package build

import "testing"

// TestADeclaredSignatureTakesABox covers the shape note 384 ended on. A program walks
// parsed JSON and hands a piece of it to a function it declared:
//
//	function pick(r: Row): number { return r.id }
//	pick(Object.values(m)[0]);
//
// The walk hands back a box and the parameter's Go slot was the struct the checker
// interned for Row, which a box has no fields for. The signature gives way rather than
// the value: the parameter takes a value.Value slot, its body reads the name through the
// value model, and every call site boxes what it passes.
//
// That has to be one decision about the function taken from all its call sites at once,
// because a Go function has one signature, so the static literal call in the first line
// below boxes on the way in and lands in the same slot the boxed call does.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestADeclaredSignatureTakesABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function pick(r: Row): number { return r.id; }\n"+
			"const label = (r: Row): string => `${r.tag}#${r.id}`;\n"+
			"function join(a: Row, b: Row): string { return a.tag + b.tag; }\n"+
			"function inner(r: Row): string { return r.tag; }\n"+
			"function outer(r: Row): string { return inner(r) + '!'; }\n"+
			"function count(rows: Row[]): number { let s = 0; for (const r of rows) { s += r.id; } return s; }\n"+
			"function spell({ id, tag }: Row): string { return tag + id; }\n"+
			"function maybe(r?: Row): string { return r ? r.tag : 'none'; }\n"+
			"function withDefault(r: Row = { id: 0, tag: 'd' }): string { return r.tag + r.id; }\n"+
			"function repeat(r: Row, n: number): string { if (n === 0) return r.tag; return repeat(r, n - 1) + '.'; }\n"+
			"function describe(r: Row): string {\n"+
			"  const own = () => r.tag.toUpperCase();\n"+
			"  return own() + typeof r + (r.id === 1) + JSON.stringify({ ...r, k: true });\n"+
			"}\n"+
			"function swap(r: Row): string { r = { id: 9, tag: 'z' }; return r.tag + r.id; }\n"+
			"const first = Object.values(m)[0];\n"+
			"console.log(pick(first), pick(Object.values(m)[1]), pick({ id: 7, tag: 'q' }));\n"+
			"console.log(label(first), label({ id: 8, tag: 'w' }));\n"+
			"console.log(join(first, Object.values(m)[1]));\n"+
			"console.log(outer(first));\n"+
			"console.log(count(Object.values(m)));\n"+
			"console.log(spell(first));\n"+
			"console.log(maybe(first), maybe());\n"+
			"console.log(withDefault(first), withDefault());\n"+
			"console.log(repeat(first, 3));\n"+
			"console.log(describe(first));\n"+
			"console.log(swap(first), first.tag);\n")
	want := "1 2 7\n" +
		"x#1 w#8\n" +
		"xy\n" +
		"x!\n" +
		"3\n" +
		"x1\n" +
		"x none\n" +
		"x1 d0\n" +
		"x...\n" +
		"Xobjecttrue{\"id\":1,\"tag\":\"x\",\"k\":true}\n" +
		"z9 x\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}

// TestAFunctionThatAnswersABox is the return half of the same rule. A helper that pulls
// a piece out of parsed JSON:
//
//	function head(): Row { return Object.values(m)[0] }
//	head().tag;
//
// declares a shape it cannot build, since what every return actually holds is a box. So
// its Go result is a value.Value and the call is itself a box, which is what makes the
// read after it dispatch.
//
// Identity is the property this buys over a conversion: head() === head() is true here
// because both calls hand back the one object the parse built, which a struct copied out
// of the box at the return would have lost.
//
// Held against what Node v24.18.0 prints.
func TestAFunctionThatAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"function head(): Row { return Object.values(m)[0]; }\n"+
			"function pickBy(key: string): Row { return m[key]; }\n"+
			"function relay(): Row { return head(); }\n"+
			"const h = head();\n"+
			"console.log(h.id, h.tag);\n"+
			"console.log(head().tag, head().id + 1);\n"+
			"console.log(head().tag.toUpperCase());\n"+
			"console.log(pickBy('b').tag, pickBy('b').id);\n"+
			"console.log(relay().tag);\n"+
			"console.log(JSON.stringify({ ...head(), k: 1 }));\n"+
			"console.log(JSON.stringify(head()));\n"+
			"console.log(typeof h, head() === head());\n"+
			"const rows = [head(), pickBy('b')];\n"+
			"console.log(rows.map((r) => r.tag).join(','));\n"+
			"console.log(head());\n")
	want := "1 x\n" +
		"x 2\n" +
		"X\n" +
		"y 2\n" +
		"x\n" +
		"{\"id\":1,\"tag\":\"x\",\"k\":1}\n" +
		"{\"id\":1,\"tag\":\"x\"}\n" +
		"object true\n" +
		"x,y\n" +
		"{ id: 1, tag: 'x' }\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
