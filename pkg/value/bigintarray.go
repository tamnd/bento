package value

import (
	"math/big"
	"unsafe"
)

// BigIntArray is bento's runtime representation of a BigInt64Array or a
// BigUint64Array, the two typed arrays whose element is a bigint rather than a
// Number (16 §6.3). Each element is a fixed 64-bit two's-complement integer, so the
// storage is a Go []int64 or []uint64, and a read widens the element to the
// arbitrary-precision *big.Int a JavaScript bigint is while a write truncates the
// bigint to 64 bits the way BigInt.asIntN(64) and asUintN(64) do. It is the bigint
// sibling of the numeric TypedArray: the numeric family reads and writes through
// float64, which a bigint cannot ride, so the bigint pair takes its own header that
// reads and writes through *big.Int.
//
// Like every other family member a BigIntArray is a view, not the storage: it
// records the ArrayBuffer it reads, the byte offset it starts at, and its element
// length. It does not cache an element slice; live forms one with unsafe.Slice over
// the buffer's current bytes on each access, so two views over one buffer observe
// each other's writes, and because both bigint elements are eight bytes wide a
// bigint view aliases a Float64Array view of the same buffer byte for byte. The
// length is consulted against the buffer's live state through liveLen, the same
// clamp the numeric family takes, so a view over a detached buffer reads as
// zero-length. The store and load functions carry the per-kind truncation and
// widening so the generic core stays one type.
type BigIntArray[T bigArrayElem] struct {
	buffer     *ArrayBuffer
	byteOffset int
	length     int
	store      func(*big.Int) T
	load       func(T) *big.Int
}

// bigArrayElem is the set of Go element types a bigint typed array stores: the two
// 64-bit integers BigInt64Array and BigUint64Array hold.
type bigArrayElem interface {
	~int64 | ~uint64
}

// newBigIntArray builds a zeroed bigint typed array of the given length with the
// store and load rules its element kind uses, the shared body of the per-kind New
// constructors. It allocates a fresh ArrayBuffer of eight bytes per element and
// views the whole of it, so the array owns its storage but reaches it through the
// shared view path. The length truncates toward zero and clamps a negative or
// not-a-number length to zero, the covered subset the numeric family documents.
func newBigIntArray[T bigArrayElem](length float64, store func(*big.Int) T, load func(T) *big.Int) *BigIntArray[T] {
	n := typedLen(length)
	buf := NewArrayBuffer(float64(n * 8))
	return newBigIntArrayView(buf, 0, n, store, load)
}

// newBigIntArrayView builds a bigint typed array that views an existing buffer from
// a byte offset for a count of elements, the one place the unsafe aliasing forms.
// The data slice aliases the buffer's bytes, so a write through this view shows
// through every other view of the same buffer.
func newBigIntArrayView[T bigArrayElem](buf *ArrayBuffer, byteOffset, length int, store func(*big.Int) T, load func(T) *big.Int) *BigIntArray[T] {
	return &BigIntArray[T]{
		buffer:     buf,
		byteOffset: byteOffset,
		length:     length,
		store:      store,
		load:       load,
	}
}

// liveLen is the view's element count as of this access, clamped against the
// buffer's current byte length so a view over a detached buffer, or once resizable
// buffers land a shrunk one, reports zero, the bigint sibling of the numeric
// family's liveLen. A bigint element is eight bytes wide.
func (a *BigIntArray[T]) liveLen() int {
	if avail := len(a.buffer.data) - a.byteOffset; avail < a.length*8 {
		return 0
	}
	return a.length
}

// live forms the element slice this view reads right now, an unsafe alias over the
// buffer's current bytes for liveLen elements, recomputed on each access so a
// detach or a resize shows through immediately.
func (a *BigIntArray[T]) live() []T {
	return bigViewSlice[T](a.buffer, a.byteOffset, a.liveLen())
}

// bigIntArrayOf builds a bigint typed array from a list of bigints with the store
// rule its element kind uses, the shared body of the per-kind Of constructors. It
// allocates a fresh buffer of the right size and truncates each element into it
// exactly as an assignment into an element would, so the array owns its storage.
func bigIntArrayOf[T bigArrayElem](store func(*big.Int) T, load func(T) *big.Int, elems ...*big.Int) *BigIntArray[T] {
	a := newBigIntArray(float64(len(elems)), store, load)
	d := a.live()
	for i, e := range elems {
		d[i] = store(e)
	}
	return a
}

// bigIntArrayView builds a bigint typed array that views an existing ArrayBuffer,
// the shared body of the per-kind View constructors and the lowering of new
// BigInt64Array(buffer, byteOffset, length). The byte offset defaults to zero and
// the length, when omitted, runs from the offset to the end of the buffer in whole
// eight-byte elements. The view aliases the buffer's bytes, so it observes writes
// made through the buffer or through any other view of it. An offset or length past
// the buffer clamps to what it holds, the covered subset the RangeError is a later
// slice of.
func bigIntArrayView[T bigArrayElem](buf *ArrayBuffer, store func(*big.Int) T, load func(T) *big.Int, byteOffset float64, length ...float64) *BigIntArray[T] {
	const elem = 8
	off := typedLen(byteOffset)
	if off > len(buf.data) {
		off = len(buf.data)
	}
	var n int
	if len(length) > 0 {
		n = typedLen(length[0])
	} else {
		n = (len(buf.data) - off) / elem
	}
	if max := (len(buf.data) - off) / elem; n > max {
		n = max
	}
	return newBigIntArrayView(buf, off, n, store, load)
}

// bigViewSlice forms the element slice a bigint typed array reads, an unsafe alias
// over the buffer's bytes starting at the byte offset for length elements, the
// aliasing step that lets two views over one buffer observe each other's writes. A
// zero length needs no storage and returns nil, which also avoids indexing a nil or
// too-short buffer. The buffer keeps its bytes eight-byte aligned and a bigint
// element is eight bytes, so the first element lands on a naturally aligned address.
func bigViewSlice[T bigArrayElem](buf *ArrayBuffer, byteOffset, length int) []T {
	if length == 0 {
		return nil
	}
	return unsafe.Slice((*T)(unsafe.Pointer(&buf.data[byteOffset])), length)
}

// Len is the array's length in elements, a Number to match the type the checker
// gives .length and to compose with the numeric path with no conversion.
func (a *BigIntArray[T]) Len() float64 { return float64(a.liveLen()) }

// Buffer is the ArrayBuffer the view aliases, the .buffer getter, the same backing
// store every other view of the buffer holds.
func (a *BigIntArray[T]) Buffer() *ArrayBuffer { return a.buffer }

// ByteOffset is the byte the view starts at within its buffer, the .byteOffset
// getter, a Number to match the property's type.
func (a *BigIntArray[T]) ByteOffset() float64 { return float64(a.byteOffset) }

// ByteLength is the view's span in bytes, the .byteLength getter: the element count
// times eight, the run of buffer bytes the view aliases.
func (a *BigIntArray[T]) ByteLength() float64 { return float64(a.liveLen() * 8) }

// BytesPerElement is the element width in bytes, the instance BYTES_PER_ELEMENT
// property, eight for a bigint element. It is a method so a read keeps the receiver
// referenced rather than folding to a literal that would orphan the binding.
func (a *BigIntArray[T]) BytesPerElement() float64 { return 8 }

// At reads the element a JavaScript index expression a[i] selects, widened to the
// *big.Int a bigint read hands out. Only a canonical integer index inside the array
// names an element; an out-of-range or non-canonical index reads as 0n here rather
// than the undefined the spec gives, the covered subset for the bigint read path,
// since At's result is a *big.Int. A read that flows into a dynamic slot takes
// GetIndex instead, which does answer undefined for those indices.
func (a *BigIntArray[T]) At(i float64) *big.Int {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		return a.load(d[idx])
	}
	return new(big.Int)
}

// SetAt writes the element a JavaScript assignment a[i] = v stores, truncating the
// bigint to the element's 64-bit width the way asIntN(64) and asUintN(64) do, so a
// value outside the element's range wraps exactly as JavaScript does. Only a
// canonical integer index inside the array names an element; a write to an
// out-of-range or non-canonical index is dropped, the no-op the spec requires.
func (a *BigIntArray[T]) SetAt(i float64, v *big.Int) {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		d[idx] = a.store(v)
	}
}

// GetIndex reads the element a JavaScript index selects as a boxed Value, the form
// a bigint read takes when it flows into a dynamic slot. It answers the element as a
// boxed bigint for a canonical in-range index and the undefined singleton for an
// out-of-range or non-canonical one, so a[100] and a[1.5] read as undefined the way
// the spec requires, which the *big.Int At cannot express.
func (a *BigIntArray[T]) GetIndex(i float64) Value {
	d := a.live()
	if idx, ok := typedElemIndex(i, len(d)); ok {
		b := &BigInt{}
		b.i.Set(a.load(d[idx]))
		return BigIntValue(b)
	}
	return Undefined
}

// The per-kind constructors wire the element type and its store and load rules.
// Each is a one-liner over the shared bodies so generated code names a plain
// value.NewBigInt64Array(n) or value.BigInt64ArrayOf(1n, 2n) rather than spell the
// coercion at the call site.

func NewBigInt64Array(length float64) *BigIntArray[int64] {
	return newBigIntArray(length, toBigInt64, fromBigInt64)
}
func BigInt64ArrayOf(elems ...*big.Int) *BigIntArray[int64] {
	return bigIntArrayOf(toBigInt64, fromBigInt64, elems...)
}
func BigInt64ArrayView(buf *ArrayBuffer, byteOffset float64, length ...float64) *BigIntArray[int64] {
	return bigIntArrayView(buf, toBigInt64, fromBigInt64, byteOffset, length...)
}

func NewBigUint64Array(length float64) *BigIntArray[uint64] {
	return newBigIntArray(length, toBigUint64, fromBigUint64)
}
func BigUint64ArrayOf(elems ...*big.Int) *BigIntArray[uint64] {
	return bigIntArrayOf(toBigUint64, fromBigUint64, elems...)
}
func BigUint64ArrayView(buf *ArrayBuffer, byteOffset float64, length ...float64) *BigIntArray[uint64] {
	return bigIntArrayView(buf, toBigUint64, fromBigUint64, byteOffset, length...)
}

// toBigInt64 is the BigInt64Array store rule: wrap the bigint to a signed 64-bit
// two's-complement integer, so 2^63n stores -2^63 and -1n stores -1, the same wrap
// BigInt.asIntN(64) applies.
func toBigInt64(b *big.Int) int64 { return BigIntAsIntN(64, b).Int64() }

// fromBigInt64 widens a stored signed element back to the bigint a read hands out.
func fromBigInt64(v int64) *big.Int { return big.NewInt(v) }

// toBigUint64 is the BigUint64Array store rule: wrap the bigint to an unsigned
// 64-bit integer, so -1n stores 2^64-1 and 2^64n stores 0, the same wrap
// BigInt.asUintN(64) applies.
func toBigUint64(b *big.Int) uint64 { return BigIntAsUintN(64, b).Uint64() }

// fromBigUint64 widens a stored unsigned element back to the bigint a read hands
// out, keeping the full 64-bit magnitude a signed int64 could not.
func fromBigUint64(v uint64) *big.Int { return new(big.Int).SetUint64(v) }
