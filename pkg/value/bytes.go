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
// a view: it records the ArrayBuffer it reads, the byte offset it starts at, and its
// byte length. It does not cache the subslice; live forms one over the buffer's
// current storage on each access, so a Uint8Array over a shared ArrayBuffer aliases
// the same bytes as any other view of it and reacts to a detach or a resize of the
// buffer. A byte is the buffer's own element, so the subslice aliases directly with
// no unsafe step. The length is consulted against the buffer's live state through
// liveLen, so a view over a detached buffer reads as zero-length. The remaining view
// methods (subarray, DataView) and the copying methods (set, slice, fill) land in
// later slices; the type carries the buffer now so those grow it in place.
type Uint8Array struct {
	buffer         *ArrayBuffer
	byteOffset     int
	length         int
	lengthTracking bool
}

// liveLen is the view's byte length as of this access, clamped against the buffer's
// current length so a view over a detached buffer, or once resizable buffers land a
// shrunk one, reports zero. A byte is one element wide, so the count is the span.
func (a *Uint8Array) liveLen() int {
	avail := len(a.buffer.data) - a.byteOffset
	if avail < 0 {
		return 0
	}
	// A length-tracking view over a resizable buffer spans from its offset to the
	// buffer's current end, so a resize changes what it reports; a byte is one element
	// wide, so the span is the byte count directly.
	if a.lengthTracking {
		return avail
	}
	if avail < a.length {
		return 0
	}
	return a.length
}

// live forms the byte subslice this view reads right now over the buffer's current
// storage for liveLen bytes, recomputed on each access so a detach or a resize shows
// through immediately. A zero length returns nil rather than index an empty buffer.
func (a *Uint8Array) live() []byte {
	n := a.liveLen()
	if n == 0 {
		return nil
	}
	return a.buffer.data[a.byteOffset : a.byteOffset+n]
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
	n := typedLen(length)
	buf := NewArrayBuffer(float64(n))
	return &Uint8Array{buffer: buf, length: n}
}

// Uint8ArrayOf builds a buffer from a list of JavaScript numbers, the lowering of
// `new Uint8Array([a, b, c])`. Each element is coerced to a byte with ToUint8, so
// a value outside 0 to 255 wraps modulo 256 exactly as an assignment into a
// Uint8Array element does. It allocates a fresh buffer of the right size and fills
// it, so the array owns its storage.
func Uint8ArrayOf(elems ...float64) *Uint8Array {
	a := NewUint8Array(float64(len(elems)))
	d := a.live()
	for i, e := range elems {
		d[i] = toUint8(e)
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
	// A view with no explicit length over a resizable buffer tracks the buffer's
	// length, following a later resize rather than staying pinned at the count here.
	return &Uint8Array{buffer: buf, byteOffset: off, length: n, lengthTracking: len(length) == 0 && buf.resizable}
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
	return &Uint8Array{buffer: &ArrayBuffer{data: b}, length: len(b)}
}

// Len is the buffer's length in bytes. JavaScript's .length is a Number, so it is
// a float64 here to match the type the checker gives the property and to compose
// with the numeric path with no conversion at the use site.
func (a *Uint8Array) Len() float64 { return float64(a.liveLen()) }

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
func (a *Uint8Array) ByteLength() float64 { return float64(a.liveLen()) }

// BytesPerElement is the element width in bytes, the instance BYTES_PER_ELEMENT
// property, one for a byte array. It is a method so a read keeps the receiver
// referenced rather than folding to a literal that would orphan the binding.
func (a *Uint8Array) BytesPerElement() float64 { return 1 }

// Bytes returns the live backing slice, the storage the bridge passes to a Go
// function taking []byte (16 §7.3). It is not a copy: while a Go call runs it may
// read these bytes, and the caller in the bridge decides whether a copy is needed
// before handing them over, so a Go API that retains the slice never aliases
// bento's buffer by surprise.
func (a *Uint8Array) Bytes() []byte { return a.live() }

// Floats widens every byte to the Number a read hands out, the source a fresh
// typed array copies when it is constructed from a Uint8Array. It allocates a new
// slice so the copy does not alias the source, matching the from-a-typed-array
// constructor's fresh-buffer rule.
func (a *Uint8Array) Floats() []float64 {
	d := a.live()
	out := make([]float64, len(d))
	for i, b := range d {
		out[i] = float64(b)
	}
	return out
}

// At reads the byte a JavaScript index expression a[i] selects, as a Number in the
// range 0 to 255. Only a canonical integer index inside the buffer names a byte; an
// out-of-range or non-canonical index reads as 0 here rather than the undefined the
// spec gives, the covered subset for the numeric read path, since At's result is a
// Number. A read that flows into a dynamic slot takes GetIndex, which answers
// undefined for those indices.
func (a *Uint8Array) At(i float64) float64 {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		return float64(d[idx])
	}
	return 0
}

// SetAt writes the byte a JavaScript assignment a[i] = v stores. The value is
// coerced to a byte with ToUint8, so a number outside 0 to 255 wraps modulo 256
// exactly as JavaScript does for a Uint8Array element. Only a canonical integer
// index inside the buffer names a byte; a write to an out-of-range or non-canonical
// index is dropped, the no-op the spec requires rather than growing the buffer.
func (a *Uint8Array) SetAt(i float64, v float64) {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		d[idx] = toUint8(v)
	}
}

// GetIndex reads the byte a JavaScript index selects as a boxed Value, the form a
// Uint8Array read takes when it flows into a dynamic slot. It answers the byte as a
// Number for a canonical in-range index and the undefined singleton for an
// out-of-range or non-canonical one, so a[100] and a[1.5] read as undefined the way
// the spec requires, which the numeric At cannot express.
func (a *Uint8Array) GetIndex(i float64) Value {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		return Number(float64(d[idx]))
	}
	return Undefined
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
