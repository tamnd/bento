package value

import "testing"

func elemsEqual(t *testing.T, got []float64, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("length = %d, want %d (%v vs %v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("elem[%d] = %v, want %v (%v vs %v)", i, got[i], want[i], got, want)
		}
	}
}

// TestCopyWithinTargetStart pins the two-bound form: copying from index 3 onto
// index 0 overwrites the front with the tail, leaving the length unchanged.
func TestCopyWithinTargetStart(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	a.CopyWithin(0, 3)
	elemsEqual(t, a.Elems(), []float64{4, 5, 3, 4, 5})
}

// TestCopyWithinThreeBounds pins the half-open source range with an explicit end.
func TestCopyWithinThreeBounds(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	a.CopyWithin(0, 3, 4)
	elemsEqual(t, a.Elems(), []float64{4, 2, 3, 4, 5})
}

// TestCopyWithinNegative pins that negative indices count from the end.
func TestCopyWithinNegative(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	a.CopyWithin(-2)
	elemsEqual(t, a.Elems(), []float64{1, 2, 3, 1, 2})
}

// TestCopyWithinOverlap pins that an overlapping copy reads the source range as
// if to a temporary before writing, the memmove behavior copyWithin requires.
func TestCopyWithinOverlap(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4, 5)
	a.CopyWithin(2, 0)
	elemsEqual(t, a.Elems(), []float64{1, 2, 1, 2, 3})
}

// TestCopyWithinReturnsReceiver pins that copyWithin mutates in place and returns
// the same array, not a copy.
func TestCopyWithinReturnsReceiver(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	if a.CopyWithin(0, 1) != a {
		t.Error("CopyWithin did not return the receiver")
	}
}
