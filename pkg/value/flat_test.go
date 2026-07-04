package value

import "testing"

// TestFlat pins that flat concatenates the inner arrays into one flat array at
// depth one.
func TestFlat(t *testing.T) {
	a := NewArray(
		NewArray[float64](1, 2),
		NewArray[float64](3),
		NewArray[float64](4, 5, 6),
	)
	got := Flat(a)
	elemsEqual(t, got.Elems(), []float64{1, 2, 3, 4, 5, 6})
}

// TestFlatEmptyInners pins that empty inner arrays contribute nothing.
func TestFlatEmptyInners(t *testing.T) {
	a := NewArray(
		NewArray[float64](),
		NewArray[float64](1),
		NewArray[float64](),
	)
	got := Flat(a)
	elemsEqual(t, got.Elems(), []float64{1})
}

// TestFlatDoesNotAlias pins that the result is a fresh array, so pushing to it
// leaves the inner arrays untouched.
func TestFlatDoesNotAlias(t *testing.T) {
	inner := NewArray[float64](1, 2)
	a := NewArray(inner)
	got := Flat(a)
	got.Push(3)
	if inner.Len() != 2 {
		t.Errorf("Flat aliased an inner array, length = %v, want 2", inner.Len())
	}
}
