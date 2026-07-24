package value

import (
	"testing"
	"unsafe"
)

// TestArrayBufferByteLength proves the constructor allocates the requested number
// of bytes and reads the count back through byteLength.
func TestArrayBufferByteLength(t *testing.T) {
	if got := NewArrayBuffer(8).ByteLength(); got != 8 {
		t.Errorf("NewArrayBuffer(8).ByteLength = %v, want 8", got)
	}
	if got := NewArrayBuffer(0).ByteLength(); got != 0 {
		t.Errorf("NewArrayBuffer(0).ByteLength = %v, want 0", got)
	}
}

// TestArrayBufferBadLength proves the byte length runs through ToIndex: a negative
// length throws a RangeError the way Node does, while a not-a-number length maps to
// zero (ToIntegerOrInfinity(NaN) is 0, which is in range) and yields an empty buffer.
func TestArrayBufferBadLength(t *testing.T) {
	wantRangeError(t, "NewArrayBuffer(-4)", func() { NewArrayBuffer(-4) })
	if got := NewArrayBuffer(nanValue()).ByteLength(); got != 0 {
		t.Errorf("NewArrayBuffer(NaN).ByteLength = %v, want 0", got)
	}
}

// TestArrayBufferBytesAlias proves the backing slice is the buffer's own storage:
// a write through the returned slice shows through a fresh read of it, and its
// length matches byteLength.
func TestArrayBufferBytesAlias(t *testing.T) {
	b := NewArrayBuffer(4)
	if len(b.Bytes()) != 4 {
		t.Fatalf("Bytes len = %d, want 4", len(b.Bytes()))
	}
	b.Bytes()[2] = 0x7f
	if got := b.Bytes()[2]; got != 0x7f {
		t.Errorf("aliased byte = %#x, want 0x7f", got)
	}
}

// TestArrayBufferAligned proves the backing bytes start on an eight-byte boundary,
// so a wide-element view over the buffer reads on a naturally aligned address.
func TestArrayBufferAligned(t *testing.T) {
	b := NewArrayBuffer(16)
	addr := uintptr(unsafe.Pointer(&b.Bytes()[0]))
	if addr%8 != 0 {
		t.Errorf("buffer base address %#x is not eight-byte aligned", addr)
	}
}
