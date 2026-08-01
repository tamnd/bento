package build

import "testing"

// TestAnObjectWalkOverATypedReceiverAnswersABox is the shape this is for. A program
// parses JSON at a record type and then walks it:
//
//	const m = JSON.parse(s) as Record<string, Row>;
//	Object.values(m).map((r: Row) => r.id);
//
// The runtime walk hands back a bag of boxes, because the property values of an object
// with no compile-time shape share no Go type. That fits where the checker types the
// call any[], which is what it does for a receiver whose value type it does not know, a
// Map and a Set and a bare any all being that. Here it does know one, so it types the
// call Row[], and the two disagreed: the emitted Go put a *value.Array[value.Value]
// where the consumer asked for a *value.Array[*Row] and did not compile.
//
// So the walk's answer is the array boxed whole, and every read off it routes at run
// time. A typed array is the case that used to hand back for exactly this mismatch, its
// indices being its own properties, and it takes the box now too.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestAnObjectWalkOverATypedReceiverAnswersABox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"console.log(Object.values(m).map((r: Row) => r.id));\n"+
			"console.log(Object.values(m).length);\n"+
			"console.log(Object.values(m)[1].tag);\n"+
			"console.log(Object.values(m).filter((r: Row) => r.id > 1).length);\n"+
			"const rows = JSON.parse('[{\"id\":3,\"tag\":\"z\"}]') as Row[];\n"+
			"console.log(Object.values(rows).map((r: Row) => r.tag));\n"+
			"const nums = JSON.parse('{\"a\":1,\"b\":2}') as Record<string, number>;\n"+
			"console.log(Object.values(nums));\n"+
			"console.log(Object.values(nums).reduce((a: number, b: number) => a + b, 0));\n"+
			"const u8 = new Uint8Array([1, 2, 3]);\n"+
			"console.log(Object.values(u8));\n"+
			"console.log(Object.entries(m).length);\n"+
			"for (const r of Object.values(m)) { console.log(r.id, r.tag); }\n")
	want := "[ 1, 2 ]\n2\ny\n1\n[ 'z' ]\n[ 1, 2 ]\n3\n[ 1, 2, 3 ]\n2\n1 x\n2 y\n"
	if got != want {
		t.Errorf("an object walk over a typed receiver read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestReadsOffAnObjectWalkRouteAtRunTime covers what a program does with the walk once
// it has it, which is where the wrong answer is Go that does not compile rather than a
// hand-back.
//
// An Object.entries pair is the sharp one. The callback holds a box, and the checker
// calls it a [string, Row], so e[0] took the tuple path and selected the field E0 off a
// value.Value. A tuple's positions are only fields when the tuple is a Go struct, and
// this one is not.
//
// The reduce is the other direction. The call dispatches through the runtime and hands
// back a box whatever the checker calls the result, so a chain does not stop being boxed
// at a call the way it stops at a read: a read the checker types number is coerced down
// to a float64 at the read itself, a call is not.
//
// Held against what Node v24.18.0 prints.
func TestReadsOffAnObjectWalkRouteAtRunTime(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const m = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"},\"b\":{\"id\":2,\"tag\":\"y\"}}') as Record<string, Row>;\n"+
			"console.log(Object.entries(m).map((e: [string, Row]) => e[0] + e[1].id));\n"+
			"console.log(Object.entries(m).length, Object.entries(m)[0][0]);\n"+
			"console.log(Object.values(m).map((r: Row) => r.tag.toUpperCase()).join('-'));\n"+
			"console.log(Object.values(m).some((r: Row) => r.id === 2));\n"+
			"const nested = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"}}') as Record<string, Row>;\n"+
			"console.log(Object.values(nested).map((r: Row) => Object.values(m).filter((q: Row) => q.id >= r.id).length));\n"+
			"console.log(Object.keys(m).map((k: string) => k + m[k].id));\n")
	want := "[ 'a1', 'b2' ]\n2 a\nX-Y\ntrue\n[ 2 ]\n[ 'a1', 'b2' ]\n"
	if got != want {
		t.Errorf("a read off an object walk read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
