package value

import "testing"

// TestSetInBounds proves a write inside the array overwrites in place, leaves the
// length alone, and returns the assigned value the way an assignment expression
// evaluates to its right side.
func TestSetInBounds(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	if got := a.Set(1, 99); got != 99 {
		t.Errorf("Set returned %v, want the assigned value 99", got)
	}
	if a.Len() != 3 {
		t.Errorf("Set inside the array changed the length to %v, want 3", a.Len())
	}
	if a.At(1) != 99 {
		t.Errorf("Set(1, 99) left At(1) = %v, want 99", a.At(1))
	}
}

// TestSetAtLengthAppends proves a write at the current length extends the array by
// one, the a[a.length] = v append idiom.
func TestSetAtLengthAppends(t *testing.T) {
	a := NewArray[float64](1, 2)
	a.Set(2, 3)
	if a.Len() != 3 {
		t.Fatalf("Set at the length gave length %v, want 3", a.Len())
	}
	if a.At(2) != 3 {
		t.Errorf("Set(2, 3) appended %v, want 3", a.At(2))
	}
}

// TestSetPastLengthFillsGap proves a write past the length grows the array and
// fills the gap with the zero value, the absent element At reads back out of range.
func TestSetPastLengthFillsGap(t *testing.T) {
	a := NewArray[float64](1)
	a.Set(3, 9)
	if a.Len() != 4 {
		t.Fatalf("Set past the length gave length %v, want 4", a.Len())
	}
	if a.At(1) != 0 || a.At(2) != 0 {
		t.Errorf("Set past the length left gap = [%v, %v], want zero fill", a.At(1), a.At(2))
	}
	if a.At(3) != 9 {
		t.Errorf("Set(3, 9) wrote %v at index 3, want 9", a.At(3))
	}
}

// TestSetTruncatesIndex proves a fractional index truncates toward zero the way At
// reads it, so Set(1.9, v) writes the same slot At(1.9) reads.
func TestSetTruncatesIndex(t *testing.T) {
	a := NewArray[float64](0, 0, 0)
	a.Set(1.9, 7)
	if a.At(1) != 7 {
		t.Errorf("Set(1.9, 7) wrote At(1) = %v, want 7", a.At(1))
	}
}

// TestSetNegativeIndexIsNoOp proves a negative index writes no element and leaves
// the length alone, since a negative index is a string-keyed property in
// JavaScript rather than an element. The assignment still yields the value.
func TestSetNegativeIndexIsNoOp(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	if got := a.Set(-1, 42); got != 42 {
		t.Errorf("Set(-1, 42) returned %v, want the value 42", got)
	}
	if a.Len() != 3 {
		t.Errorf("Set(-1, 42) changed the length to %v, want 3", a.Len())
	}
}

// TestSetIsVisibleThroughAlias proves Set mutates through the pointer, so a second
// reference to the same array sees the write.
func TestSetIsVisibleThroughAlias(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	b := a
	a.Set(0, 100)
	if b.At(0) != 100 {
		t.Errorf("Set through one reference left the alias At(0) = %v, want 100", b.At(0))
	}
}
