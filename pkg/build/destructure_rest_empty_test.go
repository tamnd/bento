package build

import "testing"

// TestObjectRestWithNothingLeftInIt covers a destructuring rest that gathers no property:
//
//	const { a: ra, ...rest } = { a: 1 }
//
// The pattern names everything the source has, so rest holds the empty object type, and
// the empty object type is the structural top type rather than a fixed shape. Everywhere
// else the lowerer holds it that way: `const e = {}` builds a value.NewObject, its slot is
// a value.Value, and a read of it goes through the runtime. The rest gather was the one
// place that did not ask, so it interned an empty struct and bound that, and the emitted
// Go did not build:
//
//	rest.OwnEnumerableKeys undefined (type *ObjEmpty has no field or method
//	OwnEnumerableKeys)
//
// A leaf a function reads showed it from the other side, since the package var the hoist
// declares is spelled by the same typeExpr the reads agree with:
//
//	cannot use &ObjEmpty{} (value of type *ObjEmpty) as value.Value value in assignment
//
// The gather now asks, and an empty one builds the same runtime object the empty literal
// builds.
//
// The forms are here: an empty rest, a rest that keeps fields, a rest with a default
// beside it, a rest off a boxed source, `{ ...all }` and `{ ...none }` with no other
// property in the pattern, a rest beside a plain name, and the assignment form. So are the
// readers, since a leaf a function reads is the shape that hoists: Object.keys, entries,
// values, JSON.stringify, typeof, a truth test, a parameter typed `{}`, a declared `{}`
// return, a static class field, and a class method with a rest of its own.
//
// Held against what Node v24.18.0 prints, one program.
func TestObjectRestWithNothingLeftInIt(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const one = { a: 1 };\n"+
			"const two = { a: 1, b: 2 };\n"+
			"const three = { a: 1, b: 2, c: 3 };\n"+
			"const raw = JSON.parse('{\"a\":1}') as { a: number };\n"+
			"const empty = {};\n"+
			"\n"+
			"const { a: ra, ...rest } = one;\n"+
			"const { a: ta, b: tb, ...gone } = two;\n"+
			"const { a: ka, ...kept } = three;\n"+
			"const { a: da, b: db = 7, ...dgone } = two;\n"+
			"const { a: ja, ...jrest } = raw;\n"+
			"const { ...all } = two;\n"+
			"const { ...none } = empty;\n"+
			"const { a: za, ...zrest } = one, z = 5;\n"+
			"\n"+
			"let ma = 0, mb = 0;\n"+
			"let mrest: {} = {};\n"+
			"({ a: ma, b: mb, ...mrest } = two);\n"+
			"\n"+
			"function readRest(): string { return `${ra}${JSON.stringify(rest)}${Object.keys(rest).length}`; }\n"+
			"function readGone(): string { return `${ta + tb} ${Object.keys(gone).length} ${typeof gone}`; }\n"+
			"function readKept(): string { return `${ka} ${JSON.stringify(kept)} ${Object.keys(kept).length}`; }\n"+
			"function take(o: {}): number { return Object.keys(o).length; }\n"+
			"function give(): {} { return dgone; }\n"+
			"class C {\n"+
			"  static st = Object.keys(gone).length;\n"+
			"  m(): string { const { a: ca, ...crest } = one; return `${ca}${Object.keys(crest).length}`; }\n"+
			"}\n"+
			"\n"+
			"console.log(readRest(), readGone(), readKept());\n"+
			"console.log(da + db, take(dgone), Object.keys(give()).length, JSON.stringify(dgone));\n"+
			"console.log(ja, JSON.stringify(jrest), Object.entries(jrest).length, Object.values(jrest).length);\n"+
			"console.log(JSON.stringify(all), JSON.stringify(none), za + Object.keys(zrest).length + z);\n"+
			"console.log(ma, mb, JSON.stringify(mrest), C.st, new C().m(), rest ? 'truthy' : 'falsy');\n")
	want := "1{}0 3 0 object 1 {\"b\":2,\"c\":3} 2\n" +
		"3 0 0 {}\n" +
		"1 {} 0 0\n" +
		"{\"a\":1,\"b\":2} {} 6\n" +
		"1 2 {} 0 10 truthy\n"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
