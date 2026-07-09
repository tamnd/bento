package value

import "testing"

// TestArraySetGrowsWithinCap proves a write past the end still grows the dense
// backing and fills the gap with the zero value, the sparse-hole behavior At
// reads back, as long as the gap stays within the cap.
func TestArraySetGrowsWithinCap(t *testing.T) {
	a := NewArray[float64]()
	a.Set(4, 7)
	if a.Len() != 5 {
		t.Fatalf("Set(4) length = %v, want 5", a.Len())
	}
	if got := a.At(4); got != 7 {
		t.Errorf("At(4) = %v, want 7", got)
	}
	if got := a.At(2); got != 0 {
		t.Errorf("At(2) hole = %v, want 0", got)
	}
}

// TestArraySetHugeIndexThrows proves a write whose gap outruns the cap throws a
// RangeError rather than growing the backing slice into an out-of-memory. The
// power-of-two index test (S15.4_A1.1_T10) writes near index 2^32, which would
// otherwise try to allocate billions of elements and take the machine down; the
// cap turns that into a bounded throw before any giant allocation.
func TestArraySetHugeIndexThrows(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("Set at a huge index did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("Set at a huge index threw %T, want *Error", r)
		}
		if e.Name().ToGoString() != "RangeError" {
			t.Errorf("Set at a huge index threw %q, want RangeError", e.Name().ToGoString())
		}
	}()
	a := NewArray[float64]()
	a.Set(1<<32-2, 1)
}
