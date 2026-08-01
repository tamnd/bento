package build

import "testing"

// TestABoxedCallbackParameterHoldsItsBox is the everyday shape this is for. A program
// parses JSON at an asserted type and then walks the result:
//
//	const rows = JSON.parse(s) as Row[];
//	rows.map((r: Row) => r.id);
//
// The receiver is a box, so the call dispatches through the runtime, which hands the
// callback its arguments already boxed. The parameter's declared type is a shape, and
// there is no way to put a box in a Go struct, so the whole thing handed back. The
// parameter holds the box instead and its body reads it the way every other read off a
// box goes, which is what the argument actually is.
//
// A primitive parameter is untouched: rows.map((x: number) => x * 2) already worked,
// because ToNumber lands the box in that slot.
//
// Held against what Node v24.18.0 prints, one program so the ordering is pinned too.
func TestABoxedCallbackParameterHoldsItsBox(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"a\"},{\"id\":2,\"tag\":\"b\"}]') as Row[];\n"+
			"console.log(rows.map((r: Row) => r.id));\n"+
			"console.log(rows.filter((r: Row) => r.id > 1).length);\n"+
			"rows.forEach((r: Row, i: number) => console.log(i, r.tag));\n"+
			"console.log(rows.reduce((a: number, r: Row) => a + r.id, 0));\n"+
			"console.log(rows.find((r: Row) => r.id === 2)?.tag);\n"+
			"console.log(rows.some((r: Row) => r.id === 2), rows.every((r: Row) => r.id > 0));\n")
	want := "[ 1, 2 ]\n1\n0 a\n1 b\n3\nb\ntrue true\n"
	if got != want {
		t.Errorf("a boxed callback parameter read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestReadsInsideABoxedCallbackRouteAtRunTime covers the bodies, which is where the
// wrong answer would have been Go that does not compile rather than a hand-back.
//
// A nested callback is the sharp one. The closure the outer callback builds captures
// rows, which is a box, and the set of boxed names was rebuilt from the inner
// function's own parameters when its body lowered, so the inner read of rows found
// nothing and emitted the static Filter on a value.Value. Capture does not change what
// a name holds, so the enclosing body's boxed names come along now.
//
// r.tag is the other direction. It is a read off a box, but the checker types it string
// and the read coerces down to a value.BStr at the read itself, so .toUpperCase() is
// the static string method. A chain stops being boxed at the first link the checker
// calls a primitive.
//
// Held against what Node v24.18.0 prints.
func TestReadsInsideABoxedCallbackRouteAtRunTime(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"a\"},{\"id\":2,\"tag\":\"b\"}]') as Row[];\n"+
			"console.log(rows.map((r: Row) => r.tag.toUpperCase()));\n"+
			"console.log(rows.map((r: Row) => { const n = r.id * 2; return n; }));\n"+
			"console.log(rows.map((r: Row) => rows.filter((q: Row) => q.id >= r.id).length));\n"+
			"console.log(rows.filter((r: Row) => r.id > 0).map((r: Row) => r.tag).join(','));\n"+
			"rows.sort((a: Row, b: Row) => b.id - a.id);\n"+
			"console.log(rows[0].id);\n")
	want := "[ 'A', 'B' ]\n[ 2, 4 ]\n[ 2, 1 ]\na,b\n2\n"
	if got != want {
		t.Errorf("a read inside a boxed callback read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestABoxedCallbackReturnsWhatItLikes covers the other side of the wrapper. Whatever
// the callback returns has to be boxed back for the runtime call to hand on, and only
// the primitives had a box picked by type flags alone. A shape and an array each have
// one, they just need the type rather than the flags to name it, and those two are what
// a callback returns in practice.
//
// Held against what Node v24.18.0 prints, including the inspector's nested forms.
func TestABoxedCallbackReturnsWhatItLikes(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"type Row = { id: number; tag: string };\n"+
			"const rows = JSON.parse('[{\"id\":1,\"tag\":\"a\"},{\"id\":2,\"tag\":\"b\"}]') as Row[];\n"+
			"console.log(rows.map((r: Row) => ({ v: r.id })));\n"+
			"console.log(rows.flatMap((r: Row) => [r.id, r.id]));\n")
	want := "[ { v: 1 }, { v: 2 } ]\n[ 1, 1, 2, 2 ]\n"
	if got != want {
		t.Errorf("a boxed callback's result read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
