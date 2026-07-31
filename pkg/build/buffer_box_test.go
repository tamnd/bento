package build

import "testing"

// A byte buffer that crosses into a dynamic slot is a run-time surface rather than an
// emission, so what matters is what the built binary prints when a program logs a
// buffer, reads one back out of a container, or writes through a view of one. These
// build real binaries and hold their whole output against what Node v24.18.0 prints for
// the same program.

// TestABoxedBufferPrintsLikeNode covers the rendering. A buffer prints its bytes as a
// hex run under the [Uint8Contents] slot and a view prints its geometry and then the
// buffer under it, which is why the view breaks over lines where the buffer alone does
// not.
func TestABoxedBufferPrintsLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new ArrayBuffer(4);\n"+
			"console.log(b);\n"+
			"console.log({ bytes: b });\n"+
			"console.log(new SharedArrayBuffer(3));\n"+
			"console.log(new DataView(new ArrayBuffer(4)));\n")
	want := "ArrayBuffer { [Uint8Contents]: <00 00 00 00>, [byteLength]: 4 }\n" +
		"{\n" +
		"  bytes: ArrayBuffer { [Uint8Contents]: <00 00 00 00>, [byteLength]: 4 }\n" +
		"}\n" +
		"SharedArrayBuffer { [Uint8Contents]: <00 00 00>, [byteLength]: 3 }\n" +
		"DataView {\n" +
		"  [byteLength]: 4,\n" +
		"  [byteOffset]: 0,\n" +
		"  [buffer]: ArrayBuffer { [Uint8Contents]: <00 00 00 00>, [byteLength]: 4 }\n" +
		"}\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAWriteThroughAViewIsSeenByTheBoxedBuffer covers the whole reason the box is a view
// rather than a copy: the program writes bytes through a DataView and then logs the
// buffer, and what it prints has to be the bytes it just wrote.
func TestAWriteThroughAViewIsSeenByTheBoxedBuffer(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new ArrayBuffer(4);\n"+
			"const v = new DataView(b);\n"+
			"v.setUint8(0, 1);\n"+
			"v.setUint8(3, 255);\n"+
			"console.log(b);\n"+
			"console.log(v.getUint8(0), v.getUint8(3), v.byteLength, v.byteOffset);\n"+
			"console.log(b.byteLength, b.resizable, b.detached, b.maxByteLength);\n")
	want := "ArrayBuffer { [Uint8Contents]: <01 00 00 ff>, [byteLength]: 4 }\n" +
		"1 255 4 0\n" +
		"4 false false 4\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABufferSlicesLikeNode covers the slice this group adds to the ArrayBuffer, which
// had no lowering at all before it: the bounds are relative the way Array.prototype
// .slice's are and an omitted one runs to the end of the buffer.
func TestABufferSlicesLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new ArrayBuffer(4);\n"+
			"console.log(b.slice(1, 3).byteLength);\n"+
			"console.log(b.slice().byteLength);\n"+
			"console.log(b.slice(-2).byteLength);\n"+
			"console.log(b.slice(3, 1).byteLength);\n")
	want := "2\n4\n2\n0\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestACollectionOfBuffersBoxesEachOne covers the element bridge rather than the buffer:
// a container of buffers is a typed container of a typed element, so it has to present
// each member through the buffer's own box when it crosses. A buffer read back out of
// one is the buffer the container holds, not a copy, which is what lets a program write
// into what it found.
func TestACollectionOfBuffersBoxesEachOne(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new ArrayBuffer(2);\n"+
			"const m = new Map();\n"+
			"m.set('bytes', b);\n"+
			"new DataView(b).setUint8(0, 9);\n"+
			"console.log(m.get('bytes'));\n"+
			"const s = new Set([new ArrayBuffer(1)]);\n"+
			"console.log(s.size, s);\n")
	want := "ArrayBuffer { [Uint8Contents]: <09 00>, [byteLength]: 2 }\n" +
		"1 Set(1) { ArrayBuffer { [Uint8Contents]: <00>, [byteLength]: 1 } }\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestABoxedBufferLooksLikeAnObject covers what a buffer is from the outside once it is
// dynamic: an object whose own property table is empty, since its bytes are reached
// through members on the prototype rather than through properties. Two neighbors of this
// belong here and cannot be written yet, each for a reason that has nothing to do with
// the box: the [object ArrayBuffer] tag needs Object.prototype.toString.call and
// 'byteLength' in b needs the in operator outside a union narrowing. Both are pinned in
// pkg/value/bufferbox_test.go instead.
func TestABoxedBufferLooksLikeAnObject(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const b = new ArrayBuffer(4);\n"+
			"console.log(typeof b, Object.keys(b).length, Object.values(b).length);\n"+
			"console.log(JSON.stringify(b), JSON.stringify({ b: b }));\n"+
			"console.log(String(b));\n")
	want := "object 0 0\n" +
		"{} {\"b\":{}}\n" +
		"[object ArrayBuffer]\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAssertOverBuffersMatchesNode covers the deep comparison through the box: a buffer
// is its bytes, so two buffers holding the same run are equal however each was built,
// and one holding a different run is not.
func TestAssertOverBuffersMatchesNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('node:assert');\n"+
			"assert.deepStrictEqual(new ArrayBuffer(2), new ArrayBuffer(2));\n"+
			"const a = new ArrayBuffer(2);\n"+
			"new DataView(a).setUint8(0, 1);\n"+
			"try {\n"+
			"  assert.deepStrictEqual(a, new ArrayBuffer(2));\n"+
			"} catch (e) {\n"+
			"  console.log('threw');\n"+
			"}\n"+
			"console.log('done');\n")
	want := "threw\ndone\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
