package value

import "testing"

// TestFlatMap pins that flatMap maps each element to an array and concatenates
// the results one level, so mapping n to [n, -n] doubles the length.
func TestFlatMap(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := FlatMap(a, func(n float64) *Array[float64] {
		return NewArray(n, -n)
	})
	elemsEqual(t, got.Elems(), []float64{1, -1, 2, -2, 3, -3})
}

// TestFlatMapEmptyResults pins that a callback returning an empty array drops the
// element from the output, the way flatMap can shrink an array.
func TestFlatMapEmptyResults(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	got := FlatMap(a, func(n float64) *Array[float64] {
		if int(n)%2 == 0 {
			return NewArray(n)
		}
		return NewArray[float64]()
	})
	elemsEqual(t, got.Elems(), []float64{2, 4})
}

// TestFlatMapChangingType pins that the callback can map to a different element
// type, mapping a number to an array of its string forms.
func TestFlatMapChangingType(t *testing.T) {
	a := NewArray[float64](1, 2)
	got := FlatMap(a, func(n float64) *Array[BStr] {
		return NewArray(NumberToString(n), NumberToString(-n))
	})
	if got.Len() != 4 {
		t.Fatalf("FlatMap length = %v, want 4", got.Len())
	}
	if got.At(0).ToGoString() != "1" || got.At(1).ToGoString() != "-1" {
		t.Errorf("FlatMap changing type = %q, %q", got.At(0).ToGoString(), got.At(1).ToGoString())
	}
}
