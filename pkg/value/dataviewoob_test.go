package value

import "testing"

// wantTypeError runs fn and fails unless it throws a TypeError. The DataView
// byteLength and byteOffset getters must throw, not return zero, once the view is
// out of bounds, so the tests below assert on the thrown class.
func wantTypeError(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("%s: expected a TypeError, got no throw", what)
		}
		err, ok := r.(*Error)
		if !ok {
			t.Fatalf("%s: recovered %T, want *Error", what, r)
		}
		if !err.IsA("TypeError") {
			t.Fatalf("%s: threw %s, want TypeError", what, err.ErrorName())
		}
	}()
	fn()
}

// TestDataViewByteLengthDetachedThrows proves the byteLength getter throws a
// TypeError once the backing buffer is detached, matching the spec getter's
// IsViewOutOfBounds check rather than the old zero return.
func TestDataViewByteLengthDetachedThrows(t *testing.T) {
	ab := NewArrayBuffer(8)
	dv := NewDataView(ab, 0)
	if dv.ByteLength() != 8 {
		t.Fatalf("byteLength before detach = %v, want 8", dv.ByteLength())
	}
	ab.Detach()
	wantTypeError(t, "byteLength after detach", func() { dv.ByteLength() })
	wantTypeError(t, "byteOffset after detach", func() { dv.ByteOffset() })
}

// TestDataViewOffsetDetachedThrows proves the byteOffset getter throws for a
// view whose buffer is detached, even when the offset is non-zero.
func TestDataViewOffsetDetachedThrows(t *testing.T) {
	ab := NewArrayBuffer(8)
	dv := NewDataView(ab, 4)
	if dv.ByteOffset() != 4 {
		t.Fatalf("byteOffset before detach = %v, want 4", dv.ByteOffset())
	}
	ab.Detach()
	wantTypeError(t, "byteOffset after detach", func() { dv.ByteOffset() })
}

// TestDataViewByteLengthTracksResizeThenThrows mirrors the test262 resizable-auto
// case: a length-tracking view at offset 1 reports its live span across in-bounds
// resizes, then throws once a shrink drops its offset past the buffer's end.
func TestDataViewByteLengthTracksResizeThenThrows(t *testing.T) {
	ab := NewResizableArrayBuffer(4, 5)
	dv := NewDataView(ab, 1)

	ab.Resize(5)
	if dv.ByteLength() != 4 {
		t.Fatalf("byteLength at size 5 = %v, want 4", dv.ByteLength())
	}
	ab.Resize(3)
	if dv.ByteLength() != 2 {
		t.Fatalf("byteLength at size 3 = %v, want 2", dv.ByteLength())
	}
	ab.Resize(1)
	if dv.ByteLength() != 0 {
		t.Fatalf("byteLength at size 1 = %v, want 0", dv.ByteLength())
	}
	if dv.ByteOffset() != 1 {
		t.Fatalf("byteOffset at size 1 = %v, want 1", dv.ByteOffset())
	}
	ab.Resize(0)
	wantTypeError(t, "byteLength when offset past end", func() { dv.ByteLength() })
	wantTypeError(t, "byteOffset when offset past end", func() { dv.ByteOffset() })
}
