package value

import "testing"

// TestWith pins that with replaces the element at the index and returns the
// edited array.
func TestWith(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.With(1, 99)
	elemsEqual(t, got.Elems(), []float64{1, 99, 3})
}

// TestWithLeavesReceiver pins that with writes to a copy, so the receiver keeps
// its original elements and the result is a distinct array.
func TestWithLeavesReceiver(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.With(0, 99)
	elemsEqual(t, a.Elems(), []float64{1, 2, 3})
	elemsEqual(t, got.Elems(), []float64{99, 2, 3})
	if a == got {
		t.Error("With returned the receiver, want a fresh array")
	}
}

// TestWithNegativeIndex pins that a negative index counts from the end.
func TestWithNegativeIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.With(-1, 99)
	elemsEqual(t, got.Elems(), []float64{1, 2, 99})
}

// TestWithTruncatesIndex pins that a fractional index truncates toward zero, so
// with(1.9) writes at index one.
func TestWithTruncatesIndex(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.With(1.9, 99)
	elemsEqual(t, got.Elems(), []float64{1, 99, 3})
}

// TestWithOutOfRangeThrows pins that an index outside the array throws a
// RangeError that reports the original argument, matching with.
func TestWithOutOfRangeThrows(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("With out of range did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("With threw %T, want *Error", r)
		}
		if e.Name().ToGoString() != "RangeError" {
			t.Errorf("With threw %q, want RangeError", e.Name().ToGoString())
		}
		if got := e.ErrorMessage(); got != "Invalid index : 5" {
			t.Errorf("With message = %q", got)
		}
	}()
	NewArray[float64](1, 2, 3).With(5, 99)
}
