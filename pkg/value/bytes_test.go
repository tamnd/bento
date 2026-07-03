package value

import "testing"

// TestNewUint8ArrayZeroed proves construction from a length gives that many zero
// bytes, the meaning of `new Uint8Array(n)`.
func TestNewUint8ArrayZeroed(t *testing.T) {
	a := NewUint8Array(4)
	if a.Len() != 4 {
		t.Fatalf("Len = %v, want 4", a.Len())
	}
	for i := 0; i < 4; i++ {
		if got := a.At(float64(i)); got != 0 {
			t.Errorf("At(%d) = %v, want 0 for a fresh buffer", i, got)
		}
	}
}

// TestNewUint8ArrayClampsBadLength proves a negative or not-a-number length yields
// an empty buffer rather than panicking, the covered-subset behavior until the
// construction lowering adds the RangeError JavaScript raises for a bad length.
func TestNewUint8ArrayClampsBadLength(t *testing.T) {
	for _, n := range []float64{-1, -100} {
		if got := NewUint8Array(n).Len(); got != 0 {
			t.Errorf("NewUint8Array(%v).Len = %v, want 0", n, got)
		}
	}
	nan := NewUint8Array(nanValue())
	if nan.Len() != 0 {
		t.Errorf("NewUint8Array(NaN).Len = %v, want 0", nan.Len())
	}
}

// TestUint8ArrayOfCoercesElements proves construction from a number list stores
// each element under ToUint8, so an in-range value passes through and an
// out-of-range value wraps modulo 256.
func TestUint8ArrayOfCoercesElements(t *testing.T) {
	a := Uint8ArrayOf(1, 2, 255, 256, -1, 3.9)
	want := []float64{1, 2, 255, 0, 255, 3}
	if a.Len() != float64(len(want)) {
		t.Fatalf("Len = %v, want %d", a.Len(), len(want))
	}
	for i, w := range want {
		if got := a.At(float64(i)); got != w {
			t.Errorf("At(%d) = %v, want %v", i, got, w)
		}
	}
}

// TestUint8ArrayFromGoAdopts proves wrapping a Go slice reads its bytes back as a
// buffer of the same length, the Go-to-bento crossing of a []byte return.
func TestUint8ArrayFromGoAdopts(t *testing.T) {
	a := Uint8ArrayFromGo([]byte{10, 20, 30})
	if a.Len() != 3 {
		t.Fatalf("Len = %v, want 3", a.Len())
	}
	if a.At(0) != 10 || a.At(1) != 20 || a.At(2) != 30 {
		t.Errorf("bytes read back as %v %v %v, want 10 20 30", a.At(0), a.At(1), a.At(2))
	}
}

// TestUint8ArrayBytesIsBacking proves Bytes returns the live storage the bridge
// passes to Go, not a copy, so a write through the buffer is visible in the slice.
func TestUint8ArrayBytesIsBacking(t *testing.T) {
	a := Uint8ArrayOf(1, 2, 3)
	b := a.Bytes()
	if len(b) != 3 {
		t.Fatalf("len(Bytes) = %d, want 3", len(b))
	}
	a.SetAt(0, 99)
	if b[0] != 99 {
		t.Errorf("Bytes did not alias the buffer: b[0] = %d, want 99", b[0])
	}
}

// TestUint8ArraySetAtCoercesAndBounds proves a write coerces with ToUint8 and an
// out-of-range write is ignored rather than growing the buffer.
func TestUint8ArraySetAtCoercesAndBounds(t *testing.T) {
	a := NewUint8Array(2)
	a.SetAt(0, 300) // 300 mod 256 = 44
	a.SetAt(1, -2)  // -2 mod 256 = 254
	a.SetAt(5, 7)   // out of range, ignored
	if a.At(0) != 44 {
		t.Errorf("At(0) = %v, want 44", a.At(0))
	}
	if a.At(1) != 254 {
		t.Errorf("At(1) = %v, want 254", a.At(1))
	}
	if a.Len() != 2 {
		t.Errorf("Len = %v, want 2 after an out-of-range write", a.Len())
	}
}

// TestUint8ArrayAtOutOfRange proves an index outside the buffer reads as 0, the
// covered-subset result the typed Array.At also gives.
func TestUint8ArrayAtOutOfRange(t *testing.T) {
	a := Uint8ArrayOf(5, 6)
	if got := a.At(9); got != 0 {
		t.Errorf("At(9) = %v, want 0", got)
	}
	if got := a.At(-1); got != 0 {
		t.Errorf("At(-1) = %v, want 0", got)
	}
}

// nanValue returns a NaN without invoking math, so the test file needs no import
// beyond testing.
func nanValue() float64 {
	zero := 0.0
	return zero / zero
}
