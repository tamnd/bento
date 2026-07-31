package build

import "testing"

// A typed array that crosses into a dynamic slot is a run-time surface rather than an
// emission, so what matters is what the built binary prints when a program logs one,
// walks its keys, serializes it, or reads one back out of a container. These build real
// binaries and hold their whole output against what Node v24.18.0 prints for the same
// program.

// TestABoxedTypedArrayPrintsLikeNode covers the rendering across the family. Every kind
// names itself and its length before its elements, which is the one thing that separates
// a typed array's printed form from a plain array's, and an empty one still names both.
func TestABoxedTypedArrayPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"console.log(new Int32Array([5, 0, 7]));\n"+
			"console.log(new Uint8Array([1, 2]));\n"+
			"console.log(new Float64Array([1.5, -0]));\n"+
			"console.log(new Uint8ClampedArray([300, 2]));\n"+
			"console.log(new BigInt64Array([1n, -2n]));\n"+
			"console.log(new Int32Array(0));\n"+
			"console.log({ v: new Int32Array([1, 2]) });\n")
	want := "Int32Array(3) [ 5, 0, 7 ]\n" +
		"Uint8Array(2) [ 1, 2 ]\n" +
		"Float64Array(2) [ 1.5, -0 ]\n" +
		"Uint8ClampedArray(2) [ 255, 2 ]\n" +
		"BigInt64Array(2) [ 1n, -2n ]\n" +
		"Int32Array(0) []\n" +
		"{ v: Int32Array(2) [ 1, 2 ] }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayReadsAndWritesItsElements covers the seam the box exists for. The
// box is a view of the same elements the static array holds, so an index written before
// the log shows in what is printed, and the four geometry members read off the box rather
// than off a copy of it.
func TestABoxedTypedArrayReadsAndWritesItsElements(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = new Int32Array([5, 0, 7]);\n"+
			"console.log(a.length, a.byteLength, a.byteOffset, a.BYTES_PER_ELEMENT);\n"+
			"console.log(a[0], a[2]);\n"+
			"a[1] = 4;\n"+
			"console.log(a);\n"+
			"console.log(String(a));\n"+
			"const buf = new ArrayBuffer(16);\n"+
			"const v = new Int32Array(buf, 4, 2);\n"+
			"v[0] = 9;\n"+
			"console.log(v, v.byteOffset, v.buffer === buf);\n")
	want := "3 12 0 4\n" +
		"5 7\n" +
		"Int32Array(3) [ 5, 4, 7 ]\n" +
		"5,4,7\n" +
		"Int32Array(2) [ 9, 0 ] 4 true\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayWalksItsIndices covers the one way a typed array's box differs in
// kind from every other box on this wall: its indices are its own properties, so the key
// walks and the JSON serializers see them where an empty property table would have given
// back nothing.
func TestABoxedTypedArrayWalksItsIndices(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = new Int32Array([5, 0, 7]);\n"+
			"console.log(Object.keys(a));\n"+
			"console.log(JSON.stringify(a));\n"+
			"console.log(JSON.stringify({ a: new Uint8Array([1, 2]) }, null, 2));\n"+
			"console.log(JSON.stringify([new Int32Array([1, 2])]));\n")
	want := "[ '0', '1', '2' ]\n" +
		"{\"0\":5,\"1\":0,\"2\":7}\n" +
		"{\n  \"a\": {\n    \"0\": 1,\n    \"1\": 2\n  }\n}\n" +
		"[{\"0\":1,\"1\":2}]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayCarriesItsPrototype covers the member surface a program reaches
// through the box. The members are written once against the backing rather than per kind,
// so the ones that hand back another array hand back the receiver's own kind, and the
// callbacks take the (value, index, array) triple the language passes.
func TestABoxedTypedArrayCarriesItsPrototype(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = new Int32Array([5, 0, 7, 3]);\n"+
			"console.log(a.at(-1), a.at(9), a.indexOf(7), a.includes(0), a.join('-'));\n"+
			"console.log(a.slice(1, 3), a.subarray(1, 3));\n"+
			"console.log(a.map(x => x * 2));\n"+
			"console.log(a.filter(x => x > 3));\n"+
			"console.log(a.reduce((p, c) => p + c, 0));\n"+
			"console.log(a.every(x => x >= 0), a.some(x => x === 7), a.find(x => x > 4), a.findIndex(x => x > 4));\n")
	want := "3 undefined 2 true 5-0-7-3\n" +
		"Int32Array(2) [ 0, 7 ] Int32Array(2) [ 0, 7 ]\n" +
		"Int32Array(4) [ 10, 0, 14, 6 ]\n" +
		"Int32Array(2) [ 5, 7 ]\n" +
		"15\n" +
		"true true 5 0\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayMutatesInPlace covers the members that write back into the receiver
// rather than answer a new array, along with the two that copy instead. The default sort
// is numeric, which is where a typed array parts from a plain array, and copyWithin works
// from a snapshot so an overlapping copy does not smear.
func TestABoxedTypedArrayMutatesInPlace(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new Int32Array(4);\n"+
			"b.set([1, 2], 1);\n"+
			"console.log(b);\n"+
			"b.fill(7, 0, 2);\n"+
			"console.log(b);\n"+
			"console.log(new Int32Array([1, 2, 3, 4, 5]).copyWithin(0, 3));\n"+
			"console.log(new Int32Array([10, 9, 2]).sort());\n"+
			"console.log(new Int32Array([1, 2, 3]).with(1, 9));\n"+
			"console.log(new Int32Array([1, 2, 3]).toReversed());\n")
	want := "Int32Array(4) [ 0, 1, 2, 0 ]\n" +
		"Int32Array(4) [ 7, 7, 2, 0 ]\n" +
		"Int32Array(5) [ 4, 5, 3, 4, 5 ]\n" +
		"Int32Array(3) [ 2, 9, 10 ]\n" +
		"Int32Array(3) [ 1, 9, 3 ]\n" +
		"Int32Array(3) [ 3, 2, 1 ]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayIteratesAndNests covers the iterator, which is what a spread and a
// for...of walk, and the element bridge: a container of typed arrays presents each member
// through the array's own box when it crosses, so what a Map prints is the array it holds
// rather than an empty object.
func TestABoxedTypedArrayIteratesAndNests(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"for (const x of new Int32Array([1, 2])) console.log(x);\n"+
			"const m = new Map();\n"+
			"m.set('k', new Int32Array([1]));\n"+
			"console.log(m);\n"+
			"console.log(new Set([new Uint8Array([1])]));\n")
	want := "1\n2\n" +
		"Map(1) { 'k' => Int32Array(1) [ 1 ] }\n" +
		"Set(1) { Uint8Array(1) [ 1 ] }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayComparesByItsBytes covers what assert's deep comparison sees, which
// Node holds against the raw bytes rather than the numbers. That is what makes two arrays
// of the same NaN equal and the two zeros not, and the kind is tested before the bytes are
// ever read.
func TestABoxedTypedArrayComparesByItsBytes(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('node:assert');\n"+
			"assert.deepStrictEqual(new Int32Array([1, 2]), new Int32Array([1, 2]));\n"+
			"assert.notDeepStrictEqual(new Int32Array([1, 2]), new Uint32Array([1, 2]));\n"+
			"assert.notDeepStrictEqual(new Float64Array([0]), new Float64Array([-0]));\n"+
			"assert.deepStrictEqual(new Float64Array([NaN]), new Float64Array([NaN]));\n"+
			"console.log('deep equal ok');\n")
	if want := "deep equal ok\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedTypedArrayShowsItsSlots covers the showHidden form, where the five members
// that live on the prototype are written out as bracketed slots. The buffer under it
// prints its length alone rather than its bytes, since the array above has already shown
// what those bytes hold.
func TestABoxedTypedArrayShowsItsSlots(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const util = require('node:util');\n"+
			"console.log(util.inspect(new Int32Array([1, 2]), { showHidden: true }));\n")
	want := "Int32Array(2) [\n" +
		"  1,\n" +
		"  2,\n" +
		"  [BYTES_PER_ELEMENT]: 4,\n" +
		"  [length]: 2,\n" +
		"  [byteLength]: 8,\n" +
		"  [byteOffset]: 0,\n" +
		"  [buffer]: ArrayBuffer { [byteLength]: 8 }\n" +
		"]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
