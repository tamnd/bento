package value

import "testing"

// TestToSpliced pins that toSpliced removes a run and inserts items in its place,
// returning the edited array.
func TestToSpliced(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	got := a.ToSpliced(1, 2, 20, 30, 40)
	elemsEqual(t, got.Elems(), []float64{1, 20, 30, 40, 4, 5})
}

// TestToSplicedLeavesReceiver pins that toSpliced edits a copy, so the receiver
// keeps its original elements and the result is a distinct array.
func TestToSplicedLeavesReceiver(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.ToSpliced(1, 2)
	elemsEqual(t, a.Elems(), []float64{1, 2, 3, 4})
	elemsEqual(t, got.Elems(), []float64{1, 4})
	if a == got {
		t.Error("ToSpliced returned the receiver, want a fresh array")
	}
}

// TestToSplicedNegativeStart pins that a negative start counts from the end, the
// same way splice reads it.
func TestToSplicedNegativeStart(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := a.ToSpliced(-2, 1, 99)
	elemsEqual(t, got.Elems(), []float64{1, 2, 99, 4})
}

// TestToSplicedToEnd pins that the one-argument form removes everything from
// start to the end into a fresh array and leaves the receiver alone.
func TestToSplicedToEnd(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	got := a.ToSplicedToEnd(2)
	elemsEqual(t, got.Elems(), []float64{1, 2})
	elemsEqual(t, a.Elems(), []float64{1, 2, 3, 4, 5})
}
