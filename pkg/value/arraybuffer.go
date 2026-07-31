package value

import (
	"math"
	"unsafe"
)

// maxAllocByteLength is the largest byte length bento will allocate for a buffer's
// backing store. ToIndex admits lengths up to 2^53-1, but a Data Block that large
// cannot be created, so AllocateArrayBuffer's CreateByteDataBlock step throws a
// RangeError above this limit (25 §6.2.9.1 and §25.1.3.1). The limit sits far above
// any buffer a program realistically builds and far below Go's own allocation
// ceiling, so an over-large request throws the spec's RangeError instead of driving
// a makeslice panic or an out-of-memory abort.
const maxAllocByteLength = 1 << 40 // 1 TiB

// toIndex converts a JavaScript Number to a Go byte count, the ToIndex operation
// (25 §7.1.22): it truncates toward zero and throws a RangeError when the result is
// negative or exceeds 2^53-1. NaN maps to 0, which is in range, and a positive
// infinity exceeds the ceiling and throws. It is the length rule ArrayBuffer's
// constructor, resize, and transfer share for every byte-length argument, the reason
// `new ArrayBuffer(-1)`, `new ArrayBuffer(2**53)`, and `new ArrayBuffer(Infinity)`
// all throw a RangeError the way Node does. The bound is checked on the float before
// the int conversion, so an infinity never reaches int(), whose result is undefined.
func toIndex(v float64) int {
	if v != v {
		return 0
	}
	i := math.Trunc(v)
	if i < 0 || i > maxSafeInteger {
		Throw(NewRangeError(FromGoString("Invalid array buffer length")))
	}
	return int(i)
}

// requireAllocatable is CreateByteDataBlock's allocation check: it throws a RangeError
// when n exceeds maxAllocByteLength and returns n otherwise. A resizable buffer reserves
// its maximum up front the way V8 does, so its maxByteLength runs through here even when
// the initial length is zero, matching the RangeError Node throws for a 7 PiB
// maxByteLength option.
func requireAllocatable(n int) int {
	if n > maxAllocByteLength {
		Throw(NewRangeError(FromGoString("Array buffer allocation failed")))
	}
	return n
}

// ArrayBuffer is bento's runtime representation of a JavaScript ArrayBuffer, the
// raw byte backing store a typed array or a DataView views (25 §6.2 and §25.1). It
// owns a flat run of bytes and nothing else: a typed array is a separate view that
// records this buffer, a byte offset, and an element length, so many views can
// share one buffer and observe each other's writes. Splitting the storage out of
// the view this way is what lets `new Int32Array(buf)` and `new Uint8Array(buf)`
// over the same buffer alias the same bytes, which is the model the test262 buffer
// tests exercise.
//
// The bytes are allocated eight-byte aligned (allocBytes) so a view of any element
// width up to eight bytes, placed at the byte offset the spec requires to be a
// multiple of that width, lands on a naturally aligned address. A view then reads
// and writes its elements straight through an aliasing slice over these bytes with
// no per-element packing, and the platform's little-endian layout is the byte order
// the buffer exposes, which is the order the tests assume.
// A resizable buffer carries the maximum byte length it may grow to and a flag that
// marks it resizable, the pair new ArrayBuffer(n, { maxByteLength }) sets (25 §25.1.3).
// A fixed-length buffer leaves both zero, so resizable reads false and the max-length
// getter falls back to the current length the way the spec's getter does. resize
// reallocates the backing run to the requested length, within the max, and every view
// recomputes its span over the new run on its next access, so a shrink turns an
// out-of-range view zero-length and a grow restores it.
type ArrayBuffer struct {
	data          []byte
	detached      bool
	resizable     bool
	maxByteLength int
	// boxed is this buffer's dynamic view, built on the first crossing into a value.Value
	// and kept so every later crossing hands back the same object. A buffer is the storage
	// its views alias, so two boxes of one buffer would compare unequal under ===
	// (buffervalue.go).
	boxed *Object
}

// NewArrayBuffer builds a zeroed buffer of the given byte length, the lowering of
// `new ArrayBuffer(n)`. The length runs through ToIndex, so a negative length, a
// non-integer past 2^53-1, or an infinity throws a RangeError before any allocation,
// and CreateByteDataBlock's allocation limit throws a RangeError for a length too
// large to back, matching the throws Node raises rather than clamping or panicking.
func NewArrayBuffer(byteLength float64) *ArrayBuffer {
	return &ArrayBuffer{data: allocBytes(requireAllocatable(toIndex(byteLength)))}
}

// NewResizableArrayBuffer builds a resizable buffer of the given byte length that may
// later grow to maxByteLength, the lowering of new ArrayBuffer(n, { maxByteLength }).
// Both arguments are Numbers truncated toward zero like ToIndex. A max below the
// initial length is a RangeError, the same throw the spec raises for an initial length
// past the maximum; the covered subset otherwise clamps a negative to zero. The
// backing run is sized to the initial length, not the maximum, and resize reallocates
// it, so an unused max costs no storage.
func NewResizableArrayBuffer(byteLength float64, maxByteLength float64) *ArrayBuffer {
	n := toIndex(byteLength)
	max := toIndex(maxByteLength)
	if n > max {
		Throw(NewRangeError(FromGoString("ArrayBuffer byte length exceeds its maxByteLength")))
	}
	requireAllocatable(max)
	return &ArrayBuffer{data: allocBytes(n), resizable: true, maxByteLength: max}
}

// Resize grows or shrinks the backing run to newLength, the lowering of
// ArrayBuffer.prototype.resize (25 §25.1.6). Resizing a detached buffer or one that is
// not resizable is a TypeError, and a length past the maximum is a RangeError, the
// throws the spec raises. The run is reallocated to the new length, the retained bytes
// copied, and any growth left zeroed, so a view over the buffer sees the new size and,
// where a shrink drops its range, reads zero-length until a later grow restores it.
func (b *ArrayBuffer) Resize(newLength float64) {
	if b.detached {
		Throw(NewTypeError(FromGoString("Cannot resize a detached ArrayBuffer")))
	}
	if !b.resizable {
		Throw(NewTypeError(FromGoString("Cannot resize a non-resizable ArrayBuffer")))
	}
	n := toIndex(newLength)
	if n > b.maxByteLength {
		Throw(NewRangeError(FromGoString("ArrayBuffer resize length exceeds its maxByteLength")))
	}
	next := allocBytes(n)
	copy(next, b.data)
	b.data = next
}

// Slice copies the bytes in [start, end) into a fresh fixed-length buffer, the lowering
// of ArrayBuffer.prototype.slice (25 §25.1.6). start and end are optional Numbers; a
// negative index counts from the end and an omitted end runs to the current byte length,
// the same relative-index rule Array.prototype.slice takes, and an end before the start
// yields an empty buffer rather than a negative length. The result owns its bytes and
// does not alias the receiver, so a later write through either shows only in that one,
// and it is never resizable even when the receiver is, which is what the spec's
// species-free allocation does. Slicing a detached buffer is a TypeError, since there
// are no bytes left to copy.
func (b *ArrayBuffer) Slice(bounds ...float64) *ArrayBuffer {
	if b.detached {
		Throw(NewTypeError(FromGoString("Cannot perform ArrayBuffer.prototype.slice on a detached ArrayBuffer")))
	}
	n := len(b.data)
	start := 0
	if len(bounds) > 0 {
		start = relativeIndex(bounds[0], n)
	}
	end := n
	if len(bounds) > 1 {
		end = relativeIndex(bounds[1], n)
	}
	if end < start {
		end = start
	}
	out := NewArrayBuffer(float64(end - start))
	copy(out.data, b.data[start:end])
	return out
}

// MaxByteLength is the largest byte length the buffer may hold, the .maxByteLength
// accessor. A resizable buffer reports the maximum it was built with; a fixed-length
// one reports its current length, the value the spec's getter returns when the buffer
// is not resizable. A detached buffer reports zero.
func (b *ArrayBuffer) MaxByteLength() float64 {
	if b.detached {
		return 0
	}
	if b.resizable {
		return float64(b.maxByteLength)
	}
	return float64(len(b.data))
}

// Resizable reports whether the buffer may be resized, the .resizable accessor, true
// only for a buffer built with a maxByteLength.
func (b *ArrayBuffer) Resizable() bool { return b.resizable }

// ByteLength is the buffer's size in bytes, a Number to match the type the checker
// gives the .byteLength property and to compose with the numeric path with no
// conversion at the use site.
func (b *ArrayBuffer) ByteLength() float64 { return float64(len(b.data)) }

// Bytes returns the buffer's backing slice, the storage every view over it shares.
// A view builds its aliasing element slice over this run, and the Go boundary hands
// these bytes to a Go function taking []byte. The slice header aliases the buffer's
// own storage, so a write through any view or through the returned slice shows
// through every other view of the buffer.
func (b *ArrayBuffer) Bytes() []byte { return b.data }

// Transfer moves the buffer's bytes to a fresh buffer of the given byte length and
// detaches the receiver, the lowering of ArrayBuffer.prototype.transfer (25 §25.1.6).
// The new buffer keeps the first min(old, new) bytes and zero-fills any growth, and
// the old buffer is detached so every view over it reads as zero-length from here on.
// The new length defaults to the receiver's current byte length when the call gives
// none. Transferring an already-detached buffer is a TypeError, the same throw the
// spec raises. The resizable distinction transferToFixedLength carries has no effect
// until the resizable buffer lands, so the two share this body today.
func (b *ArrayBuffer) Transfer(newLength ...float64) *ArrayBuffer {
	return b.transfer(newLength)
}

// TransferToFixedLength moves the bytes to a fresh fixed-length buffer and detaches
// the receiver, the lowering of ArrayBuffer.prototype.transferToFixedLength. It
// differs from Transfer only in that its result is never resizable; with the
// resizable buffer still a later slice every buffer is already fixed-length, so it
// shares Transfer's body and the distinction is a no-op until then.
func (b *ArrayBuffer) TransferToFixedLength(newLength ...float64) *ArrayBuffer {
	return b.transfer(newLength)
}

// transfer is the shared body of the two transfer methods: it allocates the new
// buffer, copies the retained bytes, and detaches the receiver.
func (b *ArrayBuffer) transfer(newLength []float64) *ArrayBuffer {
	if b.detached {
		Throw(NewTypeError(FromGoString("Cannot transfer a detached ArrayBuffer")))
	}
	n := len(b.data)
	if len(newLength) > 0 {
		n = requireAllocatable(toIndex(newLength[0]))
	}
	out := &ArrayBuffer{data: allocBytes(n)}
	copy(out.data, b.data)
	b.Detach()
	return out
}

// Detach empties the buffer and marks it detached, the state a transfer or an
// explicit detach leaves it in. The bytes are dropped so ByteLength reads zero and,
// once the view path consults the buffer's live state, every view over it reads as
// zero-length with its indexed access a no-op.
func (b *ArrayBuffer) Detach() {
	b.data = nil
	b.detached = true
}

// Detached reports whether the buffer has been detached, the ArrayBuffer.prototype
// .detached accessor and the state the $DETACHBUFFER harness hook leaves behind.
func (b *ArrayBuffer) Detached() bool { return b.detached }

// allocBytes returns a byte slice of length n whose first byte is eight-byte
// aligned, so a typed-array view of any element width up to eight bytes reads and
// writes through it on a naturally aligned address on every platform, not only the
// ones that tolerate unaligned access. It backs the bytes with a []uint64, which
// the runtime aligns to eight bytes, and returns a byte view over that run; the
// byte slice keeps the backing array alive, since its pointer references the same
// allocation. A zero length needs no storage and returns nil.
func allocBytes(n int) []byte {
	if n == 0 {
		return nil
	}
	words := (n + 7) / 8
	backing := make([]uint64, words)
	return unsafe.Slice((*byte)(unsafe.Pointer(&backing[0])), n)
}
