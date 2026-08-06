// This file is Node's Buffer: the global constructor with its statics, and the member
// table an instance answers.
//
// A Buffer in Node is a Uint8Array. Not something like one, not a wrapper over one: the
// class extends Uint8Array and adds a prototype. bento already has the Uint8Array, a
// view over an ArrayBuffer with live element access and the whole typed-array prototype
// written once against a backing interface (typedarrayvalue.go), so a Buffer here is that
// same struct with a brand set on it. The brand buys three things and nothing else: the
// constructor name is "Buffer", the member lookup consults the table below before the
// family's, and the box links to Buffer.prototype so instanceof answers. Indexing,
// .length, .buffer, .byteOffset, iteration, map, filter, set, sort and the rest are the
// typed array's, already written and already correct over these bytes.
//
// What is here is the part Node adds. It falls in four groups: the encoded string
// crossings (toString, write, and the string forms of fill and indexOf), the byte-run
// operations (equals, compare, copy, fill, indexOf), the endian-aware numeric accessors
// (the read* and write* family), and the statics that make one (alloc, from, concat).
//
// Bounds are Node's, which are stricter than the typed array's. A typed array clamps: an
// index past the end reads undefined. A Buffer numeric accessor throws ERR_OUT_OF_RANGE,
// because reading a 32-bit integer that runs off the end of the window is a bug in the
// caller's arithmetic and silently reading garbage or zero is how a parser ends up
// trusting bytes it never had. The slicing members do clamp, because there Node clamps.
//
// The one Node behaviour deliberately not reproduced is allocUnsafe's uninitialized
// memory. Go's allocator hands back zeroed pages and there is no way to ask it not to,
// so allocUnsafe is alloc here. That is a safe direction to differ in: a program that
// reads an allocUnsafe buffer before writing it sees zeros rather than another
// program's leftovers, and no correct program depends on the leftovers.

package value

import (
	"bytes"
	"encoding/binary"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// bufferMaxLength is Buffer.constants.MAX_LENGTH, the largest buffer that may be
// allocated. Node's value on a 64-bit platform was 2^32 - 1 on older releases and is
// 2^53 - 1 on current ones; the current one is used, since that is what a program checking
// against it on Node 24 compares with. It is a ceiling on the argument rather than a
// promise about memory: an allocation anywhere near it fails in the allocator on both
// runtimes.
const bufferMaxLength = 9007199254740991

// bufferMaxStringLength is Buffer.constants.MAX_STRING_LENGTH, the longest string the
// engine will build, which caps what toString may return.
const bufferMaxStringLength = 536870888

// bufferCtorValue and bufferProtoValue cache the one Buffer constructor and the one
// Buffer.prototype. Both are built on first read and kept, because identity is load
// bearing on each: `Buffer === require('buffer').Buffer` has to hold, and every instance
// links to the one prototype so instanceof walks to the object the constructor names.
var (
	bufferCtorValue  Value
	bufferProtoValue Value
)

// bufferPrototype returns Buffer.prototype, the object every Buffer's box links to. It
// holds no methods: an instance answers its members off the brand in nodeBufferMethod,
// which runs before the prototype chain is walked. What it is for is instanceof, and for
// the .constructor read that reaches it.
func bufferPrototype() Value {
	if bufferProtoValue.Kind() != KindUndefined {
		return bufferProtoValue
	}
	// The object is cached before the constructor is asked for, because the two point at
	// each other: the constructor fills its .prototype from here and the .constructor here
	// is the constructor. Caching first is what makes the recursion terminate on the one
	// object rather than build a second.
	bufferProtoValue = NewObject()
	bufferDefine(bufferProtoValue, "constructor", BufferConstructor())
	return bufferProtoValue
}

// BufferConstructor returns the Buffer global: the callable Node exposes as a constructor
// plus the statics hanging off it. The lowerer emits a call to this for a program that
// names Buffer, and globalThis carries the same value, so the identity globalThis.Buffer
// === Buffer holds the way it does for process.
func BufferConstructor() Value {
	if bufferCtorValue.Kind() != KindUndefined {
		return bufferCtorValue
	}
	// The value is stored before the statics are attached, so the recursion through
	// bufferPrototype below terminates: the prototype asks for the constructor, and by
	// then the constructor exists even though it is not finished.
	ctor := NewFunc(func(args []Value) Value { return bufferFromCall(args) })
	bufferCtorValue = ctor
	o := ctor.object()
	o.construct = func(_ Value, args []Value) Value { return bufferFromCall(args) }
	WithName(ctor, "Buffer")
	bufferDefine(ctor, "prototype", bufferPrototype())
	bufferDefine(ctor, "poolSize", Number(8192))

	bufferDefine(ctor, "alloc", boundMethod("alloc", func(args []Value) Value {
		a := newSizedBuffer(bufferSizeArg(Arg(args, 0)))
		fill := Arg(args, 1)
		if fill.kind != KindUndefined && !(fill.kind == KindNumber && fill.AsNumber() == 0) {
			bufferFill(a, fill, 0, a.liveLen(), bufferEncodingArg(Arg(args, 2)))
		}
		return a.ToValue()
	}))
	// allocUnsafe and allocUnsafeSlow differ from alloc in Node only by where the memory
	// comes from and whether it is zeroed. Go zeroes either way, so both are alloc without
	// the fill argument.
	bufferDefine(ctor, "allocUnsafe", boundMethod("allocUnsafe", func(args []Value) Value {
		return newSizedBuffer(bufferSizeArg(Arg(args, 0))).ToValue()
	}))
	bufferDefine(ctor, "allocUnsafeSlow", boundMethod("allocUnsafeSlow", func(args []Value) Value {
		return newSizedBuffer(bufferSizeArg(Arg(args, 0))).ToValue()
	}))
	bufferDefine(ctor, "from", boundMethod("from", func(args []Value) Value {
		return bufferFrom(Arg(args, 0), Arg(args, 1), Arg(args, 2))
	}))
	bufferDefine(ctor, "of", boundMethod("of", func(args []Value) Value {
		out := newSizedBuffer(len(args))
		d := out.live()
		for i, v := range args {
			d[i] = toUint8(ToNumber(v))
		}
		return out.ToValue()
	}))
	bufferDefine(ctor, "concat", boundMethod("concat", func(args []Value) Value {
		return bufferConcat(Arg(args, 0), Arg(args, 1))
	}))
	bufferDefine(ctor, "isBuffer", boundMethod("isBuffer", func(args []Value) Value {
		return Bool(asNodeBuffer(Arg(args, 0)) != nil)
	}))
	bufferDefine(ctor, "isEncoding", boundMethod("isEncoding", func(args []Value) Value {
		v := Arg(args, 0)
		if v.kind != KindString {
			return Bool(false)
		}
		_, ok := bufferEncoding(v.str().ToGoString())
		return Bool(ok)
	}))
	bufferDefine(ctor, "byteLength", boundMethod("byteLength", func(args []Value) Value {
		return Number(float64(bufferByteLengthOf(Arg(args, 0), Arg(args, 1))))
	}))
	bufferDefine(ctor, "compare", boundMethod("compare", func(args []Value) Value {
		a := requireBufferArg(Arg(args, 0), "buf1")
		b := requireBufferArg(Arg(args, 1), "buf2")
		return Number(float64(bytes.Compare(a.live(), b.live())))
	}))
	return bufferCtorValue
}

// bufferDefine installs a static on the constructor non-enumerably, the way a built-in
// carries its own members: Object.keys(Buffer) is empty in Node, so a plain write would
// make every walk of the constructor list its whole surface.
func bufferDefine(target Value, name string, val Value) {
	o := target.object()
	for i := range o.keys {
		if o.keys[i].ToGoString() == name {
			o.descs[i] = dataProperty(val, true, false, true)
			return
		}
	}
	o.keys = append(o.keys, FromGoString(name))
	o.descs = append(o.descs, dataProperty(val, true, false, true))
}

// NewNodeBuffer builds a zero-filled Buffer of n bytes over its own storage, the Go-side
// constructor the runtime and the statics both build through.
func NewNodeBuffer(n int) *Uint8Array {
	return newSizedBuffer(n)
}

// NodeBufferFromGo wraps a Go byte slice as a Buffer, adopting the slice rather than
// copying it, the same bargain Uint8ArrayFromGo makes: the caller has already decided
// whether the bytes are bento's to own.
func NodeBufferFromGo(b []byte) *Uint8Array {
	a := Uint8ArrayFromGo(b)
	a.nodeBuffer = true
	return a
}

// newSizedBuffer allocates a zeroed Buffer of n bytes.
func newSizedBuffer(n int) *Uint8Array {
	a := NewUint8Array(float64(n))
	a.nodeBuffer = true
	return a
}

// asNodeBuffer answers the Buffer behind a value, or nil when the value is not one. It is
// what Buffer.isBuffer reads and what every member taking another Buffer checks with.
func asNodeBuffer(v Value) *Uint8Array {
	if v.kind != KindObject {
		return nil
	}
	u, ok := v.object().jsTyped.(*Uint8Array)
	if !ok || !u.nodeBuffer {
		return nil
	}
	return u
}

// asByteView answers the byte run behind a value that is a Buffer, a Uint8Array or any
// other typed array, the wider set the members taking a "target" accept. A typed array of
// a wider element type contributes its bytes, which is what Node's byte-level members do
// with one.
func asByteView(v Value) ([]byte, bool) {
	if v.kind != KindObject {
		return nil, false
	}
	if t := v.object().jsTyped; t != nil {
		return typedArrayBytes(t), true
	}
	return nil, false
}

// bufferArrayBufferOf answers the ArrayBuffer a value boxes, or nil when it is not one.
// A SharedArrayBuffer is not accepted: Buffer.from over one would hand out a view whose
// bytes another thread may be writing, and bento has no story for that yet.
func bufferArrayBufferOf(v Value) *ArrayBuffer {
	if v.kind != KindObject {
		return nil
	}
	buf, _ := v.object().jsBuffer.(*ArrayBuffer)
	return buf
}

// bufferSizeArg reads an allocation size, raising the two errors Node raises: a
// non-number size is a type error, and a negative one or one past the maximum is a range
// error. A fractional size truncates, the way Node's validateNumber-then-floor does.
func bufferSizeArg(v Value) int {
	if v.kind != KindNumber {
		Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
			FromGoString("The \"size\" argument must be of type number. Received "+bufferReceivedType(v))))
		return 0
	}
	n := v.AsNumber()
	if math.IsNaN(n) || n < 0 || n > bufferMaxLength {
		// Two things about this message are node's rather than chosen. It joins its bounds
		// with "&&" where the offset messages elsewhere in this file join theirs with "and",
		// and it groups the digits of what it received with underscores where those do not.
		// Both are matched, because a program matching on the text sees what node wrote.
		Throw(NewNodeError("RangeError", "ERR_OUT_OF_RANGE",
			FromGoString("The value of \"size\" is out of range. It must be >= 0 && <= "+
				strconv.Itoa(bufferMaxLength)+". Received "+addNumericSeparator(inspectNumber(n)))))
		return 0
	}
	return int(n)
}

// bufferReceivedType names an argument's type the way Node's ERR_INVALID_ARG_TYPE message
// does, so a program matching on the message text sees what it expects.
func bufferReceivedType(v Value) string {
	switch v.kind {
	case KindUndefined:
		return "undefined"
	case KindNull:
		return "null"
	case KindString:
		return "type string ('" + v.str().ToGoString() + "')"
	case KindNumber:
		return "type number (" + inspectNumber(v.AsNumber()) + ")"
	case KindBool:
		return "type boolean (" + strconv.FormatBool(v.AsBool()) + ")"
	case KindFunc:
		return "function"
	}
	return "an instance of " + ToString(v.Get(FromGoString("constructor")).Get(FromGoString("name"))).ToGoString()
}

// bufferFromCall is the body of `Buffer(x)` and `new Buffer(x)`, the deprecated
// constructor form. Node still answers it: a number allocates that many zeroed bytes and
// anything else routes to Buffer.from.
func bufferFromCall(args []Value) Value {
	if v := Arg(args, 0); v.kind == KindNumber {
		return newSizedBuffer(bufferSizeArg(v)).ToValue()
	}
	return bufferFrom(Arg(args, 0), Arg(args, 1), Arg(args, 2))
}

// bufferFrom is Buffer.from over its five overloads: a string with an encoding, an
// ArrayBuffer with an optional window, another Buffer or typed array, an array-like of
// byte values, and an object that carries a length. The second and third parameters mean
// different things per overload, which is Node's signature and not a simplification here.
func bufferFrom(src, second, third Value) Value {
	switch {
	case src.kind == KindString:
		return NodeBufferFromGo(bufferEncode(src.str(), bufferEncodingArg(second))).ToValue()

	case src.kind == KindObject && bufferArrayBufferOf(src) != nil:
		// An ArrayBuffer is shared rather than copied: Buffer.from(arrayBuffer) is a view,
		// so a write through the buffer shows in the Buffer and the other way round.
		buf := bufferArrayBufferOf(src)
		off := bufferOffsetArg(second, 0, len(buf.data), "byteOffset")
		length := len(buf.data) - off
		if third.kind != KindUndefined {
			length = bufferOffsetArg(third, length, len(buf.data)-off, "length")
		}
		view := Uint8ArrayView(buf, float64(off), float64(length))
		view.nodeBuffer = true
		return view.ToValue()

	case src.kind == KindObject && src.object().jsTyped != nil:
		// Another Buffer or typed array is copied, one element to one byte, so the result
		// owns its storage and no write through either shows in the other.
		t := src.object().jsTyped
		out := newSizedBuffer(t.jsTypedLen())
		d := out.live()
		for i := range d {
			d[i] = toUint8(ToNumber(t.jsTypedAt(i)))
		}
		return out.ToValue()

	case src.kind == KindArray:
		elems := src.object().elems
		out := newSizedBuffer(len(elems))
		d := out.live()
		for i, e := range elems {
			if isHole(e) {
				continue
			}
			d[i] = toUint8(ToNumber(e))
		}
		return out.ToValue()

	case src.kind == KindObject:
		// The array-like overload: any object carrying a length is read index by index, the
		// shape { length: 2, 0: 1, 1: 2 } Node accepts.
		if n := src.Get(FromGoString("length")); n.kind != KindUndefined {
			count := typedLen(ToNumber(n))
			out := newSizedBuffer(count)
			d := out.live()
			for i := range d {
				d[i] = toUint8(ToNumber(src.Get(FromGoString(strconv.Itoa(i)))))
			}
			return out.ToValue()
		}
	}
	Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
		FromGoString("The first argument must be of type string or an instance of Buffer, ArrayBuffer, or Array or an Array-like Object. Received "+bufferReceivedType(src))))
	return Undefined
}

// bufferConcat is Buffer.concat: one allocation sized to the total and a copy of each
// member into it. A totalLength shorter than the members truncates and one longer leaves
// the tail zeroed, which is what Node does rather than treating either as an error.
func bufferConcat(list, total Value) Value {
	if list.kind != KindArray {
		Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
			FromGoString("The \"list\" argument must be an instance of Array. Received "+bufferReceivedType(list))))
		return Undefined
	}
	parts := make([][]byte, 0, len(list.object().elems))
	sum := 0
	for _, e := range list.object().elems {
		b, ok := asByteView(e)
		if !ok {
			Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
				FromGoString("The \"list[i]\" argument must be an instance of Buffer or Uint8Array. Received "+bufferReceivedType(e))))
			return Undefined
		}
		parts = append(parts, b)
		sum += len(b)
	}
	if total.kind != KindUndefined {
		sum = bufferSizeArg(total)
	}
	out := newSizedBuffer(sum)
	d := out.live()
	at := 0
	for _, p := range parts {
		if at >= len(d) {
			break
		}
		at += copy(d[at:], p)
	}
	return out.ToValue()
}

// bufferByteLengthOf is Buffer.byteLength over what it accepts: a string measured in the
// given encoding, and any byte-carrying object measured by its window.
func bufferByteLengthOf(v, enc Value) int {
	if v.kind == KindString {
		return bufferByteLength(v.str(), bufferEncodingArg(enc))
	}
	if b, ok := asByteView(v); ok {
		return len(b)
	}
	if v.kind == KindObject && v.object().jsBuffer != nil {
		return len(v.object().jsBuffer.jsBufferBytes())
	}
	Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
		FromGoString("The \"string\" argument must be of type string or an instance of Buffer or ArrayBuffer. Received "+bufferReceivedType(v))))
	return 0
}

// requireBufferArg reads an argument that has to carry bytes, raising Node's type error
// naming the parameter when it does not.
func requireBufferArg(v Value, name string) *Uint8Array {
	if u := asNodeBuffer(v); u != nil {
		return u
	}
	if b, ok := asByteView(v); ok {
		return NodeBufferFromGo(b)
	}
	Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
		FromGoString("The \""+name+"\" argument must be an instance of Buffer or Uint8Array. Received "+bufferReceivedType(v))))
	return nil
}

// bufferOffsetArg reads an offset or a length that clamps into [0, max], the coercion the
// slicing members take. A missing argument takes the default and a fractional or
// not-a-number one truncates toward zero.
func bufferOffsetArg(v Value, def, max int, _ string) int {
	if v.kind == KindUndefined {
		return def
	}
	n := ToNumber(v)
	if math.IsNaN(n) || n < 0 {
		return 0
	}
	if n > float64(max) {
		return max
	}
	return int(n)
}

// bufferBound reads one end of a window, honouring a negative index as an offset from the
// end the way slice, subarray, toString and fill all do, and clamping into range.
func bufferBound(v Value, def, length int) int {
	if v.kind == KindUndefined {
		return def
	}
	n := ToNumber(v)
	if math.IsNaN(n) {
		return 0
	}
	i := int(n)
	if i < 0 {
		i += length
	}
	if i < 0 {
		return 0
	}
	if i > length {
		return length
	}
	return i
}

// nodeBufferMethod builds one Buffer prototype member, the table typedArrayGet consults
// before the typed-array family's. Every member here is either one Node adds or one it
// overrides; a name that is not in it falls through to the family, which is where at,
// map, filter, set, sort, keys, values, entries and the iterator come from.
func nodeBufferMethod(a *Uint8Array, name string) (Value, bool) {
	switch name {
	case "toString":
		return boundMethod("toString", func(args []Value) Value {
			enc := bufferEncodingArg(Arg(args, 0))
			d := a.live()
			start := bufferBound(Arg(args, 1), 0, len(d))
			end := bufferBound(Arg(args, 2), len(d), len(d))
			if end < start {
				end = start
			}
			return StringValue(bufferDecode(d[start:end], enc))
		}), true

	case "toJSON":
		return boundMethod("toJSON", func(args []Value) Value {
			d := a.live()
			elems := make([]Value, len(d))
			for i, c := range d {
				elems[i] = Number(float64(c))
			}
			out := NewObject()
			out.Set(FromGoString("type"), StringValue(FromGoString("Buffer")))
			out.Set(FromGoString("data"), NewArrayValue(elems))
			return out
		}), true

	case "equals":
		return boundMethod("equals", func(args []Value) Value {
			other := requireBufferArg(Arg(args, 0), "otherBuffer")
			return Bool(bytes.Equal(a.live(), other.live()))
		}), true

	case "compare":
		return boundMethod("compare", func(args []Value) Value {
			target := requireBufferArg(Arg(args, 0), "target")
			t := target.live()
			s := a.live()
			ts := bufferOffsetArg(Arg(args, 1), 0, len(t), "targetStart")
			te := bufferOffsetArg(Arg(args, 2), len(t), len(t), "targetEnd")
			ss := bufferOffsetArg(Arg(args, 3), 0, len(s), "sourceStart")
			se := bufferOffsetArg(Arg(args, 4), len(s), len(s), "sourceEnd")
			if te < ts {
				te = ts
			}
			if se < ss {
				se = ss
			}
			return Number(float64(bytes.Compare(s[ss:se], t[ts:te])))
		}), true

	case "copy":
		return boundMethod("copy", func(args []Value) Value {
			target := requireBufferArg(Arg(args, 0), "target")
			t := target.live()
			s := a.live()
			ts := bufferOffsetArg(Arg(args, 1), 0, len(t), "targetStart")
			ss := bufferOffsetArg(Arg(args, 2), 0, len(s), "sourceStart")
			se := bufferOffsetArg(Arg(args, 3), len(s), len(s), "sourceEnd")
			if se < ss {
				se = ss
			}
			// copy is a move, not a copy: Node documents it as memmove and a program uses it
			// to shift a run within one buffer, so an overlapping source and target has to
			// come out right rather than smear.
			return Number(float64(copy(t[ts:], s[ss:se])))
		}), true

	case "write":
		return boundMethod("write", func(args []Value) Value {
			return Number(float64(bufferWriteString(a, args)))
		}), true

	case "fill":
		return boundMethod("fill", func(args []Value) Value {
			d := a.live()
			start := bufferBound(Arg(args, 1), 0, len(d))
			end := bufferBound(Arg(args, 2), len(d), len(d))
			// fill(value, encoding) is the two-argument form, so a string in the second slot
			// is an encoding rather than an offset. Node picks the same way.
			var enc string
			if Arg(args, 1).kind == KindString {
				start, end, enc = 0, len(d), bufferEncodingArg(Arg(args, 1))
			} else {
				enc = bufferEncodingArg(Arg(args, 3))
			}
			bufferFill(a, Arg(args, 0), start, end, enc)
			return a.ToValue()
		}), true

	case "slice", "subarray":
		// Buffer.prototype.slice is subarray: it answers a view over the same bytes rather
		// than the copy the typed-array slice makes. That is the one place a Buffer's
		// aliasing differs from a Uint8Array's, and it is why slice is overridden here.
		return boundMethod(name, func(args []Value) Value {
			n := a.liveLen()
			start := bufferBound(Arg(args, 0), 0, n)
			end := bufferBound(Arg(args, 1), n, n)
			if end < start {
				end = start
			}
			out := &Uint8Array{buffer: a.buffer, byteOffset: a.byteOffset + start, length: end - start, nodeBuffer: true}
			return out.ToValue()
		}), true

	case "indexOf", "includes":
		return boundMethod(name, func(args []Value) Value {
			i := bufferSearch(a, args, false)
			if name == "includes" {
				return Bool(i >= 0)
			}
			return Number(float64(i))
		}), true

	case "lastIndexOf":
		return boundMethod("lastIndexOf", func(args []Value) Value {
			return Number(float64(bufferSearch(a, args, true)))
		}), true

	case "swap16", "swap32", "swap64":
		width := 2
		switch name {
		case "swap32":
			width = 4
		case "swap64":
			width = 8
		}
		return boundMethod(name, func(args []Value) Value {
			d := a.live()
			if len(d)%width != 0 {
				Throw(NewNodeError("RangeError", "ERR_INVALID_BUFFER_SIZE",
					FromGoString("Buffer size must be a multiple of "+strconv.Itoa(width*8)+"-bits")))
				return Undefined
			}
			for i := 0; i < len(d); i += width {
				run := d[i : i+width]
				for lo, hi := 0, width-1; lo < hi; lo, hi = lo+1, hi-1 {
					run[lo], run[hi] = run[hi], run[lo]
				}
			}
			return a.ToValue()
		}), true
	}
	if fn, ok := bufferNumericMethod(a, name); ok {
		return fn, true
	}
	return Undefined, false
}

// bufferFill writes a repeating pattern across a window. A number fills with one byte, a
// string fills with its encoded bytes repeated, and another buffer fills with its bytes
// repeated. An empty pattern zeroes the window, which is what Node does with one rather
// than looping forever.
func bufferFill(a *Uint8Array, v Value, start, end int, enc string) {
	d := a.live()
	if start > len(d) {
		start = len(d)
	}
	if end > len(d) {
		end = len(d)
	}
	if end <= start {
		return
	}
	window := d[start:end]
	var pattern []byte
	switch v.kind {
	case KindString:
		pattern = bufferEncode(v.str(), enc)
	default:
		if b, ok := asByteView(v); ok {
			pattern = b
		} else {
			pattern = []byte{toUint8(ToNumber(v))}
		}
	}
	if len(pattern) == 0 {
		for i := range window {
			window[i] = 0
		}
		return
	}
	for i := range window {
		window[i] = pattern[i%len(pattern)]
	}
}

// bufferWriteString is buf.write, whose signature is the awkward one in the whole class:
// write(string[, offset[, length]][, encoding]), so the encoding may sit in any of the
// three trailing slots and the two numbers in between are optional independently. The
// shape is resolved by looking at what is actually there, which is what Node's own
// implementation does.
func bufferWriteString(a *Uint8Array, args []Value) int {
	s := ToString(Arg(args, 0))
	d := a.live()
	offset, length := 0, -1
	var enc string
	switch {
	case Arg(args, 1).kind == KindString:
		enc = bufferEncodingArg(Arg(args, 1))
	case Arg(args, 2).kind == KindString:
		offset = bufferOffsetArg(Arg(args, 1), 0, len(d), "offset")
		enc = bufferEncodingArg(Arg(args, 2))
	default:
		offset = bufferOffsetArg(Arg(args, 1), 0, len(d), "offset")
		if Arg(args, 2).kind != KindUndefined {
			length = bufferOffsetArg(Arg(args, 2), len(d)-offset, len(d)-offset, "length")
		}
		enc = bufferEncodingArg(Arg(args, 3))
	}
	room := len(d) - offset
	if length >= 0 && length < room {
		room = length
	}
	encoded := bufferEncode(s, enc)
	if len(encoded) > room {
		encoded = encoded[:room]
		if enc == "utf8" {
			// A utf8 write stops on a code-point boundary rather than leaving half a sequence
			// in the buffer, so a string cut short by the window loses its last character
			// whole.
			encoded = utf8TrimPartial(encoded)
		}
	}
	return copy(d[offset:], encoded)
}

// bufferSearch is the byte-run search under indexOf, lastIndexOf and includes. It takes
// the same three shapes of needle Node does, a number, a string with an encoding and
// another buffer, and it answers a byte index rather than an element index, which for a
// Buffer are the same thing.
func bufferSearch(a *Uint8Array, args []Value, last bool) int {
	d := a.live()
	needleVal := Arg(args, 0)
	// The second argument is a byteOffset unless it is a string, in which case it is the
	// encoding and the search starts at the default end.
	var enc string
	from := 0
	if last {
		from = len(d)
	}
	if Arg(args, 1).kind == KindString {
		enc = bufferEncodingArg(Arg(args, 1))
	} else {
		enc = bufferEncodingArg(Arg(args, 2))
		if v := Arg(args, 1); v.kind != KindUndefined {
			n := ToNumber(v)
			if math.IsNaN(n) {
				n = 0
			}
			from = int(n)
			if from < 0 {
				from += len(d)
			}
		}
	}
	var needle []byte
	switch needleVal.kind {
	case KindString:
		needle = bufferEncode(needleVal.str(), enc)
	case KindNumber:
		needle = []byte{toUint8(needleVal.AsNumber())}
	default:
		b, ok := asByteView(needleVal)
		if !ok {
			Throw(NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE",
				FromGoString("The \"value\" argument must be of type number or string or an instance of Buffer or Uint8Array. Received "+bufferReceivedType(needleVal))))
			return -1
		}
		needle = b
	}
	if last {
		if from < 0 {
			return -1
		}
		if from > len(d)-len(needle) {
			from = len(d) - len(needle)
		}
		if from < 0 {
			return -1
		}
		return bytes.LastIndex(d[:from+len(needle)], needle)
	}
	if from < 0 {
		from = 0
	}
	if from > len(d) {
		return -1
	}
	i := bytes.Index(d[from:], needle)
	if i < 0 {
		return -1
	}
	return from + i
}

// bufferNumKind describes one entry in the read*/write* family: how wide it is, which end
// it starts at, and how the bytes turn into a value and back.
type bufferNumKind struct {
	size   int
	little bool
	read   func(b []byte, little bool) Value
	write  func(b []byte, little bool, v Value)
}

// bufferNumKinds is the whole endian-aware numeric family, one entry per method name.
// Node carries a lowercase-u alias for every Uint spelling (readUint8 beside readUInt8),
// and both spellings are registered so a program written either way works.
//
// It is filled on first lookup rather than at package initialization because the entries
// close over the coercions, and the coercions reach a member read, and a member read
// reaches back here: a package-level initializer would be a cycle Go rejects. A program
// that never touches a Buffer never builds the table.
var bufferNumKinds map[string]bufferNumKind

// bufferNumKindFor looks one entry up, building the table on the first call.
func bufferNumKindFor(name string) (bufferNumKind, bool) {
	if bufferNumKinds == nil {
		bufferNumKinds = buildBufferNumKinds()
	}
	k, ok := bufferNumKinds[name]
	return k, ok
}

func buildBufferNumKinds() map[string]bufferNumKind {
	out := map[string]bufferNumKind{}
	// add registers one method under every spelling it has, for both endiannesses when the
	// width makes endianness meaningful.
	add := func(base string, size int, endian bool,
		read func(b []byte, little bool) Value, write func(b []byte, little bool, v Value)) {
		names := []string{base}
		if strings.Contains(base, "UInt") {
			names = append(names, strings.Replace(base, "UInt", "Uint", 1))
		}
		for _, n := range names {
			if !endian {
				out[n] = bufferNumKind{size: size, read: read, write: write}
				continue
			}
			out[n+"LE"] = bufferNumKind{size: size, little: true, read: read, write: write}
			out[n+"BE"] = bufferNumKind{size: size, little: false, read: read, write: write}
		}
	}
	add("readUInt8", 1, false,
		func(b []byte, _ bool) Value { return Number(float64(b[0])) }, nil)
	add("readInt8", 1, false,
		func(b []byte, _ bool) Value { return Number(float64(int8(b[0]))) }, nil)
	add("readUInt16", 2, true,
		func(b []byte, le bool) Value { return Number(float64(bufferOrder(le).Uint16(b))) }, nil)
	add("readInt16", 2, true,
		func(b []byte, le bool) Value { return Number(float64(int16(bufferOrder(le).Uint16(b)))) }, nil)
	add("readUInt32", 4, true,
		func(b []byte, le bool) Value { return Number(float64(bufferOrder(le).Uint32(b))) }, nil)
	add("readInt32", 4, true,
		func(b []byte, le bool) Value { return Number(float64(int32(bufferOrder(le).Uint32(b)))) }, nil)
	add("readFloat", 4, true,
		func(b []byte, le bool) Value { return Number(float64(math.Float32frombits(bufferOrder(le).Uint32(b)))) }, nil)
	add("readDouble", 8, true,
		func(b []byte, le bool) Value { return Number(math.Float64frombits(bufferOrder(le).Uint64(b))) }, nil)
	add("readBigUInt64", 8, true,
		func(b []byte, le bool) Value { return BigIntFromBig(new(big.Int).SetUint64(bufferOrder(le).Uint64(b))) }, nil)
	add("readBigInt64", 8, true,
		func(b []byte, le bool) Value { return BigIntFromBig(big.NewInt(int64(bufferOrder(le).Uint64(b)))) }, nil)

	add("writeUInt8", 1, false, nil,
		func(b []byte, _ bool, v Value) { b[0] = toUint8(ToNumber(v)) })
	add("writeInt8", 1, false, nil,
		func(b []byte, _ bool, v Value) { b[0] = byte(int8(toInt32(ToNumber(v)))) })
	add("writeUInt16", 2, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint16(b, uint16(toInt32(ToNumber(v)))) })
	add("writeInt16", 2, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint16(b, uint16(int16(toInt32(ToNumber(v))))) })
	add("writeUInt32", 4, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint32(b, uint32(toInt32(ToNumber(v)))) })
	add("writeInt32", 4, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint32(b, uint32(toInt32(ToNumber(v)))) })
	add("writeFloat", 4, true, nil,
		func(b []byte, le bool, v Value) {
			bufferOrder(le).PutUint32(b, math.Float32bits(float32(ToNumber(v))))
		})
	add("writeDouble", 8, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint64(b, math.Float64bits(ToNumber(v))) })
	add("writeBigUInt64", 8, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint64(b, bigIntLow64(dataViewBigArg(v))) })
	add("writeBigInt64", 8, true, nil,
		func(b []byte, le bool, v Value) { bufferOrder(le).PutUint64(b, bigIntLow64(dataViewBigArg(v))) })
	return out
}

// bufferOrder picks the byte order for a method's endianness.
func bufferOrder(little bool) binary.ByteOrder {
	if little {
		return binary.LittleEndian
	}
	return binary.BigEndian
}

// bufferNumericMethod builds one read* or write* member, including the two
// variable-width families readUIntLE/readIntLE and their write mirrors, which take the
// byte count as an argument rather than carrying it in the name.
func bufferNumericMethod(a *Uint8Array, name string) (Value, bool) {
	if k, ok := bufferNumKindFor(name); ok {
		if k.read != nil {
			return boundMethod(name, func(args []Value) Value {
				return k.read(bufferWindow(a, name, Arg(args, 0), k.size), k.little)
			}), true
		}
		return boundMethod(name, func(args []Value) Value {
			off := bufferReadOffset(Arg(args, 1))
			k.write(bufferWindow(a, name, Arg(args, 1), k.size), k.little, Arg(args, 0))
			return Number(float64(off + k.size))
		}), true
	}
	switch name {
	case "readUIntLE", "readUIntBE", "readUintLE", "readUintBE", "readIntLE", "readIntBE":
		little := strings.HasSuffix(name, "LE")
		signed := strings.Contains(name, "readInt")
		return boundMethod(name, func(args []Value) Value {
			size := bufferByteCountArg(Arg(args, 1))
			w := bufferWindow(a, name, Arg(args, 0), size)
			return Number(bufferReadVarInt(w, little, signed))
		}), true

	case "writeUIntLE", "writeUIntBE", "writeUintLE", "writeUintBE", "writeIntLE", "writeIntBE":
		little := strings.HasSuffix(name, "LE")
		return boundMethod(name, func(args []Value) Value {
			off := bufferReadOffset(Arg(args, 1))
			size := bufferByteCountArg(Arg(args, 2))
			w := bufferWindow(a, name, Arg(args, 1), size)
			bufferWriteVarInt(w, little, ToNumber(Arg(args, 0)))
			return Number(float64(off + size))
		}), true
	}
	return Undefined, false
}

// bufferReadOffset coerces an offset argument, defaulting a missing one to zero the way
// every numeric accessor does.
func bufferReadOffset(v Value) int {
	if v.kind == KindUndefined {
		return 0
	}
	n := ToNumber(v)
	if math.IsNaN(n) {
		return 0
	}
	return int(n)
}

// bufferByteCountArg reads the byteLength argument of the variable-width accessors, which
// Node restricts to one through six bytes: six is the widest run that still fits a
// double's integer range exactly.
func bufferByteCountArg(v Value) int {
	n := bufferReadOffset(v)
	if n < 1 || n > 6 {
		Throw(NewNodeError("RangeError", "ERR_OUT_OF_RANGE",
			FromGoString("The value of \"byteLength\" is out of range. It must be >= 1 and <= 6. Received "+strconv.Itoa(n))))
		return 1
	}
	return n
}

// bufferWindow is the bounds check the whole numeric family shares. Unlike a typed
// array's clamping index it raises: a read that runs off the end of the window is the
// caller's arithmetic being wrong, and answering zero for it is how a parser comes to
// trust bytes that were never there.
func bufferWindow(a *Uint8Array, name string, offv Value, size int) []byte {
	d := a.live()
	off := bufferReadOffset(offv)
	if off < 0 || off+size > len(d) {
		limit := len(d) - size
		if limit < 0 {
			limit = 0
		}
		Throw(NewNodeError("RangeError", "ERR_OUT_OF_RANGE",
			FromGoString("The value of \"offset\" is out of range. It must be >= 0 and <= "+
				strconv.Itoa(limit)+". Received "+strconv.Itoa(off))))
		return make([]byte, size)
	}
	_ = name
	return d[off : off+size]
}

// bufferReadVarInt reads a one-to-six byte integer, signed or not. The sign extension is
// done over the top bit of the run's widest byte rather than over a fixed width, which is
// what makes readIntLE(0, 3) of ff ff ff answer -1.
func bufferReadVarInt(b []byte, little, signed bool) float64 {
	var n uint64
	if little {
		for i := len(b) - 1; i >= 0; i-- {
			n = n<<8 | uint64(b[i])
		}
	} else {
		for _, c := range b {
			n = n<<8 | uint64(c)
		}
	}
	if !signed {
		return float64(n)
	}
	bits := uint(len(b) * 8)
	if n&(1<<(bits-1)) != 0 {
		return float64(int64(n) - int64(1)<<bits)
	}
	return float64(n)
}

// bufferWriteVarInt writes a one-to-six byte integer, taking the low bytes of the value
// in the requested order. A negative value writes its two's complement, which is the same
// low-byte run, so the signed and unsigned writes share this one body.
func bufferWriteVarInt(b []byte, little bool, v float64) {
	n := uint64(int64(v))
	if little {
		for i := range b {
			b[i] = byte(n)
			n >>= 8
		}
		return
	}
	for i := len(b) - 1; i >= 0; i-- {
		b[i] = byte(n)
		n >>= 8
	}
}

// bufferInspectMaxBytes is how many bytes an inspect of a Buffer prints before it says
// how many it left out. It is node's buffer.INSPECT_MAX_BYTES, a limit of its own rather
// than the maxArrayLength every other collection is cut at: passing maxArrayLength to
// inspect does not move it, and setting this does. A program that logs a megabyte buffer
// gets one line either way, and one that wants to see more raises this.
var bufferInspectMaxBytes = 50

// nodeBufferHexRun renders a Buffer's bytes the way an inspect of one shows them, as hex
// pairs separated by spaces with a count of whatever was cut. It is the inside of the
// angle brackets and not the brackets themselves, because an extra property on the buffer
// goes inside them too and the caller assembles both.
func nodeBufferHexRun(a *Uint8Array, maxLen int) string {
	d := a.live()
	shown := d
	if len(shown) > maxLen {
		shown = shown[:maxLen]
	}
	var sb strings.Builder
	const digits = "0123456789abcdef"
	for i, c := range shown {
		if i > 0 {
			sb.WriteByte(' ')
		}
		sb.WriteByte(digits[c>>4])
		sb.WriteByte(digits[c&0x0F])
	}
	if rest := len(d) - len(shown); rest > 0 {
		sb.WriteString(" ... " + strconv.Itoa(rest) + " more byte")
		if rest > 1 {
			sb.WriteString("s")
		}
	}
	return sb.String()
}
