package value

import "unsafe"

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
type ArrayBuffer struct {
	data []byte
}

// NewArrayBuffer builds a zeroed buffer of the given byte length, the lowering of
// `new ArrayBuffer(n)`. The length is a Number, so it arrives as a float64 and is
// truncated toward zero the way ToIndex does; a negative or not-a-number length
// clamps to zero here rather than throwing, the same covered-subset rule the byte
// buffer's constructor takes, with the RangeError a later slice.
func NewArrayBuffer(byteLength float64) *ArrayBuffer {
	return &ArrayBuffer{data: allocBytes(typedLen(byteLength))}
}

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
