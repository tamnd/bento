package value

import (
	"math"
	"testing"
)

// wantRangeError runs fn and fails unless it throws a RangeError.
func wantRangeError(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		rec := recover()
		if rec == nil {
			t.Fatalf("%s did not throw", what)
		}
		if e := Caught(rec); !e.IsA("RangeError") {
			t.Fatalf("%s threw %v, want a RangeError", what, e)
		}
	}()
	fn()
}

// TestArrayBufferConstructorToIndex proves the ArrayBuffer constructor runs its byte
// length through ToIndex: a negative length, a non-integer negative, a length past
// 2^53-1, and an infinity each throw a RangeError before allocation, matching Node.
func TestArrayBufferConstructorToIndex(t *testing.T) {
	for _, tc := range []struct {
		name string
		len  float64
	}{
		{"negative", -1},
		{"negative-fraction", -1.1},
		{"negative-infinity", math.Inf(-1)},
		{"two-to-the-53", math.Pow(2, 53)},
		{"positive-infinity", math.Inf(1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantRangeError(t, "new ArrayBuffer("+tc.name+")", func() { NewArrayBuffer(tc.len) })
		})
	}
}

// TestArrayBufferAllocationLimit proves a length ToIndex admits but no Data Block can
// back throws a RangeError from CreateByteDataBlock rather than panicking in makeslice:
// 7 PiB and 2^53-1 are both under ToIndex's ceiling yet over the allocation limit.
func TestArrayBufferAllocationLimit(t *testing.T) {
	for _, tc := range []struct {
		name string
		len  float64
	}{
		{"seven-pib", 7 * 1125899906842624},
		{"two-to-the-53-minus-one", math.Pow(2, 53) - 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantRangeError(t, "new ArrayBuffer("+tc.name+")", func() { NewArrayBuffer(tc.len) })
		})
	}
}

// TestResizableArrayBufferMaxByteLengthToIndex proves the maxByteLength option runs
// through ToIndex and then the allocation limit: a negative max and a max past 2^53-1
// throw from ToIndex, and a 7 PiB max throws from the allocation reservation even when
// the initial length is zero, matching V8's reserve-the-maximum behavior.
func TestResizableArrayBufferMaxByteLengthToIndex(t *testing.T) {
	for _, tc := range []struct {
		name string
		max  float64
	}{
		{"negative", -1},
		{"two-to-the-53", math.Pow(2, 53)},
		{"seven-pib", 7 * 1125899906842624},
	} {
		t.Run(tc.name, func(t *testing.T) {
			wantRangeError(t, "new ArrayBuffer(0, {maxByteLength: "+tc.name+"})", func() {
				NewResizableArrayBuffer(0, tc.max)
			})
		})
	}
}

// TestArrayBufferResizeToIndex proves resize runs its new length through ToIndex, so a
// negative length throws a RangeError before the reallocation.
func TestArrayBufferResizeToIndex(t *testing.T) {
	b := NewResizableArrayBuffer(8, 16)
	wantRangeError(t, "resize(-1)", func() { b.Resize(-1) })
}

// TestArrayBufferTransferToIndex proves both transfer methods run their new length
// through ToIndex, so a length of 2^53 throws a RangeError rather than driving an
// unbounded allocation.
func TestArrayBufferTransferToIndex(t *testing.T) {
	wantRangeError(t, "transfer(2^53)", func() {
		NewArrayBuffer(8).Transfer(math.Pow(2, 53))
	})
	wantRangeError(t, "transferToFixedLength(2^53)", func() {
		NewArrayBuffer(8).TransferToFixedLength(math.Pow(2, 53))
	})
}

// TestArrayBufferNormalSizesStillAllocate proves the guards leave ordinary buffers
// untouched: a plain and a resizable buffer of a realistic size still build, resize,
// and transfer.
func TestArrayBufferNormalSizesStillAllocate(t *testing.T) {
	if got := NewArrayBuffer(16).ByteLength(); got != 16 {
		t.Errorf("new ArrayBuffer(16) byteLength = %v, want 16", got)
	}
	b := NewResizableArrayBuffer(8, 32)
	b.Resize(24)
	if got := b.ByteLength(); got != 24 {
		t.Errorf("resize(24) byteLength = %v, want 24", got)
	}
	if got := NewArrayBuffer(8).Transfer(12).ByteLength(); got != 12 {
		t.Errorf("transfer(12) byteLength = %v, want 12", got)
	}
}
