package value

import "testing"

// TestSpliceRemoveOnly pins that splice with a delete count and no items removes
// that many elements, returns them, and shrinks the receiver.
func TestSpliceRemoveOnly(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	removed := a.Splice(1, 2)
	elemsEqual(t, removed.Elems(), []float64{2, 3})
	elemsEqual(t, a.Elems(), []float64{1, 4, 5})
}

// TestSpliceInsert pins that splice inserts items in place of the removed range,
// growing the receiver when more are inserted than removed.
func TestSpliceInsert(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	removed := a.Splice(1, 1, 9, 8, 7)
	elemsEqual(t, removed.Elems(), []float64{2})
	elemsEqual(t, a.Elems(), []float64{1, 9, 8, 7, 3})
}

// TestSpliceInsertNoRemove pins that a zero delete count inserts without removing
// anything, returning an empty removed array.
func TestSpliceInsertNoRemove(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	removed := a.Splice(1, 0, 42)
	if removed.Len() != 0 {
		t.Errorf("removed length = %v, want 0", removed.Len())
	}
	elemsEqual(t, a.Elems(), []float64{1, 42, 2, 3})
}

// TestSpliceNegativeStart pins that a negative start counts from the end.
func TestSpliceNegativeStart(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	removed := a.Splice(-2, 1)
	elemsEqual(t, removed.Elems(), []float64{4})
	elemsEqual(t, a.Elems(), []float64{1, 2, 3, 5})
}

// TestSpliceCountClamped pins that a delete count past the end removes only to
// the end rather than running off it.
func TestSpliceCountClamped(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	removed := a.Splice(1, 100)
	elemsEqual(t, removed.Elems(), []float64{2, 3})
	elemsEqual(t, a.Elems(), []float64{1})
}

// TestSpliceToEnd pins the one-argument form: everything from start to the end is
// removed and returned.
func TestSpliceToEnd(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	removed := a.SpliceToEnd(2)
	elemsEqual(t, removed.Elems(), []float64{3, 4, 5})
	elemsEqual(t, a.Elems(), []float64{1, 2})
}

// TestSpliceResultDoesNotAlias pins that pushing to the removed array leaves the
// receiver untouched, so the two share no backing storage.
func TestSpliceResultDoesNotAlias(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	removed := a.Splice(1, 2)
	removed.Push(99)
	elemsEqual(t, a.Elems(), []float64{1, 4})
}
