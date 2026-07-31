package build

import "testing"

// An optional, the T | undefined a keyed read of a collection answers, boxes into a
// dynamic value. That is what stands between a program and reading a Map or a Set back
// by key rather than by walking it, since map.get(k) is typed V | undefined. These build
// real binaries and hold their whole output against what Node v24.18.0 prints.

// TestAnOptionalReadPrintsLikeNode is the capability this is for. Every line here used
// to be wrong: the class instance and the typed array printed through a string coercion,
// which gives [object Object] and "1,2", and the date printed its toString form rather
// than the ISO one the console uses.
func TestAnOptionalReadPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; y = 's'; }\n"+
			"const m = new Map<string, P>();\n"+
			"m.set('k', new P());\n"+
			"console.log(m.get('k'));\n"+
			"console.log(m.get('z'));\n"+
			"const dm = new Map<string, Date>();\n"+
			"dm.set('d', new Date(0));\n"+
			"console.log(dm.get('d'));\n"+
			"console.log(dm.get('q'));\n"+
			"const nm = new Map<string, number>();\n"+
			"nm.set('n', 3);\n"+
			"console.log(nm.get('n'), nm.get('q'));\n"+
			"const tm = new Map<string, Uint8Array>();\n"+
			"tm.set('t', new Uint8Array([1, 2]));\n"+
			"console.log(tm.get('t'));\n"+
			"const arr = [1];\n"+
			"console.log(arr.at(0), arr.at(5));\n"+
			"const sm = new Map<string, string>();\n"+
			"sm.set('s', 'a');\n"+
			"console.log(sm.get('s'), sm.get('q'));\n")
	want := "P { x: 1, y: 's' }\n" +
		"undefined\n" +
		"1970-01-01T00:00:00.000Z\n" +
		"undefined\n" +
		"3 undefined\n" +
		"Uint8Array(2) [ 1, 2 ]\n" +
		"1 undefined\n" +
		"a undefined\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAnOptionalFromAnArrayReadsBack covers the other family of optional reads. find
// answers undefined when nothing matches, and pop and shift answer it on an empty array,
// so each is a T | undefined the console has to render arm by arm.
func TestAnOptionalFromAnArrayReadsBack(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; y = 's'; }\n"+
			"const ps = [new P(), new P()];\n"+
			"ps[1].x = 7;\n"+
			"console.log(ps.find((p) => p.x === 7));\n"+
			"console.log(ps.find((p) => p.x === 99));\n"+
			"console.log(ps.pop());\n"+
			"console.log(ps.shift());\n"+
			"console.log(ps.pop());\n"+
			"const dates = [new Date(0), new Date(86400000)];\n"+
			"console.log(dates.at(-1));\n"+
			"console.log(dates.at(9));\n"+
			"const bools = [true];\n"+
			"console.log(bools.at(0), bools.at(1));\n"+
			"const bufs = [new ArrayBuffer(2)];\n"+
			"console.log(bufs.at(0));\n"+
			"console.log([new P()].at(0), 'tail');\n")
	want := "P { x: 7, y: 's' }\n" +
		"undefined\n" +
		"P { x: 7, y: 's' }\n" +
		"P { x: 1, y: 's' }\n" +
		"undefined\n" +
		"1970-01-02T00:00:00.000Z\n" +
		"undefined\n" +
		"true undefined\n" +
		"ArrayBuffer { [Uint8Contents]: <00 00>, [byteLength]: 2 }\n" +
		"P { x: 1, y: 's' } tail\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAnOptionalInstanceStaysAView holds the thing the box has to preserve. A boxed
// class instance is a view of the instance, and threading it through the optional's box
// must not flatten it back into a copy: the write through the dynamic alias lands in the
// instance the map holds, so the next read of the map shows it.
func TestAnOptionalInstanceStaysAView(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const assert = require('node:assert');\n"+
			"class P { x = 1; y = 's'; }\n"+
			"const m = new Map<string, P>();\n"+
			"m.set('k', new P());\n"+
			"const d: any = m.get('k');\n"+
			"console.log(d);\n"+
			"console.log(d.x);\n"+
			"d.x = 8;\n"+
			"console.log(m.get('k'));\n"+
			"const gone: any = m.get('z');\n"+
			"console.log(gone, typeof gone);\n"+
			"console.log('got %s and %o', m.get('k'), m.get('k'));\n"+
			"console.log('two', m.get('k'), m.get('z'));\n"+
			"assert.deepStrictEqual(m.get('k'), m.get('k'));\n"+
			"console.log('optional deep equal');\n"+
			"assert.strictEqual(m.get('z'), undefined);\n"+
			"console.log('absent is undefined');\n"+
			"const dm = new Map<string, Date>();\n"+
			"dm.set('d', new Date(0));\n"+
			"const dd: any = dm.get('d');\n"+
			"console.log(dd, dd.getTime());\n"+
			"const tm = new Map<string, Uint8Array>();\n"+
			"tm.set('t', new Uint8Array([1, 2, 3]));\n"+
			"const td: any = tm.get('t');\n"+
			"console.log(td, td.length, td[1]);\n")
	want := "P { x: 1, y: 's' }\n" +
		"1\n" +
		"P { x: 8, y: 's' }\n" +
		"undefined undefined\n" +
		"got P { x: 8, y: 's' } and P { x: 8, y: 's' }\n" +
		"two P { x: 8, y: 's' } undefined\n" +
		"optional deep equal\n" +
		"absent is undefined\n" +
		"1970-01-01T00:00:00.000Z 0\n" +
		"Uint8Array(3) [ 1, 2, 3 ] 3 2\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAnOptionalReadsThroughEverySink walks the other places an optional read lands.
// util.inspect is the console's own renderer called directly; a template literal and
// String() are string coercions, where Node itself prints [object Object] and the two
// paths agree; and a literal holding one boxes it member by member, which is the same
// box the console argument takes.
func TestAnOptionalReadsThroughEverySink(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"const util = require('node:util');\n"+
			"class P { x = 1; y = 's'; }\n"+
			"const m = new Map<string, P>();\n"+
			"m.set('k', new P());\n"+
			"console.log(util.inspect(m.get('k')));\n"+
			"console.log(util.inspect(m.get('z')));\n"+
			"console.log(`${m.get('k')}`);\n"+
			"console.log(String(m.get('z')));\n"+
			"console.log([m.get('k')]);\n"+
			"console.log({ p: m.get('k') });\n")
	want := "P { x: 1, y: 's' }\n" +
		"undefined\n" +
		"[object Object]\n" +
		"undefined\n" +
		"[ P { x: 1, y: 's' } ]\n" +
		"{ p: P { x: 1, y: 's' } }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestJSONStringifyOfAnOptionalSerializesItsArm holds the other sink an optional read
// flows into. The reflection walk would otherwise reach the value.Opt struct itself,
// whose two fields are unexported, and write it as an empty object; the absent case is
// the value undefined rather than a string, which is why the call answers a box.
func TestJSONStringifyOfAnOptionalSerializesItsArm(t *testing.T) {
	got := buildAndRunFile(t, "main.ts",
		"class P { x = 1; y = 's'; }\n"+
			"const m = new Map<string, P>();\n"+
			"m.set('k', new P());\n"+
			"console.log(JSON.stringify(m.get('k')));\n"+
			"console.log(JSON.stringify(m.get('z')));\n"+
			"const tm = new Map<string, Uint8Array>();\n"+
			"tm.set('t', new Uint8Array([1, 2, 3]));\n"+
			"console.log(JSON.stringify(tm.get('t')));\n"+
			"console.log(JSON.stringify(tm.get('q')));\n"+
			"const nm = new Map<string, number>();\n"+
			"nm.set('n', 3);\n"+
			"console.log(JSON.stringify(nm.get('n')), JSON.stringify(nm.get('q')));\n"+
			"const sm = new Map<string, string>();\n"+
			"sm.set('s', 'a');\n"+
			"console.log(JSON.stringify(sm.get('s')));\n")
	want := "{\"x\":1,\"y\":\"s\"}\n" +
		"undefined\n" +
		"{\"0\":1,\"1\":2,\"2\":3}\n" +
		"undefined\n" +
		"3 undefined\n" +
		"\"a\"\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
