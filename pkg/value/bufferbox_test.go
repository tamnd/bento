package value

import (
	"strings"
	"testing"
)

// The box the three byte-buffer objects take when they cross into a dynamic slot is a
// view of the live bytes rather than a copy, which is most of what these check: a write
// made through one view is seen through every other, one buffer has one box however many
// times it crosses, and the members answer off the storage as it is now rather than as it
// was when the box was built. The renderings and the deep comparisons below are held
// against Node v24.

// boxedBuffer builds a four-byte buffer and hands back both halves, so a test can write
// through one and read through the other.
func boxedBuffer(t *testing.T) (*ArrayBuffer, Value) {
	t.Helper()
	b := NewArrayBuffer(4)
	return b, b.ToValue()
}

// boxedView builds a view over an eight-byte buffer and hands back both halves.
func boxedView(t *testing.T) (*DataView, Value) {
	t.Helper()
	d := NewDataView(NewArrayBuffer(8), 0)
	return d, d.ToValue()
}

// TestABoxedArrayBufferReadsTheLiveBuffer covers the ArrayBuffer member surface. The four
// accessors answer values rather than callables because they are getters on the
// prototype, and each of them is read after the buffer moved underneath it, so what is
// checked is that the box looks at the storage rather than at a snapshot.
func TestABoxedArrayBufferReadsTheLiveBuffer(t *testing.T) {
	b := NewResizableArrayBuffer(2, 8)
	box := b.ToValue()

	if got := box.Get(FromGoString("byteLength")); got.AsNumber() != 2 {
		t.Errorf("byteLength = %v, want 2", got.AsNumber())
	}
	if got := box.Get(FromGoString("maxByteLength")); got.AsNumber() != 8 {
		t.Errorf("maxByteLength = %v, want 8", got.AsNumber())
	}
	if got := box.Get(FromGoString("resizable")); !got.AsBool() {
		t.Error("resizable = false, want true")
	}
	if got := box.Get(FromGoString("detached")); got.AsBool() {
		t.Error("detached = true, want false")
	}

	b.Resize(6)
	if got := box.Get(FromGoString("byteLength")); got.AsNumber() != 6 {
		t.Errorf("byteLength after a resize = %v, want 6, the box reads a snapshot", got.AsNumber())
	}

	b.Detach()
	if got := box.Get(FromGoString("detached")); !got.AsBool() {
		t.Error("detached after a detach = false, want true")
	}
	if got := box.Get(FromGoString("byteLength")); got.AsNumber() != 0 {
		t.Errorf("byteLength of a detached buffer = %v, want 0", got.AsNumber())
	}
	if got := box.Get(FromGoString("nope")); got.Kind() != KindUndefined {
		t.Errorf("a name that is not an ArrayBuffer member = %v, want undefined", got.Kind())
	}
}

// TestABoxedArrayBufferCallsItsMethods covers the method half: slice copies the bytes it
// was pointed at, the result is itself a box so a read chains off it, and the transfer
// pair hands the bytes away and leaves the receiver detached.
func TestABoxedArrayBufferCallsItsMethods(t *testing.T) {
	b, box := boxedBuffer(t)
	copy(b.Bytes(), []byte{1, 2, 3, 4})

	cut := callMember(t, box, "slice", Number(1), Number(3))
	if got := cut.Get(FromGoString("byteLength")); got.AsNumber() != 2 {
		t.Errorf("slice(1, 3).byteLength = %v, want 2", got.AsNumber())
	}
	if got := NodeInspect(cut).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <02 03>, [byteLength]: 2 }" {
		t.Errorf("slice(1, 3) = %s, want the two bytes it was pointed at", got)
	}
	// An omitted bound is absent rather than a NaN, so a bare slice copies the whole run.
	if got := NodeInspect(callMember(t, box, "slice")).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <01 02 03 04>, [byteLength]: 4 }" {
		t.Errorf("slice() = %s, want the whole buffer", got)
	}
	// A negative bound counts from the end, the relative-index rule slice shares with
	// Array.prototype.slice.
	if got := NodeInspect(callMember(t, box, "slice", Number(-2))).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <03 04>, [byteLength]: 2 }" {
		t.Errorf("slice(-2) = %s, want the last two bytes", got)
	}
	// The copy owns its bytes, so a write through the receiver does not reach it.
	b.Bytes()[1] = 0xff
	if got := NodeInspect(cut).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <02 03>, [byteLength]: 2 }" {
		t.Errorf("the slice after a write through the receiver = %s, want the copy it took", got)
	}

	moved := callMember(t, box, "transfer")
	if got := NodeInspect(moved).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <01 ff 03 04>, [byteLength]: 4 }" {
		t.Errorf("transfer() = %s, want the receiver's bytes", got)
	}
	if got := box.Get(FromGoString("detached")); !got.AsBool() {
		t.Error("the receiver of a transfer is not detached, want detached")
	}
	if got := catchThrown(t, func() { callMember(t, box, "slice") }); got != "TypeError: Cannot perform ArrayBuffer.prototype.slice on a detached ArrayBuffer" {
		t.Errorf("slice of a detached buffer threw %q, want the TypeError", got)
	}

	grown := NewResizableArrayBuffer(2, 8).ToValue()
	callMember(t, grown, "resize", Number(5))
	if got := grown.Get(FromGoString("byteLength")); got.AsNumber() != 5 {
		t.Errorf("byteLength after a resize through the box = %v, want 5", got.AsNumber())
	}
	if got := NodeInspect(callMember(t, grown, "transferToFixedLength", Number(2))).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <00 00>, [byteLength]: 2 }" {
		t.Errorf("transferToFixedLength(2) = %s, want the first two bytes", got)
	}
}

// TestABoxedSharedArrayBufferCallsItsMethods covers the shared buffer's surface, the
// ArrayBuffer's with the members that give bytes away dropped: it never detaches, its
// resizable flag is spelled growable, and its resize only ever enlarges.
func TestABoxedSharedArrayBufferCallsItsMethods(t *testing.T) {
	s := NewGrowableSharedArrayBuffer(2, 8)
	box := s.ToValue()
	copy(s.Buffer().Bytes(), []byte{7, 9})

	if got := box.Get(FromGoString("byteLength")); got.AsNumber() != 2 {
		t.Errorf("byteLength = %v, want 2", got.AsNumber())
	}
	if got := box.Get(FromGoString("maxByteLength")); got.AsNumber() != 8 {
		t.Errorf("maxByteLength = %v, want 8", got.AsNumber())
	}
	if got := box.Get(FromGoString("growable")); !got.AsBool() {
		t.Error("growable = false, want true")
	}
	// The three ArrayBuffer members a shared buffer does not carry read undefined rather
	// than answering the ArrayBuffer's behavior through the storage under it.
	for _, name := range []string{"detached", "resizable", "resize", "transfer", "transferToFixedLength"} {
		if got := box.Get(FromGoString(name)); got.Kind() != KindUndefined {
			t.Errorf("%s off a shared buffer = %v, want undefined", name, got.Kind())
		}
	}

	callMember(t, box, "grow", Number(4))
	if got := NodeInspect(box).ToGoString(); got != "SharedArrayBuffer { [Uint8Contents]: <07 09 00 00>, [byteLength]: 4 }" {
		t.Errorf("the buffer after a grow through the box = %s, want the two bytes it kept plus zeroes", got)
	}
	if got := NodeInspect(callMember(t, box, "slice", Number(1), Number(3))).ToGoString(); got != "SharedArrayBuffer { [Uint8Contents]: <09 00>, [byteLength]: 2 }" {
		t.Errorf("slice(1, 3) = %s, want a shared buffer of the two bytes", got)
	}
	if got := catchThrown(t, func() { callMember(t, box, "grow", Number(2)) }); got != "RangeError: SharedArrayBuffer grow length is smaller than the current byte length" {
		t.Errorf("a shrinking grow threw %q, want the RangeError", got)
	}
}

// TestABoxedDataViewReadsAndWritesThroughTheWindow covers the view's member surface. What
// makes it worth its own test is that a view is not storage: every read and write lands
// in the buffer under it, so what is checked is that the box reaches through the view to
// those bytes rather than holding any of its own.
func TestABoxedDataViewReadsAndWritesThroughTheWindow(t *testing.T) {
	d, box := boxedView(t)

	if got := box.Get(FromGoString("byteLength")); got.AsNumber() != 8 {
		t.Errorf("byteLength = %v, want 8", got.AsNumber())
	}
	if got := box.Get(FromGoString("byteOffset")); got.AsNumber() != 0 {
		t.Errorf("byteOffset = %v, want 0", got.AsNumber())
	}
	if got := box.Get(FromGoString("buffer")); !StrictEquals(got, d.Buffer().ToValue()) {
		t.Error("the buffer read off the box is not the view's own buffer")
	}

	// A write through the box is seen by the typed view, and the other way round, because
	// both reach the same run of bytes.
	callMember(t, box, "setInt32", Number(0), Number(-2))
	if got := d.GetInt32(0); got != -2 {
		t.Errorf("the typed view read %v after a write through the box, want -2", got)
	}
	d.SetFloat64(0, 1.5)
	if got := callMember(t, box, "getFloat64", Number(0)); got.AsNumber() != 1.5 {
		t.Errorf("getFloat64(0) = %v after a write through the view, want 1.5", got.AsNumber())
	}

	// The endianness flag is optional and absent means big-endian, so the same bytes read
	// two ways under the two spellings.
	callMember(t, box, "setUint16", Number(0), Number(0x0102))
	if got := callMember(t, box, "getUint16", Number(0)); got.AsNumber() != 0x0102 {
		t.Errorf("getUint16(0) = %v, want 258, an omitted flag means big-endian", got.AsNumber())
	}
	if got := callMember(t, box, "getUint16", Number(0), Bool(true)); got.AsNumber() != 0x0201 {
		t.Errorf("getUint16(0, true) = %v, want 513", got.AsNumber())
	}
	if got := callMember(t, box, "getUint8", Number(0)); got.AsNumber() != 1 {
		t.Errorf("getUint8(0) = %v, want 1", got.AsNumber())
	}
	callMember(t, box, "setInt8", Number(7), Number(-1))
	if got := callMember(t, box, "getInt8", Number(7)); got.AsNumber() != -1 {
		t.Errorf("getInt8(7) = %v, want -1", got.AsNumber())
	}

	// The eight-byte integers speak bigint rather than Number, since the low bits of a
	// 64-bit value do not survive a float64.
	callMember(t, box, "setBigInt64", Number(0), BigIntFromInt64(9007199254740993))
	got := callMember(t, box, "getBigInt64", Number(0))
	if got.Kind() != KindBigInt || ToString(got).ToGoString() != "9007199254740993" {
		t.Errorf("getBigInt64(0) = %v %q, want the bigint that was written", got.Kind(), ToString(got).ToGoString())
	}
	// A Number is not a bigint and the spec makes that an error rather than a rounding,
	// which is the whole reason these two members exist apart from setFloat64.
	if got := catchThrown(t, func() { callMember(t, box, "setBigUint64", Number(0), Number(1)) }); got != "TypeError: Cannot convert 1 to a BigInt" {
		t.Errorf("setBigUint64 with a Number threw %q, want the TypeError", got)
	}
	if got := box.Get(FromGoString("nope")); got.Kind() != KindUndefined {
		t.Errorf("a name that is not a DataView member = %v, want undefined", got.Kind())
	}
}

// TestOneBufferHasOneBox covers the identity the view model exists for. Two crossings of
// one buffer are one object under ===, so a program that boxes the same buffer twice, or
// that reads a view's .buffer twice, is holding one value rather than two copies of the
// bytes.
func TestOneBufferHasOneBox(t *testing.T) {
	b, box := boxedBuffer(t)
	if !StrictEquals(box, b.ToValue()) {
		t.Error("two boxes of one ArrayBuffer are not ===")
	}
	s := NewSharedArrayBuffer(2)
	if !StrictEquals(s.ToValue(), s.ToValue()) {
		t.Error("two boxes of one SharedArrayBuffer are not ===")
	}
	d := NewDataView(b, 0)
	if !StrictEquals(d.ToValue(), d.ToValue()) {
		t.Error("two boxes of one DataView are not ===")
	}
	// A shared buffer and the ArrayBuffer holding its bytes are two values, which they
	// are in JavaScript: the storage is shared, the objects are not.
	if StrictEquals(s.ToValue(), s.Buffer().ToValue()) {
		t.Error("a SharedArrayBuffer and its backing ArrayBuffer are ===, want two values")
	}
	// A view's .buffer is the box of the buffer it was built over, so reading it and
	// boxing that buffer directly land on one object.
	if !StrictEquals(d.ToValue().Get(FromGoString("buffer")), box) {
		t.Error("the .buffer read off a view is not the box of the buffer it views")
	}
}

// TestABufferIsTaggedAndWalksEmpty covers what the three look like from the outside. None
// of them has an own property, so every key walk is empty and their bytes are reached
// only through their members, and each names itself with a real Symbol.toStringTag on the
// prototype rather than with the internal slot a Date uses, so a plain read of the symbol
// answers as well as Object.prototype.toString does.
func TestABufferIsTaggedAndWalksEmpty(t *testing.T) {
	_, box := boxedBuffer(t)
	_, view := boxedView(t)
	shared := NewSharedArrayBuffer(2).ToValue()

	for _, c := range []struct {
		what string
		v    Value
		tag  string
	}{
		{"an ArrayBuffer", box, "ArrayBuffer"},
		{"a SharedArrayBuffer", shared, "SharedArrayBuffer"},
		{"a DataView", view, "DataView"},
	} {
		if got := ClassTag(c.v).ToGoString(); got != "[object "+c.tag+"]" {
			t.Errorf("Object.prototype.toString.call of %s = %q, want [object %s]", c.what, got, c.tag)
		}
		if got := c.v.OwnKeys().Len(); got != 0 {
			t.Errorf("the own keys of %s = %v, want none", c.what, got)
		}
		if got := c.v.GetElem(SymbolToStringTag()); got.AsString().ToGoString() != c.tag {
			t.Errorf("the Symbol.toStringTag of %s = %q, want %s", c.what, got.AsString().ToGoString(), c.tag)
		}
		if got := ToString(c.v).ToGoString(); got != "[object "+c.tag+"]" {
			t.Errorf("String() of %s = %q, want [object %s]", c.what, got, c.tag)
		}
		if got := c.v.TypeOf().ToGoString(); got != "object" {
			t.Errorf("typeof %s = %q, want object", c.what, got)
		}
		// The members are not own properties but they are still members, so the in
		// operator finds them through the prototype the way it does in Node.
		if !c.v.HasProperty(FromGoString("byteLength")) {
			t.Errorf("'byteLength' in %s = false, want true", c.what)
		}
		if c.v.HasProperty(FromGoString("nope")) {
			t.Errorf("'nope' in %s = true, want false", c.what)
		}
	}
}

// TestABufferRendersLikeNode holds the renderings against Node v24. A buffer prints its
// bytes as a hex run under the [Uint8Contents] slot, cut off at the inspector's array
// limit, a detached one prints that it is detached rather than an empty run, and a view
// prints its geometry and then the buffer under it.
func TestABufferRendersLikeNode(t *testing.T) {
	b, box := boxedBuffer(t)
	copy(b.Bytes(), []byte{0, 1, 0x10, 0xff})

	if got := NodeInspect(box).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <00 01 10 ff>, [byteLength]: 4 }" {
		t.Errorf("a four-byte buffer = %s", got)
	}
	if got := NodeInspect(NewArrayBuffer(0).ToValue()).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <>, [byteLength]: 0 }" {
		t.Errorf("an empty buffer = %s", got)
	}
	if got := NodeInspect(NewSharedArrayBuffer(2).ToValue()).ToGoString(); got != "SharedArrayBuffer { [Uint8Contents]: <00 00>, [byteLength]: 2 }" {
		t.Errorf("a shared buffer = %s", got)
	}

	// The contents are cut at the same hundred-element limit an array's elements are, and
	// the count of what was left out rides the end of the run.
	long := NodeInspect(NewArrayBuffer(200).ToValue()).ToGoString()
	if !strings.Contains(long, "... 100 more bytes>") {
		t.Errorf("a two-hundred-byte buffer = %s, want the run cut at a hundred bytes", long)
	}
	if got := NodeInspect(NewArrayBuffer(101).ToValue()).ToGoString(); !strings.Contains(got, "... 1 more byte>") {
		t.Errorf("a hundred-and-one-byte buffer = %s, want one byte left over, singular", got)
	}

	// A view renders its window and then the buffer under it, which is why it breaks over
	// several lines where a buffer of the same size does not.
	small := NodeInspect(NewDataView(NewArrayBuffer(4), 0).ToValue()).ToGoString()
	wantSmall := "DataView {\n" +
		"  [byteLength]: 4,\n" +
		"  [byteOffset]: 0,\n" +
		"  [buffer]: ArrayBuffer { [Uint8Contents]: <00 00 00 00>, [byteLength]: 4 }\n" +
		"}"
	if small != wantSmall {
		t.Errorf("a four-byte view = %s, want\n%s", small, wantSmall)
	}
	// The nested buffer is rendered one indent in, so it runs out of the line budget two
	// bytes sooner than a bare one of the same length would and breaks in turn.
	_, view := boxedView(t)
	want := "DataView {\n" +
		"  [byteLength]: 8,\n" +
		"  [byteOffset]: 0,\n" +
		"  [buffer]: ArrayBuffer {\n" +
		"    [Uint8Contents]: <00 00 00 00 00 00 00 00>,\n" +
		"    [byteLength]: 8\n" +
		"  }\n" +
		"}"
	if got := NodeInspect(view).ToGoString(); got != want {
		t.Errorf("an eight-byte view = %s, want\n%s", got, want)
	}

	// Both kinds collapse to their constructor name once the depth limit is reached, the
	// name coming off the brand rather than off a prototype neither of them has.
	if got := NodeInspectArgs(objectOf("v", view), Undefined, Number(0)).ToGoString(); got != "{ v: [DataView] }" {
		t.Errorf("a nested view at depth zero = %s, want [DataView]", got)
	}
	if got := NodeInspectArgs(objectOf("b", box), Undefined, Number(0)).ToGoString(); got != "{ b: [ArrayBuffer] }" {
		t.Errorf("a nested buffer at depth zero = %s, want [ArrayBuffer]", got)
	}
	if got := NodeInspectArgs(objectOf("s", NewSharedArrayBuffer(4).ToValue()), Undefined, Number(0)).ToGoString(); got != "{ s: [SharedArrayBuffer] }" {
		t.Errorf("a nested shared buffer at depth zero = %s, want [SharedArrayBuffer]", got)
	}

	b.Detach()
	if got := NodeInspect(box).ToGoString(); got != "ArrayBuffer { (detached), [byteLength]: 0 }" {
		t.Errorf("a detached buffer = %s", got)
	}

	// A view whose buffer was detached under it is out of bounds, which makes both of its
	// geometry getters throw. Printing a value is not a place to raise from, so the
	// rendering reads the state off the view instead and says what node says of one: a
	// zero span and no offset. Node's own rendering of this is indented wrong, the leak
	// of the throw it caught halfway through, which is the one thing here not held to it.
	stale := NewDataView(NewArrayBuffer(4), 0)
	stale.Buffer().Detach()
	want = "DataView {\n" +
		"  [byteLength]: 0,\n" +
		"  [byteOffset]: undefined,\n" +
		"  [buffer]: ArrayBuffer { (detached), [byteLength]: 0 }\n" +
		"}"
	if got := NodeInspect(stale.ToValue()).ToGoString(); got != want {
		t.Errorf("a view over a detached buffer = %s, want\n%s", got, want)
	}
}

// TestABufferSerializesAsAnEmptyObject covers JSON.stringify. None of the three carries an
// own property or a toJSON hook, so each writes the empty object: the bytes are not data
// JSON has a form for, which is why a program that wants them serializes a typed array
// over the buffer instead.
func TestABufferSerializesAsAnEmptyObject(t *testing.T) {
	_, box := boxedBuffer(t)
	_, view := boxedView(t)
	shared := NewSharedArrayBuffer(2).ToValue()

	if got := JSONStringify(box).ToGoString(); got != "{}" {
		t.Errorf("JSON.stringify of a buffer = %s, want {}", got)
	}
	if got := JSONStringify(view).ToGoString(); got != "{}" {
		t.Errorf("JSON.stringify of a view = %s, want {}", got)
	}
	if got := JSONStringify(objectOf("b", shared)).ToGoString(); got != `{"b":{}}` {
		t.Errorf("JSON.stringify of a nested shared buffer = %s", got)
	}
}

// TestTwoBuffersAreDeeplyEqualByTheirBytes covers the deep comparison. A buffer is its
// bytes and nothing else, so where it came from and whether it can grow do not enter into
// it, and a view is compared by the window it can see rather than by the buffer under it,
// so two views onto different buffers at different offsets are equal when they read the
// same run.
func TestTwoBuffersAreDeeplyEqualByTheirBytes(t *testing.T) {
	two := NewArrayBuffer(2)
	alsoTwo := NewArrayBuffer(2)
	three := NewArrayBuffer(3)

	if !DeepStrictEqual(two.ToValue(), alsoTwo.ToValue()) {
		t.Error("two empty two-byte buffers are not deeply equal")
	}
	if DeepStrictEqual(two.ToValue(), three.ToValue()) {
		t.Error("buffers of different lengths are deeply equal, want not")
	}
	alsoTwo.Bytes()[0] = 1
	if DeepStrictEqual(two.ToValue(), alsoTwo.ToValue()) {
		t.Error("buffers holding different bytes are deeply equal, want not")
	}
	// A resizable buffer and a fixed one holding the same bytes are equal: the flag is a
	// property of the object, not of the data.
	if !DeepStrictEqual(two.ToValue(), NewResizableArrayBuffer(2, 8).ToValue()) {
		t.Error("a resizable buffer is not deeply equal to a fixed one of the same bytes")
	}
	// A shared buffer names itself, so the tag test keeps the two kinds apart before their
	// bytes are ever compared.
	if DeepStrictEqual(two.ToValue(), NewSharedArrayBuffer(2).ToValue()) {
		t.Error("an ArrayBuffer and a SharedArrayBuffer are deeply equal, want not")
	}
	if !DeepStrictEqual(NewSharedArrayBuffer(2).ToValue(), NewSharedArrayBuffer(2).ToValue()) {
		t.Error("two shared buffers of the same bytes are not deeply equal")
	}
	if DeepStrictEqual(two.ToValue(), NewObject()) {
		t.Error("a buffer and a plain object are deeply equal, want not")
	}

	// The window is what a view is compared by: these two read the same two bytes out of
	// two different buffers at two different offsets, and node calls them equal.
	wide := NewDataView(NewArrayBuffer(8), 4, 2)
	narrow := NewDataView(NewArrayBuffer(2), 0, 2)
	if !DeepStrictEqual(wide.ToValue(), narrow.ToValue()) {
		t.Error("two views onto the same bytes at different offsets are not deeply equal")
	}
	narrow.SetInt8(0, 1)
	if DeepStrictEqual(wide.ToValue(), narrow.ToValue()) {
		t.Error("views reading different bytes are deeply equal, want not")
	}
	if DeepStrictEqual(wide.ToValue(), NewDataView(NewArrayBuffer(8), 0).ToValue()) {
		t.Error("views of different lengths are deeply equal, want not")
	}
	// A view and the buffer it views are two kinds, however their bytes line up.
	if DeepStrictEqual(NewDataView(two, 0).ToValue(), two.ToValue()) {
		t.Error("a view and its buffer are deeply equal, want not")
	}
}

// TestABufferBoxedInACollectionStaysLive covers the element path: a buffer held in a
// collection that is itself boxed is presented through the same box the typed side holds,
// so a write made through the collection's view is seen by the buffer the program still
// has a typed handle on.
func TestABufferBoxedInACollectionStaysLive(t *testing.T) {
	b := NewArrayBuffer(2)
	m := NewStringMap[*ArrayBuffer]()
	m.Set(FromGoString("bytes"), b)

	read := callMember(t, m.ToValue(), "get", StringValue(FromGoString("bytes")))
	if !StrictEquals(read, b.ToValue()) {
		t.Error("the buffer read out of a boxed map is not the box of the one that was put in")
	}
	b.Bytes()[0] = 9
	if got := NodeInspect(read).ToGoString(); got != "ArrayBuffer { [Uint8Contents]: <09 00>, [byteLength]: 2 }" {
		t.Errorf("the buffer read out of a boxed map = %s, want the write made through the typed handle", got)
	}
}
