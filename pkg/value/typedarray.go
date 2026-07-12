package value

import (
	"math"
	"unsafe"
)

// TypedArray is bento's runtime representation of a JavaScript numeric typed
// array whose element width the compiler proved: Int8Array, Uint8ClampedArray,
// Int16Array, Uint16Array, Int32Array, Uint32Array, Float32Array, and
// Float64Array (16 §6.3). It wraps a Go slice of the element's fixed-width type,
// the same storage the platform gives that array kind, so a read is a slice index
// and a write is a slice store, with none of the per-element boxing a JavaScript
// engine keeps. It is spelled *TypedArray[T] in generated code the same way a
// dense array is spelled *Array[T], so the two read alike at the use site.
//
// Uint8Array is the one family member kept separate (bytes.go): it stores a Go
// []byte so it can hand its backing slice to a Go function taking []byte across
// the go: boundary with no copy (16 §7.3). The bigint-element arrays
// (BigInt64Array, BigUint64Array) store a *big.Int element and are a later slice.
//
// The store coercion is the one per-element behavior that differs across the
// family: a write into an Int8Array wraps modulo 256 into signed range, a write
// into a Uint8ClampedArray clamps to 0 to 255, a write into a Float32Array rounds
// to single precision, and so on. The header carries that coercion as a function
// so the generic core stays one type; a read always widens the stored element
// back to a Number, which every member does the same way.
//
// A typed array is a view, not the storage: it records the ArrayBuffer it reads,
// the byte offset it starts at, and an element slice that aliases the buffer's
// bytes at that offset (section 6.2 and §6.3). The data slice is built with
// unsafe.Slice over the buffer's bytes, so an element read and write go straight
// through it with no per-element packing, and two views over one buffer, even of
// different element widths, observe each other's writes because they alias the
// same run of bytes. The buffer allocates eight-byte aligned and the byte offset
// the spec requires to be a multiple of the element width keeps every element on a
// naturally aligned address, and the platform's little-endian layout is the byte
// order the buffer exposes. Keeping data as a live view is what lets every
// element-access method below stay a plain slice operation while still aliasing
// the shared buffer.
type TypedArray[T typedElem] struct {
	buffer     *ArrayBuffer
	byteOffset int
	data       []T
	coerce     func(float64) T
}

// typedElem is the set of Go element types a numeric typed array stores. Every
// member is convertible to float64, which is what lets a read widen the stored
// element back to the Number JavaScript hands out.
type typedElem interface {
	~int8 | ~uint8 | ~int16 | ~uint16 | ~int32 | ~uint32 | ~float32 | ~float64
}

// newTypedArray builds a zeroed typed array of the given length with the store
// coercion its element kind uses, the shared body of the per-kind New
// constructors. It allocates a fresh ArrayBuffer sized to hold the elements and
// views it from byte offset zero, so the array owns its storage but reaches it
// through the shared view path. The length is a Number, so it arrives as a float64
// and is truncated toward zero the way ToIndex does; a negative or not-a-number
// length clamps to zero, matching the covered subset the byte buffer documents.
func newTypedArray[T typedElem](length float64, coerce func(float64) T) *TypedArray[T] {
	n := typedLen(length)
	buf := NewArrayBuffer(float64(n * elemBytes[T]()))
	return newTypedArrayView(buf, 0, n, coerce)
}

// newTypedArrayView builds a typed array that views an existing buffer from a byte
// offset for a count of elements, the shared body of every typed-array
// constructor. The data slice aliases the buffer's bytes, so a write through this
// view shows through every other view of the same buffer. It is the one place the
// unsafe aliasing is formed, so the byte offset and length the callers pass are the
// only inputs that decide what the view sees.
func newTypedArrayView[T typedElem](buf *ArrayBuffer, byteOffset, length int, coerce func(float64) T) *TypedArray[T] {
	return &TypedArray[T]{
		buffer:     buf,
		byteOffset: byteOffset,
		data:       viewSlice[T](buf, byteOffset, length),
		coerce:     coerce,
	}
}

// typedArrayOf builds a typed array from a list of JavaScript numbers with the
// store coercion its element kind uses, the shared body of the per-kind Of
// constructors. It allocates a fresh buffer of the right size and coerces each
// element into it exactly as an assignment into an element would, so the array owns
// its storage.
func typedArrayOf[T typedElem](coerce func(float64) T, elems ...float64) *TypedArray[T] {
	a := newTypedArray(float64(len(elems)), coerce)
	for i, e := range elems {
		a.data[i] = coerce(e)
	}
	return a
}

// elemBytes is the byte width of a typed array's element type, read from the Go
// type so the buffer allocation and the byte-offset arithmetic agree with the
// element slice the view aliases.
func elemBytes[T typedElem]() int {
	var zero T
	return int(unsafe.Sizeof(zero))
}

// viewSlice forms the element slice a typed array reads, an unsafe alias over the
// buffer's bytes starting at the byte offset for length elements. It is the aliasing
// step that lets two views over one buffer observe each other's writes: the slice
// header points into the buffer's own storage rather than a copy. A zero length
// needs no storage and returns nil, which also avoids indexing a nil or too-short
// buffer. The buffer keeps its bytes eight-byte aligned and the byte offset is a
// multiple of the element width, so the first element lands on a naturally aligned
// address.
func viewSlice[T typedElem](buf *ArrayBuffer, byteOffset, length int) []T {
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&buf.data[byteOffset])), length)
}

// Len is the array's length in elements, a Number to match the type the checker
// gives the .length property and to compose with the numeric path with no
// conversion at the use site.
func (a *TypedArray[T]) Len() float64 { return float64(len(a.data)) }

// At reads the element a JavaScript index expression a[i] selects, widened to the
// Number a typed-array read hands out. The index is a Number, so it arrives as a
// float64 and truncates toward zero. An index outside the array reads as 0 rather
// than undefined, matching the covered subset the byte buffer's At documents.
func (a *TypedArray[T]) At(i float64) float64 {
	idx := typedIndex(i)
	if idx >= 0 && idx < len(a.data) {
		return float64(a.data[idx])
	}
	return 0
}

// SetAt writes the element a JavaScript assignment a[i] = v stores, coercing the
// value with the element kind's store rule so a number outside the element's
// range wraps or clamps exactly as JavaScript does. A write past the end of the
// array is ignored, matching JavaScript, which silently drops an out-of-range
// typed-array element assignment rather than growing the array.
func (a *TypedArray[T]) SetAt(i float64, v float64) {
	idx := typedIndex(i)
	if idx >= 0 && idx < len(a.data) {
		a.data[idx] = a.coerce(v)
	}
}

// AtI reads the element at a Go int index, the integer-index form of At the
// lowerer emits when the checker proved the index expression is an integer. The
// float truncation At runs on its Number index is then dead work, so this form
// takes the index already narrowed. The bounds check and the out-of-range 0 are
// the same as At, so the two reads agree on every index; only the index type
// differs, which is what keeps a proven-integer index a native slice index rather
// than a float that round trips through typedIndex on every access.
func (a *TypedArray[T]) AtI(i int) float64 {
	if i >= 0 && i < len(a.data) {
		return float64(a.data[i])
	}
	return 0
}

// SetAtI writes v at a Go int index, the integer-index form of SetAt. It coerces
// the value with the element kind's store rule and drops an out-of-range write
// exactly as SetAt does, so the only difference is that the index arrives already
// narrowed to an int rather than truncated from a Number here.
func (a *TypedArray[T]) SetAtI(i int, v float64) {
	if i >= 0 && i < len(a.data) {
		a.data[i] = a.coerce(v)
	}
}

// Data returns the backing slice, the storage a read and a write share. The
// lowerer indexes it directly, bData[i], at an access it proved stays inside the
// array, so the read skips the At bounds branch and the Number round trip and an
// integer store skips the coerce function pointer: a proven-integer value goes
// straight into the element through a Go conversion that wraps to the element
// width exactly as the store coercion does. The slice header aliases the array's
// own storage, so a write through the returned slice shows through the array and
// through any other reference to it, the same as a write through SetAt. A typed
// array never grows or reallocates its backing, so the header stays valid for the
// life of the array; the caller only takes this path when it proved the index in
// range, so the Go slice bounds check it still carries never trips.
func (a *TypedArray[T]) Data() []T { return a.data }

// The per-kind constructors wire the element type and its store coercion. Each is
// a one-liner over the shared bodies so generated code names a plain
// value.NewInt32Array(n) or value.Int32ArrayOf(1, 2, 3) rather than spell the
// coercion at the call site.

func NewInt8Array(length float64) *TypedArray[int8]  { return newTypedArray(length, toInt8) }
func Int8ArrayOf(elems ...float64) *TypedArray[int8] { return typedArrayOf(toInt8, elems...) }

func NewUint8ClampedArray(length float64) *TypedArray[uint8] {
	return newTypedArray(length, toUint8Clamped)
}
func Uint8ClampedArrayOf(elems ...float64) *TypedArray[uint8] {
	return typedArrayOf(toUint8Clamped, elems...)
}

func NewInt16Array(length float64) *TypedArray[int16]  { return newTypedArray(length, toInt16) }
func Int16ArrayOf(elems ...float64) *TypedArray[int16] { return typedArrayOf(toInt16, elems...) }

func NewUint16Array(length float64) *TypedArray[uint16]  { return newTypedArray(length, toUint16) }
func Uint16ArrayOf(elems ...float64) *TypedArray[uint16] { return typedArrayOf(toUint16, elems...) }

func NewInt32Array(length float64) *TypedArray[int32]  { return newTypedArray(length, toInt32) }
func Int32ArrayOf(elems ...float64) *TypedArray[int32] { return typedArrayOf(toInt32, elems...) }

func NewUint32Array(length float64) *TypedArray[uint32]  { return newTypedArray(length, toUint32) }
func Uint32ArrayOf(elems ...float64) *TypedArray[uint32] { return typedArrayOf(toUint32, elems...) }

func NewFloat32Array(length float64) *TypedArray[float32]  { return newTypedArray(length, toFloat32) }
func Float32ArrayOf(elems ...float64) *TypedArray[float32] { return typedArrayOf(toFloat32, elems...) }

func NewFloat64Array(length float64) *TypedArray[float64]  { return newTypedArray(length, toFloat64) }
func Float64ArrayOf(elems ...float64) *TypedArray[float64] { return typedArrayOf(toFloat64, elems...) }

// typedLen truncates a JavaScript length Number to a Go element count, clamping a
// negative or not-a-number length to zero. It is the length rule the per-kind New
// constructors share, the same one NewUint8Array applies.
func typedLen(length float64) int {
	n := int(length) // ToInteger truncates toward zero.
	if length != length || n < 0 {
		n = 0
	}
	return n
}

// typedIndex truncates a JavaScript index Number to a Go slice index, sending NaN
// to 0 the way ToIntegerOrInfinity does. The caller bounds-checks the result, so
// an out-of-range index reads as 0 or drops a write rather than panic.
func typedIndex(i float64) int {
	if i != i {
		return 0
	}
	return int(i) // JavaScript ToInteger truncates toward zero.
}

// wrapMod reduces a JavaScript number into the range [0, mod) with ECMAScript's
// truncate-then-modulo step, the shared core of ToInt8, ToUint8, ToInt16,
// ToUint16, ToInt32, and ToUint32 (7.1.6 through 7.1.13). A not-a-number or
// infinite value becomes 0; any other number is truncated toward zero and reduced
// modulo mod, with a negative remainder folded back up into range. The modulus is
// a power of two no larger than 2^32, all exactly representable as a float64, so
// the reduction is exact for every Number that carries integer precision.
func wrapMod(f float64, mod float64) float64 {
	if f != f || math.IsInf(f, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(f), mod)
	if m < 0 {
		m += mod
	}
	return m
}

// toInt8 is ECMAScript ToInt8: reduce modulo 256, then read the top half of the
// range as negative, so 128 stores -128 and -1 stores -1.
func toInt8(f float64) int8 {
	m := wrapMod(f, 256)
	if m >= 128 {
		m -= 256
	}
	return int8(m)
}

// toInt16 is ECMAScript ToInt16: reduce modulo 65536, then read the top half as
// negative.
func toInt16(f float64) int16 {
	m := wrapMod(f, 65536)
	if m >= 32768 {
		m -= 65536
	}
	return int16(m)
}

// toUint16 is ECMAScript ToUint16 (7.1.13): reduce modulo 65536 into 0 to 65535.
func toUint16(f float64) uint16 { return uint16(wrapMod(f, 65536)) }

// toInt32 is ECMAScript ToInt32 (7.1.6): reduce modulo 2^32, then read the top
// half of the range as negative.
func toInt32(f float64) int32 {
	m := wrapMod(f, 4294967296)
	if m >= 2147483648 {
		m -= 4294967296
	}
	return int32(m)
}

// toUint32 is ECMAScript ToUint32 (7.1.7): reduce modulo 2^32 into 0 to 2^32-1.
func toUint32(f float64) uint32 { return uint32(wrapMod(f, 4294967296)) }

// toUint8Clamped is ECMAScript ToUint8Clamp (7.1.11), the Uint8ClampedArray store
// rule: a not-a-number value becomes 0, a value at or below 0 clamps to 0, a value
// at or above 255 clamps to 255, and any value between rounds to the nearest
// integer with ties going to the even neighbor, so 0.5 stores 0 and 1.5 stores 2.
// It is a clamp-and-round rather than the modulo wrap the integer arrays use.
func toUint8Clamped(x float64) uint8 {
	if x != x || x <= 0 {
		return 0
	}
	if x >= 255 {
		return 255
	}
	f := math.Floor(x)
	switch {
	case f+0.5 < x:
		return uint8(f + 1)
	case x < f+0.5:
		return uint8(f)
	case int64(f)%2 == 0: // exactly halfway rounds to the even neighbor.
		return uint8(f)
	default:
		return uint8(f + 1)
	}
}

// toFloat32 is the Float32Array store rule: round the Number to single precision,
// keeping a NaN or infinity, so a read back through At widens the rounded value
// and shows the precision the store dropped.
func toFloat32(v float64) float32 { return float32(v) }

// toFloat64 is the Float64Array store rule: a Float64Array holds the Number
// itself, so the store is the identity.
func toFloat64(v float64) float64 { return v }
