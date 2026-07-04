package value

import "testing"

// TestToSorted pins that toSorted returns the elements in comparator order.
func TestToSorted(t *testing.T) {
	a := NewArray[float64](3, 1, 2)
	got := a.ToSorted(func(x, y float64) float64 { return x - y })
	elemsEqual(t, got.Elems(), []float64{1, 2, 3})
}

// TestToSortedLeavesReceiver pins that toSorted orders a copy, so the receiver
// keeps its original order and the result is a distinct array.
func TestToSortedLeavesReceiver(t *testing.T) {
	a := NewArray[float64](3, 1, 2)
	got := a.ToSorted(func(x, y float64) float64 { return x - y })
	elemsEqual(t, a.Elems(), []float64{3, 1, 2})
	elemsEqual(t, got.Elems(), []float64{1, 2, 3})
	if a == got {
		t.Error("ToSorted returned the receiver, want a fresh array")
	}
}

// TestToSortedStable pins that toSorted keeps the relative order of elements the
// comparator calls equal, sorting pairs by their first field only.
func TestToSortedStable(t *testing.T) {
	type pair struct {
		key int
		ord int
	}
	a := NewArray(pair{1, 0}, pair{1, 1}, pair{0, 2}, pair{1, 3})
	got := a.ToSorted(func(x, y pair) float64 { return float64(x.key - y.key) })
	ords := make([]float64, len(got.Elems()))
	for i, p := range got.Elems() {
		ords[i] = float64(p.ord)
	}
	elemsEqual(t, ords, []float64{2, 0, 1, 3})
}
