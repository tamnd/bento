package build

import "testing"

// TestATupleReadsAsAnArrayEverywhere is the capability this is for. A tuple is an array
// in JavaScript and a positional struct in Go, and every walk over a Go value used to
// meet the struct: JSON.stringify of an Object.entries result read
// [{"E0":"a","E1":1},{"E0":"b","E1":2}] where the engine reads [["a",1],["b",2]].
//
// That is the shape of bug worth the most care, a wrong answer rather than a refusal,
// and it was wrong at every depth, since a tuple is reached far more often inside
// something else than on its own. Object.entries is the everyday way to reach one.
//
// The fix is the generated tuple struct saying what it is, through a JSONTuple method
// the runtime's walks ask for, so one hook covers the entries pair, the object field,
// the array of tuples, the map value, and the indented form alike.
//
// This builds a real binary and holds its whole output against what Node v24.18.0 prints.
func TestATupleReadsAsAnArrayEverywhere(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const o = { a: 1, b: 2 };\n"+
			"const e = Object.entries(o);\n"+
			"console.log(JSON.stringify(e));\n"+
			"console.log(e);\n"+
			"console.log(String(e));\n"+
			"const t: [number, string] = [1, 'a'];\n"+
			"console.log(JSON.stringify({ p: t }));\n"+
			"console.log(JSON.stringify([t, t]));\n"+
			"console.log(JSON.stringify(t, null, 2));\n"+
			"console.log({ p: t });\n")
	want := "[[\"a\",1],[\"b\",2]]\n" +
		"[ [ 'a', 1 ], [ 'b', 2 ] ]\n" +
		"a,1,b,2\n" +
		"{\"p\":[1,\"a\"]}\n" +
		"[[1,\"a\"],[1,\"a\"]]\n" +
		"[\n  1,\n  \"a\"\n]\n" +
		"{ p: [ 1, 'a' ] }\n"
	if got != want {
		t.Errorf("a tuple read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestAnOptionalTupleReadsAsAnArray covers the tuple reached through a Map lookup, which
// hands back a T | undefined rather than a T. That shape had no box before this: the
// optional boxer needs a boxer for the element it wraps, and a tuple had none, so a
// perfectly ordinary Map<string, [number, string]> could not be logged at all. It has one
// now, so a present entry prints its positions and an absent one prints undefined.
//
// Held against what Node v24.18.0 prints.
func TestAnOptionalTupleReadsAsAnArray(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const outer = new Map<string, [number, string]>();\n"+
			"outer.set('a', [1, 'x']);\n"+
			"console.log(outer.get('a'));\n"+
			"console.log(outer.get('b'));\n")
	want := "[ 1, 'x' ]\nundefined\n"
	if got != want {
		t.Errorf("an optional tuple read differently than Node:\ngot:\n%s\nwant:\n%s", got, want)
	}
}
