package value

import "testing"

// TestToReversed pins that toReversed returns the elements in reverse order.
func TestToReversed(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.ToReversed()
	elemsEqual(t, got.Elems(), []float64{4, 3, 2, 1})
}

// TestToReversedLeavesReceiver pins that toReversed copies rather than reordering
// in place, so the receiver still reads in its original order and the result is a
// distinct array.
func TestToReversedLeavesReceiver(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.ToReversed()
	elemsEqual(t, a.Elems(), []float64{1, 2, 3})
	elemsEqual(t, got.Elems(), []float64{3, 2, 1})
	if a == got {
		t.Error("ToReversed returned the receiver, want a fresh array")
	}
}

// TestToReversedEmpty pins that reversing an empty array yields a fresh empty
// array rather than sharing storage with the receiver.
func TestToReversedEmpty(t *testing.T) {
	a := NewArray[float64]()
	got := a.ToReversed()
	if got.Len() != 0 {
		t.Errorf("ToReversed of empty = %v, want empty", got.Elems())
	}
	if a == got {
		t.Error("ToReversed of empty returned the receiver, want a fresh array")
	}
}
