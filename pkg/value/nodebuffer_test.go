package value

import (
	"strings"
	"testing"
)

// These cover Buffer from the outside, through the same dynamic member reads a compiled
// program makes, because that is the only way a program ever reaches one: nothing in the
// AOT path lowers a Buffer call statically. Every expectation is the output of node
// v24.11.0 on the same program.

// bufOf builds a Buffer over the bytes of a Go string and hands back its dynamic value,
// the thing a program holds.
func bufOf(s string) Value {
	return NodeBufferFromGo([]byte(s)).ToValue()
}

// bufHex reads a buffer's bytes as hex, the shortest way to say what is in one.
func bufHex(t *testing.T, b Value) string {
	t.Helper()
	return callMember(t, b, "toString", StringValue(FromGoString("hex"))).AsString().ToGoString()
}

// bufStatic reads a static off the Buffer constructor and calls it.
func bufStatic(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	return callMember(t, BufferConstructor(), name, args...)
}

// TestBufferAllocatesTheWayNodeDoes covers the three allocators and their fill argument.
// allocUnsafe is the one worth naming: node hands back whatever was in the memory, and Go
// has no way to ask its allocator for that, so bento's is zeroed. A program that reads
// before it writes is broken either way; this one just fails less interestingly.
func TestBufferAllocatesTheWayNodeDoes(t *testing.T) {
	if got := bufHex(t, bufStatic(t, "alloc", Number(4))); got != "00000000" {
		t.Errorf("alloc(4) = %s, want four zero bytes", got)
	}
	if got := bufStatic(t, "allocUnsafe", Number(3)).Get(FromGoString("length")); got.AsNumber() != 3 {
		t.Errorf("allocUnsafe(3).length = %v, want 3", got.AsNumber())
	}
	if got := bufHex(t, bufStatic(t, "allocUnsafeSlow", Number(2))); got != "0000" {
		t.Errorf("allocUnsafeSlow(2) = %s, want two zero bytes", got)
	}
	if got := bufStatic(t, "alloc", Number(0)).Get(FromGoString("length")); got.AsNumber() != 0 {
		t.Errorf("alloc(0).length = %v, want 0", got.AsNumber())
	}

	// The fill argument takes a byte, a string, or a string in an encoding, and a string
	// shorter than the buffer repeats rather than leaving the rest zero.
	if got := callMember(t, bufStatic(t, "alloc", Number(5), StringValue(FromGoString("ab"))), "toString").AsString().ToGoString(); got != "ababa" {
		t.Errorf("alloc(5, 'ab') = %q, want ababa", got)
	}
	if got := callMember(t, bufStatic(t, "alloc", Number(4), Number(0x61)), "toString").AsString().ToGoString(); got != "aaaa" {
		t.Errorf("alloc(4, 0x61) = %q, want aaaa", got)
	}
	fromB64 := bufStatic(t, "alloc", Number(4), StringValue(FromGoString("aGk=")), StringValue(FromGoString("base64")))
	if got := bufHex(t, fromB64); got != "68696869" {
		t.Errorf("alloc(4, 'aGk=', 'base64') = %s, want the two bytes repeated", got)
	}

	if code, _ := catchThrownCode(func() { bufStatic(t, "alloc", Number(-1)) }); code != "ERR_OUT_OF_RANGE" {
		t.Errorf("alloc(-1) threw %q, want ERR_OUT_OF_RANGE", code)
	}
}

// TestBufferFromTakesEachOfItsSources covers the five shapes Buffer.from accepts. The
// ArrayBuffer one is the odd member of the set: it shares the bytes rather than copying
// them, which is the whole reason it exists, and every other source copies.
func TestBufferFromTakesEachOfItsSources(t *testing.T) {
	if got := bufHex(t, bufStatic(t, "from", StringValue(FromGoString("hello")))); got != "68656c6c6f" {
		t.Errorf("from('hello') = %s", got)
	}
	if got := callMember(t, bufStatic(t, "from", StringValue(FromGoString("aGVsbG8=")), StringValue(FromGoString("base64"))), "toString").AsString().ToGoString(); got != "hello" {
		t.Errorf("from('aGVsbG8=', 'base64') = %q, want hello", got)
	}

	arr := NewArray[Value]()
	arr.Push(Number(1), Number(2), Number(300))
	// An element is stored as a byte, so 300 wraps the way a write into a Uint8Array does.
	if got := bufHex(t, bufStatic(t, "from", arr.ToValue())); got != "01022c" {
		t.Errorf("from([1, 2, 300]) = %s, want the third element wrapped", got)
	}

	// A view over a buffer shares it, so a write through one is seen by the other.
	raw := NewArrayBuffer(4)
	copy(raw.Bytes(), []byte{1, 2, 3, 4})
	shared := bufStatic(t, "from", raw.ToValue(), Number(1), Number(2))
	if got := bufHex(t, shared); got != "0203" {
		t.Errorf("from(buffer, 1, 2) = %s, want the window it was pointed at", got)
	}
	raw.Bytes()[1] = 0xff
	if got := bufHex(t, shared); got != "ff03" {
		t.Errorf("from(ArrayBuffer) copied its bytes, want a view: %s", got)
	}

	// A typed array is copied element by element, so the two do not share and a write
	// through the source does not reach the buffer.
	src := NewUint8Array(2)
	src.live()[0], src.live()[1] = 7, 8
	taken := bufStatic(t, "from", src.ToValue())
	src.live()[0] = 9
	if got := bufHex(t, taken); got != "0708" {
		t.Errorf("from(Uint8Array) = %s, want a copy of the bytes as they were", got)
	}

	// A number is not a source. Node made this an error rather than an allocation on
	// purpose, since the old Buffer(n) constructor handed back uninitialized memory.
	code, msg := catchThrownCode(func() { bufStatic(t, "from", Number(5)) })
	want := "The first argument must be of type string or an instance of Buffer, ArrayBuffer, or Array or an Array-like Object. Received type number (5)"
	if code != "ERR_INVALID_ARG_TYPE" || msg != want {
		t.Errorf("from(5) threw %q %q, want ERR_INVALID_ARG_TYPE and node's wording", code, msg)
	}
}

// TestBufferStaticsAnswerAboutBuffers covers the statics that are not allocators:
// the two predicates, the byte count, the ordering, and the concatenation.
func TestBufferStaticsAnswerAboutBuffers(t *testing.T) {
	if got := bufStatic(t, "isBuffer", bufOf("a")); !got.AsBool() {
		t.Error("isBuffer of a buffer = false, want true")
	}
	// A plain Uint8Array is not a Buffer even though a Buffer is a Uint8Array, which is the
	// asymmetry the brand exists to record.
	if got := bufStatic(t, "isBuffer", NewUint8Array(2).ToValue()); got.AsBool() {
		t.Error("isBuffer of a Uint8Array = true, want false")
	}
	if got := bufStatic(t, "isBuffer", NewObject()); got.AsBool() {
		t.Error("isBuffer of a plain object = true, want false")
	}

	for _, name := range []string{"UTF-8", "ucs2", "utf-16LE", "binary", "hex", "base64url"} {
		if got := bufStatic(t, "isEncoding", StringValue(FromGoString(name))); !got.AsBool() {
			t.Errorf("isEncoding(%q) = false, want true", name)
		}
	}
	if got := bufStatic(t, "isEncoding", StringValue(FromGoString("nope"))); got.AsBool() {
		t.Error("isEncoding('nope') = true, want false")
	}

	if got := bufStatic(t, "byteLength", StringValue(FromGoString("héllo"))); got.AsNumber() != 6 {
		t.Errorf("byteLength('héllo') = %v, want 6", got.AsNumber())
	}
	// A buffer measures itself, so byteLength answers its length without encoding anything.
	if got := bufStatic(t, "byteLength", bufOf("hello")); got.AsNumber() != 5 {
		t.Errorf("byteLength of a buffer = %v, want 5", got.AsNumber())
	}

	if got := bufStatic(t, "compare", bufOf("b"), bufOf("a")); got.AsNumber() != 1 {
		t.Errorf("compare('b', 'a') = %v, want 1", got.AsNumber())
	}
	if got := bufStatic(t, "compare", bufOf("a"), bufOf("a")); got.AsNumber() != 0 {
		t.Errorf("compare('a', 'a') = %v, want 0", got.AsNumber())
	}

	list := NewArray[Value]()
	list.Push(bufOf("ab"), bufOf("cd"))
	if got := callMember(t, bufStatic(t, "concat", list.ToValue()), "toString").AsString().ToGoString(); got != "abcd" {
		t.Errorf("concat(['ab', 'cd']) = %q, want abcd", got)
	}
	// A total length shorter than the parts truncates, and one longer pads with zeroes,
	// because the total is the size of the result rather than a hint about it.
	if got := callMember(t, bufStatic(t, "concat", list.ToValue(), Number(3)), "toString").AsString().ToGoString(); got != "abc" {
		t.Errorf("concat(list, 3) = %q, want abc", got)
	}
	if got := bufHex(t, bufStatic(t, "concat", list.ToValue(), Number(5))); got != "6162636400" {
		t.Errorf("concat(list, 5) = %s, want a zero on the end", got)
	}
	if got := bufStatic(t, "concat", NewArray[Value]().ToValue()).Get(FromGoString("length")); got.AsNumber() != 0 {
		t.Errorf("concat([]) has length %v, want 0", got.AsNumber())
	}
}

// TestABufferSlicesWithoutCopying covers slice and subarray, which are the same function:
// Buffer.prototype.slice is subarray, so both hand back a window onto the receiver's bytes
// rather than a copy, and a write through one is seen by the other. This diverges from
// Array.prototype.slice, and a program that assumed otherwise corrupts its own data.
func TestABufferSlicesWithoutCopying(t *testing.T) {
	b := bufOf("hello")
	if got := callMember(t, callMember(t, b, "slice", Number(1), Number(3)), "toString").AsString().ToGoString(); got != "el" {
		t.Errorf("slice(1, 3) = %q, want el", got)
	}
	if got := callMember(t, callMember(t, b, "subarray", Number(-2)), "toString").AsString().ToGoString(); got != "lo" {
		t.Errorf("subarray(-2) = %q, want lo", got)
	}
	// An end before the start is an empty window rather than an error or a reversal.
	if got := callMember(t, b, "slice", Number(3), Number(1)).Get(FromGoString("length")); got.AsNumber() != 0 {
		t.Errorf("slice(3, 1).length = %v, want 0", got.AsNumber())
	}

	head := callMember(t, b, "slice", Number(0), Number(2))
	head.SetKey(FromGoString("0"), Number(0x48))
	if got := callMember(t, b, "toString").AsString().ToGoString(); got != "Hello" {
		t.Errorf("the receiver after a write through its slice = %q, want Hello", got)
	}
	// The window is a Buffer too, not a bare Uint8Array, so the whole surface is still there.
	if got := bufStatic(t, "isBuffer", head); !got.AsBool() {
		t.Error("a slice of a buffer is not a buffer")
	}
}

// TestABufferComparesAndCopies covers the four members that take another buffer. copy is a
// memmove rather than a copy loop, so an overlapping copy inside one buffer is still right.
func TestABufferComparesAndCopies(t *testing.T) {
	src := bufOf("hello")
	if got := callMember(t, src, "equals", bufOf("hello")); !got.AsBool() {
		t.Error("equals of the same bytes = false, want true")
	}
	if got := callMember(t, src, "equals", bufOf("hellp")); got.AsBool() {
		t.Error("equals of different bytes = true, want false")
	}
	if got := callMember(t, src, "compare", bufOf("hellp")); got.AsNumber() != -1 {
		t.Errorf("compare('hello', 'hellp') = %v, want -1", got.AsNumber())
	}
	// The four range arguments compare a window of each side, so a program can order two
	// slices without cutting them first.
	if got := callMember(t, src, "compare", bufOf("xhelloy"), Number(1), Number(6)); got.AsNumber() != 0 {
		t.Errorf("compare against a window = %v, want 0", got.AsNumber())
	}

	dst := bufStatic(t, "alloc", Number(5), Number('.'))
	if got := callMember(t, src, "copy", dst, Number(1), Number(0), Number(3)); got.AsNumber() != 3 {
		t.Errorf("copy returned %v, want the 3 bytes it wrote", got.AsNumber())
	}
	if got := callMember(t, dst, "toString").AsString().ToGoString(); got != ".hel." {
		t.Errorf("the target after a copy = %q, want .hel.", got)
	}

	// An overlapping copy within one buffer moves the bytes rather than smearing the first
	// one across the range, which a naive forward loop would do.
	over := bufOf("abcdef")
	callMember(t, over, "copy", over, Number(2), Number(0), Number(4))
	if got := callMember(t, over, "toString").AsString().ToGoString(); got != "ababcd" {
		t.Errorf("an overlapping copy = %q, want ababcd", got)
	}
}

// TestABufferSearchesForBytesAndStrings covers indexOf, lastIndexOf and includes. Each
// takes a needle of three kinds, which is what separates them from the Array members of
// the same name: a string is its encoded bytes, a buffer is its bytes, and a number is one
// byte rather than an element compared by value.
func TestABufferSearchesForBytesAndStrings(t *testing.T) {
	b := bufOf("hello")
	if got := callMember(t, b, "indexOf", StringValue(FromGoString("ll"))); got.AsNumber() != 2 {
		t.Errorf("indexOf('ll') = %v, want 2", got.AsNumber())
	}
	if got := callMember(t, b, "indexOf", Number(0x6c)); got.AsNumber() != 2 {
		t.Errorf("indexOf(0x6c) = %v, want 2", got.AsNumber())
	}
	if got := callMember(t, b, "indexOf", bufOf("lo")); got.AsNumber() != 3 {
		t.Errorf("indexOf(buffer 'lo') = %v, want 3", got.AsNumber())
	}
	if got := callMember(t, b, "indexOf", StringValue(FromGoString("z"))); got.AsNumber() != -1 {
		t.Errorf("indexOf('z') = %v, want -1", got.AsNumber())
	}
	if got := callMember(t, b, "includes", StringValue(FromGoString("lo"))); !got.AsBool() {
		t.Error("includes('lo') = false, want true")
	}
	if got := callMember(t, bufOf("aXbXc"), "lastIndexOf", StringValue(FromGoString("X"))); got.AsNumber() != 3 {
		t.Errorf("lastIndexOf('X') = %v, want 3", got.AsNumber())
	}
	// The offset counts from the end when negative, the same rule the string members take.
	if got := callMember(t, b, "indexOf", StringValue(FromGoString("l")), Number(-2)); got.AsNumber() != 3 {
		t.Errorf("indexOf('l', -2) = %v, want 3", got.AsNumber())
	}
	// An empty needle is found at the offset rather than nowhere.
	if got := callMember(t, b, "indexOf", StringValue(FromGoString(""))); got.AsNumber() != 0 {
		t.Errorf("indexOf('') = %v, want 0", got.AsNumber())
	}
	// A needle in an encoding is its decoded bytes, so the same two bytes are found under
	// two spellings.
	if got := callMember(t, b, "indexOf", StringValue(FromGoString("6c6f")), Number(0), StringValue(FromGoString("hex"))); got.AsNumber() != 3 {
		t.Errorf("indexOf('6c6f', 0, 'hex') = %v, want 3", got.AsNumber())
	}
}

// TestABufferWritesAndFills covers the two members that put a string into existing bytes.
// write's signature is the awkward one, since the encoding may take the second or the
// third slot depending on whether an offset was passed, and it is resolved by looking at
// which slots hold strings rather than by counting arguments.
func TestABufferWritesAndFills(t *testing.T) {
	b := bufStatic(t, "alloc", Number(6))
	if got := callMember(t, b, "write", StringValue(FromGoString("abc"))); got.AsNumber() != 3 {
		t.Errorf("write('abc') returned %v, want 3", got.AsNumber())
	}
	if got := bufHex(t, b); got != "616263000000" {
		t.Errorf("the buffer after a write = %s, want the three bytes at the front", got)
	}
	callMember(t, b, "write", StringValue(FromGoString("de")), Number(4))
	if got := bufHex(t, b); got != "616263006465" {
		t.Errorf("the buffer after a write at an offset = %s", got)
	}
	callMember(t, b, "write", StringValue(FromGoString("4142")), Number(0), StringValue(FromGoString("hex")))
	if got := bufHex(t, b); got != "414263006465" {
		t.Errorf("the buffer after a hex write at an offset = %s", got)
	}
	callMember(t, b, "write", StringValue(FromGoString("zz")), StringValue(FromGoString("latin1")))
	if got := bufHex(t, b); got != "7a7a63006465" {
		t.Errorf("the buffer after a write with an encoding and no offset = %s", got)
	}

	// A write with no room for the last code point stops on a boundary rather than leaving
	// a fragment that would read back as a replacement character.
	short := bufStatic(t, "alloc", Number(4))
	if got := callMember(t, short, "write", StringValue(FromGoString("€uro"))); got.AsNumber() != 4 {
		t.Errorf("a truncated write returned %v, want 4", got.AsNumber())
	}
	if got := bufHex(t, short); got != "e282ac75" {
		t.Errorf("a truncated write left %s, want the euro sign and the u", got)
	}

	f := bufStatic(t, "alloc", Number(6))
	callMember(t, f, "fill", Number(0x61))
	if got := callMember(t, f, "toString").AsString().ToGoString(); got != "aaaaaa" {
		t.Errorf("fill(0x61) = %q, want aaaaaa", got)
	}
	callMember(t, f, "fill", StringValue(FromGoString("xy")), Number(1), Number(5))
	if got := callMember(t, f, "toString").AsString().ToGoString(); got != "axyxya" {
		t.Errorf("fill('xy', 1, 5) = %q, want the pattern repeated in the window", got)
	}
	// The two-argument form with a string in the second slot is fill(value, encoding), not
	// fill(value, offset), which is the one place the signature is decided by a type.
	e := bufStatic(t, "alloc", Number(4))
	callMember(t, e, "fill", StringValue(FromGoString("aGk=")), StringValue(FromGoString("base64")))
	if got := bufHex(t, e); got != "68696869" {
		t.Errorf("fill('aGk=', 'base64') = %s, want the two decoded bytes repeated", got)
	}
}

// TestABufferReadsAndWritesNumbers covers the fixed-width half of the numeric family. Each
// pair is checked through the bytes rather than only against itself, so a read and a write
// that were wrong in the same direction would still be caught.
func TestABufferReadsAndWritesNumbers(t *testing.T) {
	b := bufStatic(t, "alloc", Number(8))

	callMember(t, b, "writeUInt32BE", Number(0xdeadbeef), Number(0))
	if got := bufHex(t, b); got != "deadbeef0000000000000000"[:16] {
		t.Errorf("writeUInt32BE(0xdeadbeef) left %s", got)
	}
	if got := callMember(t, b, "readUInt32BE", Number(0)); got.AsNumber() != 3735928559 {
		t.Errorf("readUInt32BE = %v, want 3735928559", got.AsNumber())
	}
	// The same four bytes read signed are a negative number, which is the whole point of
	// there being two members over one width.
	if got := callMember(t, b, "readInt32BE", Number(0)); got.AsNumber() != -559038737 {
		t.Errorf("readInt32BE = %v, want -559038737", got.AsNumber())
	}

	callMember(t, b, "writeUInt32LE", Number(0xdeadbeef), Number(0))
	if got := bufHex(t, b)[:8]; got != "efbeadde" {
		t.Errorf("writeUInt32LE left %s, want the bytes reversed", got)
	}
	// The lowercase-u spelling is the same member, an alias node carries for every one of
	// the unsigned reads and writes.
	if got := callMember(t, b, "readUint32LE", Number(0)); got.AsNumber() != 3735928559 {
		t.Errorf("readUint32LE = %v, want 3735928559", got.AsNumber())
	}

	callMember(t, b, "writeInt8", Number(-1), Number(0))
	if got := callMember(t, b, "readInt8", Number(0)); got.AsNumber() != -1 {
		t.Errorf("readInt8 = %v, want -1", got.AsNumber())
	}
	if got := callMember(t, b, "readUInt8", Number(0)); got.AsNumber() != 255 {
		t.Errorf("readUInt8 of the same byte = %v, want 255", got.AsNumber())
	}

	callMember(t, b, "writeInt16LE", Number(-2), Number(0))
	if got := callMember(t, b, "readInt16LE", Number(0)); got.AsNumber() != -2 {
		t.Errorf("readInt16LE = %v, want -2", got.AsNumber())
	}
	if got := callMember(t, b, "readInt16BE", Number(0)); got.AsNumber() != -257 {
		t.Errorf("readInt16BE of the same bytes = %v, want -257", got.AsNumber())
	}

	callMember(t, b, "writeFloatLE", Number(1.5), Number(0))
	if got := bufHex(t, b)[:8]; got != "0000c03f" {
		t.Errorf("writeFloatLE(1.5) left %s", got)
	}
	if got := callMember(t, b, "readFloatLE", Number(0)); got.AsNumber() != 1.5 {
		t.Errorf("readFloatLE = %v, want 1.5", got.AsNumber())
	}
	callMember(t, b, "writeDoubleBE", Number(-0.5), Number(0))
	if got := bufHex(t, b); got != "bfe0000000000000" {
		t.Errorf("writeDoubleBE(-0.5) left %s", got)
	}
	if got := callMember(t, b, "readDoubleBE", Number(0)); got.AsNumber() != -0.5 {
		t.Errorf("readDoubleBE = %v, want -0.5", got.AsNumber())
	}

	// The eight-byte integers speak bigint, since a double cannot carry their low bits.
	callMember(t, b, "writeBigInt64BE", BigIntFromInt64(-2), Number(0))
	got := callMember(t, b, "readBigInt64BE", Number(0))
	if got.Kind() != KindBigInt || ToString(got).ToGoString() != "-2" {
		t.Errorf("readBigInt64BE = %v %q, want the bigint -2", got.Kind(), ToString(got).ToGoString())
	}
	unsigned := callMember(t, b, "readBigUInt64BE", Number(0))
	if ToString(unsigned).ToGoString() != "18446744073709551614" {
		t.Errorf("readBigUInt64BE of the same bytes = %q, want 18446744073709551614", ToString(unsigned).ToGoString())
	}
	if ToString(callMember(t, b, "readBigUint64BE", Number(0))).ToGoString() != "18446744073709551614" {
		t.Error("readBigUint64BE is not the same member as readBigUInt64BE")
	}
}

// TestABufferReadsAndWritesVariableWidthNumbers covers the other half of the family, the
// six members that take a byte count instead of carrying one in their name. They exist for
// widths a fixed member does not have, three and five and six bytes, which a program
// reading a packed record needs and which no other API in the language offers.
func TestABufferReadsAndWritesVariableWidthNumbers(t *testing.T) {
	v := bufStatic(t, "alloc", Number(6))

	callMember(t, v, "writeUIntBE", Number(0x010203040506), Number(0), Number(6))
	if got := bufHex(t, v); got != "010203040506" {
		t.Errorf("writeUIntBE of a six-byte value left %s", got)
	}
	if got := callMember(t, v, "readUIntBE", Number(0), Number(6)); got.AsNumber() != 1108152157446 {
		t.Errorf("readUIntBE = %v, want 1108152157446", got.AsNumber())
	}
	if got := callMember(t, v, "readUIntLE", Number(0), Number(6)); got.AsNumber() != 6618611909121 {
		t.Errorf("readUIntLE of the same bytes = %v, want 6618611909121", got.AsNumber())
	}

	callMember(t, v, "writeIntLE", Number(-3), Number(0), Number(3))
	if got := bufHex(t, v); got != "fdffff040506" {
		t.Errorf("writeIntLE(-3, 0, 3) left %s", got)
	}
	if got := callMember(t, v, "readIntLE", Number(0), Number(3)); got.AsNumber() != -3 {
		t.Errorf("readIntLE = %v, want -3", got.AsNumber())
	}
	// The unsigned read of the same three bytes is the two's-complement pattern as a
	// magnitude, which is what makes the signed member worth having.
	if got := callMember(t, v, "readUIntLE", Number(0), Number(3)); got.AsNumber() != 16777213 {
		t.Errorf("readUIntLE of the same three bytes = %v, want 16777213", got.AsNumber())
	}
}

// TestANumericAccessOutOfRangeThrows covers the bounds. Node made these throw rather than
// answer undefined or a partial value, because a read past the end of a buffer is the
// mistake a parser makes on malformed input and silence there is how it becomes a
// vulnerability. The wording is held too, since a program logs it.
func TestANumericAccessOutOfRangeThrows(t *testing.T) {
	b := bufStatic(t, "alloc", Number(8))

	code, msg := catchThrownCode(func() { callMember(t, b, "readUInt32BE", Number(6)) })
	if code != "ERR_OUT_OF_RANGE" || msg != `The value of "offset" is out of range. It must be >= 0 and <= 4. Received 6` {
		t.Errorf("a read past the end threw %q %q", code, msg)
	}
	// The ceiling in the message is the last offset a read of this width could start at,
	// not the buffer's length, so it moves with the member being called.
	_, msg = catchThrownCode(func() { callMember(t, b, "readUInt8", Number(9)) })
	if msg != `The value of "offset" is out of range. It must be >= 0 and <= 7. Received 9` {
		t.Errorf("a one-byte read past the end said %q", msg)
	}
	code, msg = catchThrownCode(func() { callMember(t, b, "readUIntBE", Number(0), Number(7)) })
	if code != "ERR_OUT_OF_RANGE" || msg != `The value of "byteLength" is out of range. It must be >= 1 and <= 6. Received 7` {
		t.Errorf("a seven-byte read threw %q %q", code, msg)
	}
	if code, _ := catchThrownCode(func() { callMember(t, b, "writeUInt32BE", Number(1), Number(6)) }); code != "ERR_OUT_OF_RANGE" {
		t.Errorf("a write past the end threw %q, want ERR_OUT_OF_RANGE", code)
	}
}

// TestABufferSwapsItsBytes covers the three byte-order reversals. Each throws rather than
// working on a partial group when the length does not divide, since half a swap would
// leave the buffer holding data in neither order.
func TestABufferSwapsItsBytes(t *testing.T) {
	four := bufStatic(t, "from", arrayOfBytes(1, 2, 3, 4))
	if got := bufHex(t, callMember(t, four, "swap16")); got != "02010403" {
		t.Errorf("swap16 = %s, want the pairs reversed", got)
	}
	// The receiver is swapped in place and handed back, so the return is the same buffer
	// rather than a reversed copy.
	if got := bufHex(t, four); got != "02010403" {
		t.Errorf("the receiver after swap16 = %s, want it swapped in place", got)
	}
	if got := bufHex(t, callMember(t, bufStatic(t, "from", arrayOfBytes(1, 2, 3, 4)), "swap32")); got != "04030201" {
		t.Errorf("swap32 = %s, want the four bytes reversed", got)
	}
	eight := bufStatic(t, "from", arrayOfBytes(1, 2, 3, 4, 5, 6, 7, 8))
	if got := bufHex(t, callMember(t, eight, "swap64")); got != "0807060504030201" {
		t.Errorf("swap64 = %s", got)
	}

	code, msg := catchThrownCode(func() { callMember(t, bufStatic(t, "from", arrayOfBytes(1, 2, 3)), "swap32") })
	if code != "ERR_INVALID_BUFFER_SIZE" || msg != "Buffer size must be a multiple of 32-bits" {
		t.Errorf("swap32 of three bytes threw %q %q", code, msg)
	}
	if code, _ := catchThrownCode(func() { callMember(t, bufStatic(t, "from", arrayOfBytes(1)), "swap16") }); code != "ERR_INVALID_BUFFER_SIZE" {
		t.Errorf("swap16 of one byte threw %q, want ERR_INVALID_BUFFER_SIZE", code)
	}
}

// arrayOfBytes builds the JavaScript array Buffer.from takes, the shortest way to write a
// buffer of particular bytes in a test.
func arrayOfBytes(bs ...int) Value {
	a := NewArray[Value]()
	for _, b := range bs {
		a.Push(Number(float64(b)))
	}
	return a.ToValue()
}

// TestABufferIsAUint8ArrayWithAWiderSurface covers what the brand does and does not
// change. Every typed-array member still works over the same bytes, because a Buffer is
// one struct rather than a wrapper, and the members Buffer overrides are the ones whose
// meaning differs.
func TestABufferIsAUint8ArrayWithAWiderSurface(t *testing.T) {
	b := bufOf("hello")

	if got := b.Get(FromGoString("length")); got.AsNumber() != 5 {
		t.Errorf("length = %v, want 5", got.AsNumber())
	}
	if got := b.Get(FromGoString("byteLength")); got.AsNumber() != 5 {
		t.Errorf("byteLength = %v, want 5", got.AsNumber())
	}
	if got := b.Get(FromGoString("BYTES_PER_ELEMENT")); got.AsNumber() != 1 {
		t.Errorf("BYTES_PER_ELEMENT = %v, want 1", got.AsNumber())
	}
	if got := b.Get(FromGoString("0")); got.AsNumber() != 0x68 {
		t.Errorf("b[0] = %v, want 0x68", got.AsNumber())
	}
	// The family's own members are untouched, so a Buffer maps and sorts and iterates.
	if got := callMember(t, b, "at", Number(-1)); got.AsNumber() != 0x6f {
		t.Errorf("at(-1) = %v, want 0x6f", got.AsNumber())
	}
	if got := callMember(t, b, "includes", Number(0x65)); !got.AsBool() {
		t.Error("includes(0x65) = false, want true, the numeric needle still reaches a byte")
	}
	if got := b.Get(FromGoString("buffer")).Get(FromGoString("byteLength")); got.AsNumber() != 5 {
		t.Errorf(".buffer.byteLength = %v, want 5", got.AsNumber())
	}
	if got := b.Get(FromGoString("byteOffset")); got.AsNumber() != 0 {
		t.Errorf("byteOffset = %v, want 0", got.AsNumber())
	}

	// toString is the clearest of the overrides: the family's joins the elements with
	// commas, and a Buffer's decodes them.
	if got := callMember(t, b, "toString").AsString().ToGoString(); got != "hello" {
		t.Errorf("toString() = %q, want hello, not the element list", got)
	}
	if got := ToString(b).ToGoString(); got != "hello" {
		t.Errorf("String(buffer) = %q, want hello", got)
	}
	if got := callMember(t, b, "toString", StringValue(FromGoString("utf8")), Number(1), Number(3)).AsString().ToGoString(); got != "el" {
		t.Errorf("toString('utf8', 1, 3) = %q, want el", got)
	}
}

// TestABufferNamesItselfLikeNode covers the three questions a program asks about a value's
// kind, which do not all answer the same way. A Buffer's constructor is Buffer, and it
// passes instanceof Buffer, and yet Object.prototype.toString calls it a Uint8Array,
// because that tag comes from the typed-array prototype's getter and Buffer.prototype adds
// no tag of its own.
func TestABufferNamesItselfLikeNode(t *testing.T) {
	b := bufOf("abc")

	if got := ClassTag(b).ToGoString(); got != "[object Uint8Array]" {
		t.Errorf("Object.prototype.toString.call(buffer) = %q, want [object Uint8Array]", got)
	}
	if got := b.GetElem(SymbolToStringTag()).AsString().ToGoString(); got != "Uint8Array" {
		t.Errorf("Symbol.toStringTag = %q, want Uint8Array", got)
	}
	if got := b.Get(FromGoString("constructor")).Get(FromGoString("name")).AsString().ToGoString(); got != "Buffer" {
		t.Errorf("constructor.name = %q, want Buffer", got)
	}
	if !InstanceOf(b, BufferConstructor()) {
		t.Error("a buffer is not instanceof Buffer")
	}
	// A plain Uint8Array is not one, so the prototype link is doing real work rather than
	// answering true for the whole family.
	if InstanceOf(NewUint8Array(2).ToValue(), BufferConstructor()) {
		t.Error("a Uint8Array is instanceof Buffer, want not")
	}
	if got := b.TypeOf().ToGoString(); got != "object" {
		t.Errorf("typeof buffer = %q, want object", got)
	}
	if !ToBoolean(b) {
		t.Error("an empty-ish buffer is falsy, want every object truthy")
	}
}

// TestABufferSerializesThroughToJSON covers JSON.stringify. A Buffer carries a toJSON
// hook, so unlike an ArrayBuffer it survives a round trip through JSON as the tagged
// object node reads back.
func TestABufferSerializesThroughToJSON(t *testing.T) {
	b := bufOf("Hello")
	if got := JSONStringify(b).ToGoString(); got != `{"type":"Buffer","data":[72,101,108,108,111]}` {
		t.Errorf("JSON.stringify of a buffer = %s", got)
	}
	j := callMember(t, b, "toJSON")
	if got := j.Get(FromGoString("type")).AsString().ToGoString(); got != "Buffer" {
		t.Errorf("toJSON().type = %q, want Buffer", got)
	}
	if got := j.Get(FromGoString("data")).Get(FromGoString("length")); got.AsNumber() != 5 {
		t.Errorf("toJSON().data.length = %v, want 5", got.AsNumber())
	}
	if got := JSONStringify(bufStatic(t, "alloc", Number(0))).ToGoString(); got != `{"type":"Buffer","data":[]}` {
		t.Errorf("JSON.stringify of an empty buffer = %s", got)
	}
	// Buffer.from reads the object back, so the round trip is a round trip.
	back := bufStatic(t, "from", j.Get(FromGoString("data")))
	if got := callMember(t, back, "toString").AsString().ToGoString(); got != "Hello" {
		t.Errorf("the buffer read back out of its JSON = %q, want Hello", got)
	}
}

// TestABufferRendersAsItsBytes holds the rendering against node. A Buffer prints as a hex
// run in angle brackets rather than as the bracketed element list its Uint8Array half
// would take, it never collapses to a name at the depth limit the way an array does, and
// an extra property rides after the bytes.
func TestABufferRendersAsItsBytes(t *testing.T) {
	if got := NodeInspect(bufOf("abc")).ToGoString(); got != "<Buffer 61 62 63>" {
		t.Errorf("a three-byte buffer = %s", got)
	}
	if got := NodeInspect(bufStatic(t, "alloc", Number(0))).ToGoString(); got != "<Buffer >" {
		t.Errorf("an empty buffer = %s", got)
	}
	// A buffer nested past the depth limit still prints its bytes, since there is nothing
	// to recurse into and node prints it in full at every depth.
	if got := NodeInspectArgs(objectOf("b", bufOf("abc")), Undefined, Number(0)).ToGoString(); got != "{ b: <Buffer 61 62 63> }" {
		t.Errorf("a nested buffer at depth zero = %s, want the bytes", got)
	}

	// An extra property goes inside the brackets after the bytes, and it is formatted the
	// way any other property is, so a nested value in one still collapses at the depth
	// limit.
	withProp := bufStatic(t, "alloc", Number(2))
	withProp.SetKey(FromGoString("x"), Number(1))
	withProp.SetKey(FromGoString("yy"), objectOf("z", Number(2)))
	if got := NodeInspect(withProp).ToGoString(); got != "<Buffer 00 00, x: 1, yy: { z: 2 }>" {
		t.Errorf("a buffer with properties = %s, want them after the bytes", got)
	}
	if got := NodeInspectArgs(withProp, Undefined, Number(0)).ToGoString(); got != "<Buffer 00 00, x: 1, yy: [Object]>" {
		t.Errorf("a buffer with properties at depth zero = %s", got)
	}
	// The comma is only written when there were bytes for it to follow, so an empty buffer
	// carrying a property has no leading one.
	empty := bufStatic(t, "alloc", Number(0))
	empty.SetKey(FromGoString("x"), Number(1))
	if got := NodeInspect(empty).ToGoString(); got != "<Buffer x: 1>" {
		t.Errorf("an empty buffer with a property = %s, want no leading comma", got)
	}

	// The run is cut at the inspector's array limit with a count of what was left out, and
	// the count is singular when one byte was.
	long := NodeInspect(bufStatic(t, "alloc", Number(200))).ToGoString()
	if !strings.Contains(long, "... 150 more bytes>") {
		t.Errorf("a two-hundred-byte buffer = %s, want the run cut at fifty bytes", long)
	}
	if got := NodeInspect(bufStatic(t, "alloc", Number(51))).ToGoString(); !strings.Contains(got, "... 1 more byte>") {
		t.Errorf("a fifty-one-byte buffer = %s, want one byte left over, singular", got)
	}
	// The cut is buffer.INSPECT_MAX_BYTES rather than the inspector's array limit, so
	// raising maxArrayLength does not show more bytes and writing the module member does.
	opts := NewObject()
	opts.SetKey(FromGoString("maxArrayLength"), Number(3))
	if got := NodeInspectArgs(bufStatic(t, "alloc", Number(60)), opts).ToGoString(); !strings.Contains(got, "... 10 more bytes>") {
		t.Errorf("a sixty-byte buffer under maxArrayLength 3 = %s, want the fifty-byte cut", got)
	}
	mod := RequireBuiltin("buffer")
	mod.SetKey(FromGoString("INSPECT_MAX_BYTES"), Number(4))
	defer mod.SetKey(FromGoString("INSPECT_MAX_BYTES"), Number(50))
	if got := mod.Get(FromGoString("INSPECT_MAX_BYTES")); got.AsNumber() != 4 {
		t.Errorf("INSPECT_MAX_BYTES read back %v after a write of 4", got.AsNumber())
	}
	if got := NodeInspect(bufStatic(t, "alloc", Number(10))).ToGoString(); got != "<Buffer 00 00 00 00 ... 6 more bytes>" {
		t.Errorf("a ten-byte buffer after INSPECT_MAX_BYTES was set to 4 = %s", got)
	}
}

// TestTheBufferModuleIsTheGlobal covers the identities a program relies on without ever
// checking them: the constructor reached through require('buffer') is the one the global
// names, so a buffer made through either passes an instanceof against the other.
func TestTheBufferModuleIsTheGlobal(t *testing.T) {
	mod := RequireBuiltin("buffer")
	ctor := mod.Get(FromGoString("Buffer"))
	if !StrictEquals(ctor, BufferConstructor()) {
		t.Error("require('buffer').Buffer is not the Buffer global")
	}
	if !StrictEquals(GlobalThisValue().Get(FromGoString("Buffer")), BufferConstructor()) {
		t.Error("globalThis.Buffer is not the Buffer global")
	}
	if got := mod.Get(FromGoString("kMaxLength")); got.AsNumber() != bufferMaxLength {
		t.Errorf("kMaxLength = %v, want %v", got.AsNumber(), float64(bufferMaxLength))
	}
	if got := mod.Get(FromGoString("constants")).Get(FromGoString("MAX_STRING_LENGTH")); got.AsNumber() != bufferMaxStringLength {
		t.Errorf("constants.MAX_STRING_LENGTH = %v, want %v", got.AsNumber(), float64(bufferMaxStringLength))
	}
	// The two base64 helpers on the module are the globals of the same names rather than
	// second copies, which is what keeps them from drifting apart.
	if !StrictEquals(mod.Get(FromGoString("atob")), GlobalValue("atob")) {
		t.Error("buffer.atob is not the atob global")
	}
	if !StrictEquals(mod.Get(FromGoString("btoa")), GlobalValue("btoa")) {
		t.Error("buffer.btoa is not the btoa global")
	}
	// A member of the module that is not implemented throws when it is read rather than
	// answering undefined, so a program that reaches for one is told so at the reach.
	if msg := catchThrown(t, func() { mod.Get(FromGoString("transcode")) }); msg == "" {
		t.Error("an unimplemented member of the buffer module read as undefined, want a throw")
	}
}

// TestPrototypeCachingBuildsOneObject covers the recursion between the constructor and its
// prototype, which point at each other. Reaching either one first has to end with one
// prototype object and one constructor, not two of one of them.
func TestPrototypeCachingBuildsOneObject(t *testing.T) {
	ctor := BufferConstructor()
	if !StrictEquals(ctor, BufferConstructor()) {
		t.Error("two reads of the Buffer constructor are not the same value")
	}
	proto := ctor.Get(FromGoString("prototype"))
	if !StrictEquals(proto.Get(FromGoString("constructor")), ctor) {
		t.Error("Buffer.prototype.constructor is not Buffer")
	}
	if !StrictEquals(proto, bufferPrototype()) {
		t.Error("two reads of Buffer.prototype are not the same object")
	}
	// Neither the constructor's own statics nor the prototype's link is enumerable, so a
	// key walk over the constructor answers nothing the way node's does.
	if got := ctor.OwnEnumerableKeys().Len(); got != 0 {
		t.Errorf("the Buffer constructor has %v enumerable own keys, want none", got)
	}
	if got := proto.OwnEnumerableKeys().Len(); got != 0 {
		t.Errorf("Buffer.prototype has %v enumerable own keys, want none", got)
	}
}

// TestANewBufferIsAlsoABuffer covers the deprecated constructor form. new Buffer(n) and
// Buffer(n) both still work in node and both are the same as calling from or alloc, since
// the constructor's two entry points share one implementation.
func TestANewBufferIsAlsoABuffer(t *testing.T) {
	made := Construct(BufferConstructor(), StringValue(FromGoString("hi")))
	if got := callMember(t, made, "toString").AsString().ToGoString(); got != "hi" {
		t.Errorf("new Buffer('hi') = %q, want hi", got)
	}
	if !InstanceOf(made, BufferConstructor()) {
		t.Error("new Buffer('hi') is not instanceof Buffer")
	}
	called := BufferConstructor().Call(StringValue(FromGoString("hi")))
	if got := callMember(t, called, "toString").AsString().ToGoString(); got != "hi" {
		t.Errorf("Buffer('hi') = %q, want hi", got)
	}
}
