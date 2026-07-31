package value

import (
	"math/big"
	"strings"
	"testing"
)

// The box a typed array takes when it crosses into a dynamic slot is a view of the live
// elements rather than a copy, and unlike every other box on this wall it presents own
// properties: its indices. These hold that pair of facts through every surface that can
// see it, the reads, the writes, the key walks, the JSON serializers, the deep
// comparison and the rendering, all against Node v24.

// boxedInt32Array builds a three-element array and hands back both halves, so a test can
// write through one and read through the other.
func boxedInt32Array(t *testing.T) (*TypedArray[int32], Value) {
	t.Helper()
	a := Int32ArrayOf(5, 0, 7)
	return a, a.ToValue()
}

// TestATypedArrayHasOneBox holds the identity the whole slice rests on. A typed array is
// a view over a buffer, so two boxes of one array would be two objects writing into the
// same bytes while === said they were different, and a copy would be worse still: a write
// through it would land where nobody reads.
func TestATypedArrayHasOneBox(t *testing.T) {
	a, box := boxedInt32Array(t)

	if again := a.ToValue(); !StrictEquals(box, again) {
		t.Error("two crossings of one typed array are not the same object")
	}
	a.SetAt(0, 9)
	if got := box.Get(FromGoString("0")); got.AsNumber() != 9 {
		t.Errorf("a[0] off the box after a typed write = %v, want 9", got.AsNumber())
	}
	box.SetKey(FromGoString("1"), Number(4))
	if got := a.At(1); got != 4 {
		t.Errorf("a[1] off the array after a boxed write = %v, want 4", got)
	}
}

// TestATypedArrayBoxReadsIndicesAndGeometry covers the read surface that is not a method:
// an index, the four accessors the prototype carries, and the element-width constant. An
// index past the end reads undefined rather than climbing the prototype chain, which is
// what makes an integer-indexed slot different from an ordinary property.
func TestATypedArrayBoxReadsIndicesAndGeometry(t *testing.T) {
	buf := NewArrayBuffer(16)
	a := Int32ArrayView(buf, 4, 3)
	box := a.ToValue()
	a.SetAt(2, 8)

	cases := []struct {
		name string
		want float64
	}{
		{"2", 8},
		{"length", 3},
		{"byteLength", 12},
		{"byteOffset", 4},
		{"BYTES_PER_ELEMENT", 4},
	}
	for _, c := range cases {
		if got := box.Get(FromGoString(c.name)); got.AsNumber() != c.want {
			t.Errorf("%s off the box = %v, want %v", c.name, got.AsNumber(), c.want)
		}
	}
	if got := box.Get(FromGoString("3")); got.Kind() != KindUndefined {
		t.Errorf("an index past the end = %v, want undefined", got.Kind())
	}
	// The .buffer read hands back the same object the buffer's own box is, since both
	// come off the buffer's cached view rather than out of a fresh wrapper.
	if !StrictEquals(box.Get(FromGoString("buffer")), buf.ToValue()) {
		t.Error("a.buffer off the box is not the buffer's own box")
	}
}

// TestATypedArrayBoxWritesThroughTheKindsCoercion covers the write half. Each kind
// applies its own store rule, which is the one per-element behavior that differs across
// the family, and a write past the end is dropped rather than growing the array, since a
// typed array's length is its view's.
func TestATypedArrayBoxWritesThroughTheKindsCoercion(t *testing.T) {
	wrapped := NewInt8Array(1).ToValue()
	wrapped.SetKey(FromGoString("0"), Number(300))
	if got := wrapped.Get(FromGoString("0")); got.AsNumber() != 44 {
		t.Errorf("300 written into an Int8Array = %v, want 44, the wrap", got.AsNumber())
	}

	clamped := Uint8ClampedArrayOf(0).ToValue()
	clamped.SetKey(FromGoString("0"), Number(300))
	if got := clamped.Get(FromGoString("0")); got.AsNumber() != 255 {
		t.Errorf("300 written into a Uint8ClampedArray = %v, want 255, the clamp", got.AsNumber())
	}

	short := NewInt32Array(2).ToValue()
	short.SetKey(FromGoString("5"), Number(1))
	if got := short.Get(FromGoString("length")); got.AsNumber() != 2 {
		t.Errorf("length after a write past the end = %v, want 2", got.AsNumber())
	}
	if got := short.Get(FromGoString("5")); got.Kind() != KindUndefined {
		t.Error("a write past the end was stored, want dropped")
	}

	// A name that is not an index is an ordinary property on a typed array, so it is
	// stored and then walked after the indices, which is what node lists.
	named := Int32ArrayOf(1, 2, 3).ToValue()
	named.SetKey(FromGoString("foo"), Number(1))
	if got := named.Get(FromGoString("foo")); got.AsNumber() != 1 {
		t.Errorf("a named property off the box = %v, want 1", got.AsNumber())
	}
	if got := keyList(named.OwnEnumerableKeys()); got != "0,1,2,foo" {
		t.Errorf("Object.keys after a named write = %q, want \"0,1,2,foo\"", got)
	}
}

// TestATypedArrayBoxWalksItsIndices covers the key walks, the one place a typed array
// differs in kind from every other box here: its indices are its own properties, so
// Object.keys of one lists them where Object.keys of a Map or a buffer answers nothing.
// hasOwn is narrower than the in operator, since the length and the methods live on the
// prototype rather than on the array.
func TestATypedArrayBoxWalksItsIndices(t *testing.T) {
	_, box := boxedInt32Array(t)

	if got := keyList(box.OwnEnumerableKeys()); got != "0,1,2" {
		t.Errorf("Object.keys of a typed array = %q, want \"0,1,2\"", got)
	}
	vals := box.OwnValues().Elems()
	if len(vals) != 3 || vals[0].AsNumber() != 5 || vals[2].AsNumber() != 7 {
		t.Errorf("Object.values of a typed array = %v, want [5 0 7]", vals)
	}
	if !box.HasProperty(FromGoString("0")) {
		t.Error("'0' in a = false, want true")
	}
	if box.HasProperty(FromGoString("5")) {
		t.Error("'5' in a = true, want false, an index past the end is not a property")
	}
	if !box.HasProperty(FromGoString("length")) || !box.HasProperty(FromGoString("map")) {
		t.Error("the prototype members are not reachable through in")
	}
	if !box.HasOwnElem(StringValue(FromGoString("0"))) {
		t.Error("Object.hasOwn(a, 0) = false, want true")
	}
	if box.HasOwnElem(StringValue(FromGoString("length"))) {
		t.Error("Object.hasOwn(a, 'length') = true, want false, length is on the prototype")
	}
}

// kindName names a value's kind for a comparison a test failure can read.
func kindName(v Value) string {
	if v.Kind() == KindUndefined {
		return "undefined"
	}
	return ToString(v).ToGoString()
}

// keyList joins a key array for a readable comparison.
func keyList(keys *Array[BStr]) string {
	var parts []string
	for _, k := range keys.Elems() {
		parts = append(parts, k.ToGoString())
	}
	return strings.Join(parts, ",")
}

// TestATypedArrayBoxIsTaggedAndCoerces covers what a typed array looks like from the
// outside. It names itself through a real Symbol.toStringTag the way a buffer does rather
// than through an internal slot the way a Date does, and its string form is its elements
// joined with commas, which it gets from a toString on the prototype rather than from a
// Symbol.toPrimitive.
func TestATypedArrayBoxIsTaggedAndCoerces(t *testing.T) {
	_, box := boxedInt32Array(t)

	if got := ClassTag(box).ToGoString(); got != "[object Int32Array]" {
		t.Errorf("Object.prototype.toString.call(a) = %q, want \"[object Int32Array]\"", got)
	}
	if got := ToString(box.getSymKey(symbolToStringTag)).ToGoString(); got != "Int32Array" {
		t.Errorf("a[Symbol.toStringTag] = %q, want \"Int32Array\"", got)
	}
	if got := ToString(box).ToGoString(); got != "5,0,7" {
		t.Errorf("String(a) = %q, want \"5,0,7\"", got)
	}
	// Every kind names itself, including the two the runtime keeps in their own structs.
	kinds := []struct {
		box  Value
		want string
	}{
		{Uint8ArrayOf(1).ToValue(), "Uint8Array"},
		{Uint8ClampedArrayOf(1).ToValue(), "Uint8ClampedArray"},
		{Float64ArrayOf(1).ToValue(), "Float64Array"},
		{BigInt64ArrayOf(big.NewInt(1)).ToValue(), "BigInt64Array"},
	}
	for _, k := range kinds {
		if got := ClassTag(k.box).ToGoString(); got != "[object "+k.want+"]" {
			t.Errorf("the class tag = %q, want \"[object %s]\"", got, k.want)
		}
	}
}

// TestATypedArrayBoxIterates covers the default iterator, which is what a spread and a
// for...of over a boxed array walk, along with the three named walks. A buffer carries no
// iterator at all, so this is the first box on this wall since the collections to have
// one.
func TestATypedArrayBoxIterates(t *testing.T) {
	_, box := boxedInt32Array(t)

	if got := drainNumbers(t, box.getSymKey(symbolIterator).Call()); got != "5,0,7" {
		t.Errorf("[...a] = %q, want \"5,0,7\"", got)
	}
	if got := drainNumbers(t, callMember(t, box, "values")); got != "5,0,7" {
		t.Errorf("a.values() = %q, want \"5,0,7\"", got)
	}
	if got := drainNumbers(t, callMember(t, box, "keys")); got != "0,1,2" {
		t.Errorf("a.keys() = %q, want \"0,1,2\"", got)
	}
	entries := callMember(t, box, "entries")
	first := entries.Get(FromGoString("next")).Call().Get(FromGoString("value"))
	if got := ToString(first).ToGoString(); got != "0,5" {
		t.Errorf("the first entry = %q, want \"0,5\"", got)
	}
}

// drainNumbers walks an iterator to exhaustion and joins what it yielded.
func drainNumbers(t *testing.T, iter Value) string {
	t.Helper()
	next := iter.Get(FromGoString("next"))
	var parts []string
	for i := 0; i < 100; i++ {
		res := next.Call()
		if ToBoolean(res.Get(FromGoString("done"))) {
			return strings.Join(parts, ",")
		}
		parts = append(parts, ToString(res.Get(FromGoString("value"))).ToGoString())
	}
	t.Fatal("the iterator did not finish")
	return ""
}

// TestATypedArrayBoxCarriesThePrototype covers the member surface. The methods are written
// once against the backing interface rather than delegated to the runtime struct, which is
// what gives the Uint8Array and the bigint kinds a map and a filter their structs never
// grew, and what lets the callbacks take the (value, index, array) triple the language
// passes where the struct's own callbacks take the element alone.
func TestATypedArrayBoxCarriesThePrototype(t *testing.T) {
	a := Int32ArrayOf(5, 0, 7, 3)
	box := a.ToValue()
	num := func(f float64) Value { return Number(f) }
	str := func(s string) Value { return StringValue(FromGoString(s)) }

	cases := []struct {
		name string
		got  string
		want string
	}{
		{"at", ToString(callMember(t, box, "at", num(-1))).ToGoString(), "3"},
		{"at past the end", kindName(callMember(t, box, "at", num(9))), kindName(Undefined)},
		{"indexOf", ToString(callMember(t, box, "indexOf", num(7))).ToGoString(), "2"},
		{"indexOf from", ToString(callMember(t, box, "indexOf", num(0), num(2))).ToGoString(), "-1"},
		{"lastIndexOf", ToString(callMember(t, box, "lastIndexOf", num(3))).ToGoString(), "3"},
		{"includes", ToString(callMember(t, box, "includes", num(7))).ToGoString(), "true"},
		{"join", ToString(callMember(t, box, "join", str("-"))).ToGoString(), "5-0-7-3"},
		{"toString", ToString(callMember(t, box, "toString")).ToGoString(), "5,0,7,3"},
		{"slice", ToString(callMember(t, box, "slice", num(1), num(3))).ToGoString(), "0,7"},
		{"slice from the end", ToString(callMember(t, box, "slice", num(-2))).ToGoString(), "7,3"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}

	// A callback gets the element, its index and the array, which is what separates these
	// members from the runtime struct's own.
	seen := ""
	callMember(t, box, "forEach", NewFunc(func(args []Value) Value {
		seen += ToString(args[0]).ToGoString() + "@" + ToString(args[1]).ToGoString() +
			"/" + ToString(args[2].Get(FromGoString("length"))).ToGoString() + " "
		return Undefined
	}))
	if seen != "5@0/4 0@1/4 7@2/4 3@3/4 " {
		t.Errorf("forEach saw %q, want the element, index and array of each", seen)
	}

	if got := ToString(callMember(t, box, "reduce", NewFunc(func(args []Value) Value {
		return Number(ToNumber(args[0]) + ToNumber(args[1]))
	}))).ToGoString(); got != "15" {
		t.Errorf("reduce = %q, want \"15\"", got)
	}
	if got := ToString(callMember(t, box, "reduceRight", NewFunc(func(args []Value) Value {
		return StringValue(FromGoString(ToString(args[0]).ToGoString() + "|" + ToString(args[1]).ToGoString()))
	}))).ToGoString(); got != "3|7|0|5" {
		t.Errorf("reduceRight = %q, want \"3|7|0|5\"", got)
	}
}

// TestATypedArrayBoxAllocatesItsOwnKind covers the members that hand back another array.
// Each one is built through the backing's view constructor, so the result is the same kind
// as the receiver whatever kind that is, and subarray is the one that aliases rather than
// copies.
func TestATypedArrayBoxAllocatesItsOwnKind(t *testing.T) {
	a := Int32ArrayOf(1, 2, 3, 4)
	box := a.ToValue()

	mapped := callMember(t, box, "map", NewFunc(func(args []Value) Value {
		return Number(ToNumber(args[0])*10 + ToNumber(args[1]))
	}))
	if got := ClassTag(mapped).ToGoString(); got != "[object Int32Array]" {
		t.Errorf("a.map(...) is a %q, want an Int32Array", got)
	}
	if got := ToString(mapped).ToGoString(); got != "10,21,32,43" {
		t.Errorf("a.map(...) = %q, want \"10,21,32,43\"", got)
	}

	filtered := callMember(t, box, "filter", NewFunc(func(args []Value) Value {
		return Bool(int(ToNumber(args[0]))%2 == 0)
	}))
	if got := ToString(filtered).ToGoString(); got != "2,4" {
		t.Errorf("a.filter(...) = %q, want \"2,4\"", got)
	}

	// A Uint8Array and a BigInt64Array get the same members even though neither struct
	// carries a map of its own, which is the whole point of writing them against the
	// backing rather than delegating.
	bytes := callMember(t, Uint8ArrayOf(1, 2).ToValue(), "map", NewFunc(func(args []Value) Value {
		return Number(ToNumber(args[0]) * 2)
	}))
	if got := ClassTag(bytes).ToGoString(); got != "[object Uint8Array]" {
		t.Errorf("a Uint8Array's map hands back a %q, want a Uint8Array", got)
	}
	if got := ToString(bytes).ToGoString(); got != "2,4" {
		t.Errorf("a Uint8Array's map = %q, want \"2,4\"", got)
	}
	bigs := callMember(t, BigInt64ArrayOf(big.NewInt(3), big.NewInt(1)).ToValue(), "toSorted")
	if got := ClassTag(bigs).ToGoString(); got != "[object BigInt64Array]" {
		t.Errorf("a BigInt64Array's toSorted hands back a %q, want a BigInt64Array", got)
	}
	if got := ToString(bigs).ToGoString(); got != "1,3" {
		t.Errorf("a BigInt64Array's toSorted = %q, want \"1,3\"", got)
	}

	// subarray is the one member that answers a view, so a write through it shows in the
	// receiver, where slice's copy does not.
	sub := callMember(t, box, "subarray", Number(1), Number(3))
	sub.SetKey(FromGoString("0"), Number(99))
	if got := ToString(box).ToGoString(); got != "1,99,3,4" {
		t.Errorf("the receiver after a write through subarray = %q, want \"1,99,3,4\"", got)
	}
	if got := sub.Get(FromGoString("byteOffset")).AsNumber(); got != 4 {
		t.Errorf("the subarray's byteOffset = %v, want 4", got)
	}
	if !StrictEquals(sub.Get(FromGoString("buffer")), box.Get(FromGoString("buffer"))) {
		t.Error("a subarray does not share the receiver's buffer")
	}
}

// TestATypedArrayBoxMutatesInPlace covers the members that write back into the receiver.
// Each of them works from a snapshot of the elements, so an overlapping copy takes the
// values the array held before the write started rather than smearing what it just wrote.
func TestATypedArrayBoxMutatesInPlace(t *testing.T) {
	cases := []struct {
		name string
		make func() Value
		want string
	}{
		{"fill", func() Value {
			box := NewInt32Array(4).ToValue()
			callMember(t, box, "fill", Number(7), Number(1), Number(3))
			return box
		}, "0,7,7,0"},
		{"fill with no bounds", func() Value {
			box := NewInt32Array(3).ToValue()
			callMember(t, box, "fill", Number(1))
			return box
		}, "1,1,1"},
		{"copyWithin", func() Value {
			box := Int32ArrayOf(1, 2, 3, 4, 5).ToValue()
			callMember(t, box, "copyWithin", Number(0), Number(3))
			return box
		}, "4,5,3,4,5"},
		{"reverse", func() Value {
			box := Int32ArrayOf(1, 2, 3).ToValue()
			callMember(t, box, "reverse")
			return box
		}, "3,2,1"},
		{"sort", func() Value {
			box := Int32ArrayOf(10, 9, 2).ToValue()
			callMember(t, box, "sort")
			return box
		}, "2,9,10"},
		{"sort with a comparator", func() Value {
			box := Int32ArrayOf(10, 9, 2).ToValue()
			callMember(t, box, "sort", NewFunc(func(args []Value) Value {
				return Number(ToNumber(args[1]) - ToNumber(args[0]))
			}))
			return box
		}, "10,9,2"},
		{"set from a plain array", func() Value {
			box := NewInt32Array(4).ToValue()
			callMember(t, box, "set", NewArrayValue([]Value{Number(1), Number(2)}), Number(1))
			return box
		}, "0,1,2,0"},
		{"set from another typed array", func() Value {
			box := NewInt32Array(4).ToValue()
			callMember(t, box, "set", Int8ArrayOf(1, 2).ToValue(), Number(2))
			return box
		}, "0,0,1,2"},
		{"with", func() Value {
			return callMember(t, Int32ArrayOf(1, 2, 3).ToValue(), "with", Number(1), Number(9))
		}, "1,9,3"},
		{"toReversed", func() Value {
			return callMember(t, Int32ArrayOf(1, 2, 3).ToValue(), "toReversed")
		}, "3,2,1"},
	}
	for _, c := range cases {
		if got := ToString(c.make()).ToGoString(); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}

	// The default order is numeric rather than by string form, which is what makes a typed
	// array's sort different from an ordinary array's, and the two edges the spec pins are
	// a NaN last and a negative zero before a positive one.
	floats := Float64ArrayOf(nan(), 1, negZero(), 0, -1).ToValue()
	callMember(t, floats, "sort")
	if got := ToString(floats).ToGoString(); got != "-1,0,0,1,NaN" {
		t.Errorf("a float sort = %q, want \"-1,0,0,1,NaN\"", got)
	}
	if !signbitAt(floats, 1) || signbitAt(floats, 2) {
		t.Error("the negative zero did not sort before the positive one")
	}
}

// signbitAt reports whether the element at i is a negative zero, which the string form
// cannot show.
func signbitAt(box Value, i int) bool {
	f := box.Get(NumberToString(float64(i))).AsNumber()
	return f == 0 && strings.HasPrefix(NumberToString(1/f).ToGoString(), "-")
}

// TestATypedArrayBoxThrowsWhereNodeDoes covers the three members that raise rather than
// answer, each with node's own wording so a catch that reads the message sees what node
// reports.
func TestATypedArrayBoxThrowsWhereNodeDoes(t *testing.T) {
	cases := []struct {
		name string
		fn   func()
		want string
	}{
		{"with past the end", func() {
			callMember(t, NewInt32Array(2).ToValue(), "with", Number(5), Number(1))
		}, "RangeError: Invalid typed array index"},
		{"set past the end", func() {
			callMember(t, NewInt32Array(2).ToValue(), "set",
				NewArrayValue([]Value{Number(1), Number(2), Number(3)}))
		}, "RangeError: offset is out of bounds"},
		{"reduce of an empty array", func() {
			callMember(t, NewInt32Array(0).ToValue(), "reduce",
				NewFunc(func(args []Value) Value { return args[0] }))
		}, "TypeError: Reduce of empty array with no initial value"},
		{"a number written into a bigint array", func() {
			BigInt64ArrayOf(big.NewInt(1)).ToValue().SetKey(FromGoString("0"), Number(5))
		}, "TypeError: Cannot convert 5 to a BigInt"},
	}
	for _, c := range cases {
		if got := catchThrown(t, c.fn); got != c.want {
			t.Errorf("%s threw %q, want %q", c.name, got, c.want)
		}
	}
}

// TestATypedArraySerializesAsAnIndexObject covers JSON.stringify over a box in each of the
// three serializers. A typed array has no toJSON, so what is written is its own
// properties, which are its indices: node writes {"0":5,"1":0,"2":7} where an empty
// property table would give {}. A bigint element has no JSON form at all and throws.
func TestATypedArraySerializesAsAnIndexObject(t *testing.T) {
	_, box := boxedInt32Array(t)

	if got := JSONStringify(box).ToGoString(); got != `{"0":5,"1":0,"2":7}` {
		t.Errorf("JSON.stringify(a) = %q, want the index object", got)
	}
	want := "{\n  \"0\": 5,\n  \"1\": 0,\n  \"2\": 7\n}"
	if got := JSONStringifyIndentNum(box, 2).ToGoString(); got != want {
		t.Errorf("JSON.stringify(a, null, 2) = %q, want %q", got, want)
	}
	// The replacer walk reaches the same keys, so a replacer sees the elements under their
	// index names rather than an object with nothing to visit.
	doubling := func(_ BStr, v Value) Value {
		if v.kind != KindNumber {
			return v
		}
		return Number(v.AsNumber() * 2)
	}
	if got := JSONStringifyReplacerFunc(box, doubling, "").ToGoString(); got != `{"0":10,"1":0,"2":14}` {
		t.Errorf("JSON.stringify(a, fn) = %q, want the doubled index object", got)
	}
	// A named property written onto the array joins the indices, after them.
	box.SetKey(FromGoString("foo"), Number(1))
	if got := JSONStringify(box).ToGoString(); got != `{"0":5,"1":0,"2":7,"foo":1}` {
		t.Errorf("JSON.stringify of an array with a named property = %q", got)
	}

	// A statically typed array reaches the serializers as the runtime struct rather than
	// as a box, since the lowerer passes it straight through, so each walk routes it to
	// its own box instead of reflecting it into an empty object.
	if got := JSONStringify(Int32ArrayOf(1, 2)).ToGoString(); got != `{"0":1,"1":2}` {
		t.Errorf("JSON.stringify of an unboxed typed array = %q, want the index object", got)
	}
	if got := JSONStringifyIndentNum(Uint8ArrayOf(1), 2).ToGoString(); got != "{\n  \"0\": 1\n}" {
		t.Errorf("the indented form of an unboxed typed array = %q", got)
	}
	nested := NewArray[any](Int32ArrayOf(1, 2))
	if got := JSONStringify(nested).ToGoString(); got != `[{"0":1,"1":2}]` {
		t.Errorf("JSON.stringify of an array of typed arrays = %q", got)
	}

	bigs := BigInt64ArrayOf(big.NewInt(1)).ToValue()
	if got := catchThrown(t, func() { JSONStringify(bigs) }); got != "TypeError: Do not know how to serialize a BigInt" {
		t.Errorf("JSON.stringify of a bigint array threw %q", got)
	}
}

// TestATypedArrayComparesByItsBytes covers the deep comparison. Node holds two typed
// arrays against their bytes rather than their numbers, which is what makes two NaNs of
// the same bit pattern equal and the two zeros not, and the tag test keeps the kinds apart
// before the bytes are ever read.
func TestATypedArrayComparesByItsBytes(t *testing.T) {
	cases := []struct {
		name string
		a, b Value
		want bool
	}{
		{"the same elements", Int32ArrayOf(1, 2).ToValue(), Int32ArrayOf(1, 2).ToValue(), true},
		{"different kinds", Int32ArrayOf(1, 2).ToValue(), Uint32ArrayOf(1, 2).ToValue(), false},
		{"different lengths", Int32ArrayOf(1).ToValue(), Int32ArrayOf(1, 2).ToValue(), false},
		{"two NaNs", Float64ArrayOf(nan()).ToValue(), Float64ArrayOf(nan()).ToValue(), true},
		{"the two zeros", Float64ArrayOf(0).ToValue(), Float64ArrayOf(negZero()).ToValue(), false},
		{"against a plain array", Int32ArrayOf(1).ToValue(), NewArrayValue([]Value{Number(1)}), false},
	}
	for _, c := range cases {
		if got := DeepStrictEqual(c.a, c.b); got != c.want {
			t.Errorf("deepStrictEqual of %s = %v, want %v", c.name, got, c.want)
		}
	}
	// A named property is compared the way an array's named extras are, so one side
	// carrying it and the other not are different arrays.
	withProp := Int32ArrayOf(1).ToValue()
	withProp.SetKey(FromGoString("foo"), Number(1))
	if DeepStrictEqual(withProp, Int32ArrayOf(1).ToValue()) {
		t.Error("an array with a named property compared equal to one without it")
	}
}

// TestATypedArrayRendersLikeNode holds the rendering whole against Node v24.18.0. A typed
// array prints in brackets like an array and, unlike an array, always names its kind and
// its length, and a long one lays out in right-aligned columns because every element of
// one is a number.
func TestATypedArrayRendersLikeNode(t *testing.T) {
	long := NewInt32Array(10)
	for i := 0; i < 10; i++ {
		long.SetAt(float64(i), float64(i+1))
	}

	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"three elements", Int32ArrayOf(5, 0, 7).ToValue(), "Int32Array(3) [ 5, 0, 7 ]"},
		{"empty", NewInt32Array(0).ToValue(), "Int32Array(0) []"},
		{"bytes", Uint8ArrayOf(1, 2, 3).ToValue(), "Uint8Array(3) [ 1, 2, 3 ]"},
		{"a negative zero", Float64ArrayOf(1.5, negZero()).ToValue(), "Float64Array(2) [ 1.5, -0 ]"},
		{"bigints", BigInt64ArrayOf(big.NewInt(1), big.NewInt(-2)).ToValue(), "BigInt64Array(2) [ 1n, -2n ]"},
		{"clamped", Uint8ClampedArrayOf(300, 2).ToValue(), "Uint8ClampedArray(2) [ 255, 2 ]"},
		{"nested", objectOf("a", Int32ArrayOf(1, 2).ToValue()), "{ a: Int32Array(2) [ 1, 2 ] }"},
		// A long array wraps into columns lined up on their right edge, the layout node
		// gives any array of short numeric entries.
		{"in columns", objectOf("v", long.ToValue()),
			"{\n  v: Int32Array(10) [\n    1, 2, 3, 4,  5,\n    6, 7, 8, 9, 10\n  ]\n}"},
	}
	for _, c := range cases {
		if got := NodeInspect(c.v).ToGoString(); got != c.want {
			t.Errorf("%s rendered %q, want %q", c.name, got, c.want)
		}
	}

	// A named property prints after the elements the way an array's does.
	named := Int32ArrayOf(1, 2, 3).ToValue()
	named.SetKey(FromGoString("foo"), Number(1))
	if got := NodeInspect(named).ToGoString(); got != "Int32Array(3) [ 1, 2, 3, foo: 1 ]" {
		t.Errorf("an array with a named property rendered %q", got)
	}
	// Past the depth limit it collapses to its kind, which happens before the arm that
	// would have rendered the elements is ever chosen.
	if got := NodeInspectArgs(objectOf("a", Int32ArrayOf(1).ToValue()), Undefined, Number(0)).ToGoString(); got != "{ a: [Int32Array] }" {
		t.Errorf("a typed array past the depth limit rendered %q", got)
	}
	// A view over a detached buffer is a zero-length view, so it renders as an empty one
	// rather than raising while it is being printed.
	buf := NewArrayBuffer(8)
	stale := Int32ArrayView(buf, 0, 2)
	buf.Detach()
	if got := NodeInspect(stale.ToValue()).ToGoString(); got != "Int32Array(0) []" {
		t.Errorf("a detached array rendered %q, want \"Int32Array(0) []\"", got)
	}
}

// TestATypedArrayRendersItsSlotsUnderShowHidden holds the showHidden form, where the five
// members that live on the prototype are written out as bracketed slots. The buffer is the
// one that is not a number, so it goes last, and it prints only its length rather than its
// bytes: the array above it has already shown what those bytes hold.
func TestATypedArrayRendersItsSlotsUnderShowHidden(t *testing.T) {
	want := "Int32Array(2) [\n  1,\n  2,\n  [BYTES_PER_ELEMENT]: 4,\n  [length]: 2,\n" +
		"  [byteLength]: 8,\n  [byteOffset]: 0,\n  [buffer]: ArrayBuffer { [byteLength]: 8 }\n]"
	got := NodeInspectArgs(Int32ArrayOf(1, 2).ToValue(), objectOf("showHidden", Bool(true))).ToGoString()
	if got != want {
		t.Errorf("the showHidden rendering = %q, want %q", got, want)
	}
}
