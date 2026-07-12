package value

import "math"

// Uint8Array is bento's runtime representation of a JavaScript Uint8Array, the
// byte-buffer type a Go []byte projects to across the go: boundary (16 §6.3 and
// §7.3). It wraps a Go []byte, the exact storage a Go []byte already uses, so a
// crossing into a Go function taking []byte hands the bytes over with at most one
// copy and the value model's own collector owns the buffer (10 §6 and §7). A
// Uint8Array is spelled as a *Uint8Array in generated code the same way a typed
// array is spelled *Array[T], so the methods below can grow without changing how
// the value is named.
//
// This slice implements the dense core the boundary needs: construction from a
// length, from a JavaScript number list, and from a Go slice, the .length as a
// Number, indexed reads and writes with JavaScript's byte coercion, and the
// backing slice the bridge passes to Go. Like the numeric family, a Uint8Array is
// a view: it records the ArrayBuffer it reads and the byte offset it starts at, and
// its bytes slice is a subslice of the buffer's storage, so a Uint8Array over a
// shared ArrayBuffer aliases the same bytes as any other view of it. A byte is the
// buffer's own element, so the subslice aliases directly with no unsafe step. The
// remaining view methods (subarray, DataView) and the copying methods (set, slice,
// fill) land in later slices; the type carries the buffer now so those grow it in
// place.
type Uint8Array struct {
	buffer     *ArrayBuffer
	byteOffset int
	bytes      []byte
}

// NewUint8Array builds a zeroed buffer of the given length, the lowering of
// `new Uint8Array(n)`. It allocates a fresh ArrayBuffer sized to the length and
// views the whole of it, so the array owns its storage but exposes a buffer like
// every other typed array. The length is a Number in JavaScript, so it arrives as
// a float64 and is truncated toward zero the way ToIndex does. A negative or
// not-a-number length clamps to zero here rather than throwing; the RangeError
// JavaScript raises for a negative length is a later slice, and the covered subset
// passes a valid length.
func NewUint8Array(length float64) *Uint8Array {
	buf := NewArrayBuffer(float64(typedLen(length)))
	return &Uint8Array{buffer: buf, bytes: buf.data}
}

// Uint8ArrayOf builds a buffer from a list of JavaScript numbers, the lowering of
// `new Uint8Array([a, b, c])`. Each element is coerced to a byte with ToUint8, so
// a value outside 0 to 255 wraps modulo 256 exactly as an assignment into a
// Uint8Array element does. It allocates a fresh buffer of the right size and fills
// it, so the array owns its storage.
func Uint8ArrayOf(elems ...float64) *Uint8Array {
	a := NewUint8Array(float64(len(elems)))
	for i, e := range elems {
		a.bytes[i] = toUint8(e)
	}
	return a
}

// Uint8ArrayView builds a Uint8Array that views an existing ArrayBuffer, the
// lowering of new Uint8Array(buffer, byteOffset, length). The byte offset defaults
// to zero and the length, when omitted, runs from the offset to the end of the
// buffer. The bytes slice is a subslice of the buffer's storage, so the view
// observes writes made through the buffer or through any other view of it. A byte
// offset or length past the buffer clamps to what it holds, the covered subset the
// RangeError is a later slice of.
func Uint8ArrayView(buf *ArrayBuffer, byteOffset float64, length ...float64) *Uint8Array {
	off := typedLen(byteOffset)
	if off > len(buf.data) {
		off = len(buf.data)
	}
	var n int
	if len(length) > 0 {
		n = typedLen(length[0])
	} else {
		n = len(buf.data) - off
	}
	if max := len(buf.data) - off; n > max {
		n = max
	}
	return &Uint8Array{buffer: buf, byteOffset: off, bytes: buf.data[off : off+n]}
}

// Uint8ArrayFromGo wraps a Go []byte as a Uint8Array, the Go-to-bento crossing of
// a []byte return (16 §7.3). It adopts the slice rather than copying, because the
// bridge has already decided on the copy-versus-share question by the time it
// calls this: when Go may keep or mutate the bytes after return the bridge passes
// a copy and this adopts that copy, and when the return is bento's to own the
// bridge passes the slice itself. Either way the buffer is bento's after the call.
// The adopted slice is wrapped in an ArrayBuffer so the array exposes a buffer like
// every other typed array.
func Uint8ArrayFromGo(b []byte) *Uint8Array {
	return &Uint8Array{buffer: &ArrayBuffer{data: b}, bytes: b}
}

// Len is the buffer's length in bytes. JavaScript's .length is a Number, so it is
// a float64 here to match the type the checker gives the property and to compose
// with the numeric path with no conversion at the use site.
func (a *Uint8Array) Len() float64 { return float64(len(a.bytes)) }

// Buffer is the ArrayBuffer the view aliases, the .buffer getter, the same backing
// store every other view of the buffer holds so an identity comparison of two
// views' buffers holds.
func (a *Uint8Array) Buffer() *ArrayBuffer { return a.buffer }

// ByteOffset is the byte the view starts at within its buffer, the .byteOffset
// getter, a Number to match the property's type.
func (a *Uint8Array) ByteOffset() float64 { return float64(a.byteOffset) }

// ByteLength is the view's span in bytes, the .byteLength getter. A byte is the
// element, so the span equals the element count and Len reports the same Number,
// but both getters exist because the numeric family separates the two.
func (a *Uint8Array) ByteLength() float64 { return float64(len(a.bytes)) }

// Bytes returns the live backing slice, the storage the bridge passes to a Go
// function taking []byte (16 §7.3). It is not a copy: while a Go call runs it may
// read these bytes, and the caller in the bridge decides whether a copy is needed
// before handing them over, so a Go API that retains the slice never aliases
// bento's buffer by surprise.
func (a *Uint8Array) Bytes() []byte { return a.bytes }

// Floats widens every byte to the Number a read hands out, the source a fresh
// typed array copies when it is constructed from a Uint8Array. It allocates a new
// slice so the copy does not alias the source, matching the from-a-typed-array
// constructor's fresh-buffer rule.
func (a *Uint8Array) Floats() []float64 {
	out := make([]float64, len(a.bytes))
	for i, b := range a.bytes {
		out[i] = float64(b)
	}
	return out
}

// At reads the byte a JavaScript index expression a[i] selects, as a Number in the
// range 0 to 255. The index is a Number, so it arrives as a float64 and truncates
// toward zero the way a JavaScript index does. An index outside the buffer reads
// as 0 rather than undefined, matching the covered subset the typed Array.At
// documents: the programs that index a buffer do so within its bounds, and the
// noUncheckedIndexedAccess shape that types the read as number | undefined is a
// later slice.
func (a *Uint8Array) At(i float64) float64 {
	idx := int(i) // JavaScript ToInteger truncates toward zero.
	if i != i {   // NaN truncates to 0, matching ToIntegerOrInfinity.
		idx = 0
	}
	if idx >= 0 && idx < len(a.bytes) {
		return float64(a.bytes[idx])
	}
	return 0
}

// SetAt writes the byte a JavaScript assignment a[i] = v stores. The value is
// coerced to a byte with ToUint8, so a number outside 0 to 255 wraps modulo 256
// exactly as JavaScript does for a Uint8Array element. A write past the end of the
// buffer is ignored, matching JavaScript, which silently drops an out-of-range
// typed-array element assignment rather than growing the buffer.
func (a *Uint8Array) SetAt(i float64, v float64) {
	idx := int(i)
	if i != i {
		idx = 0
	}
	if idx >= 0 && idx < len(a.bytes) {
		a.bytes[idx] = toUint8(v)
	}
}

// toUint8 is ECMAScript ToUint8 (7.1.10): a not-a-number or infinite value becomes
// 0, and any other number is truncated toward zero and reduced modulo 256 into the
// range 0 to 255. This is the coercion a store into a Uint8Array element applies,
// so 256 stores 0, -1 stores 255, and 3.9 stores 3.
func toUint8(f float64) byte {
	if f != f || math.IsInf(f, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(f), 256)
	if m < 0 {
		m += 256
	}
	return byte(m)
}
