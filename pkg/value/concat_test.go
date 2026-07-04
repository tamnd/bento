package value

import "testing"

// TestConcat pins that concat appends the argument arrays to a copy of the
// receiver in order.
func TestConcat(t *testing.T) {
	a := NewArray[float64](1, 2)
	b := NewArray[float64](3, 4)
	c := NewArray[float64](5)
	got := a.Concat(b, c).Elems()
	want := []float64{1, 2, 3, 4, 5}
	if len(got) != len(want) {
		t.Fatalf("Concat length = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Concat[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestConcatNoArgs pins that concat with no arguments returns a copy of the
// receiver, matching JavaScript where a.concat() is a shallow clone.
func TestConcatNoArgs(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	got := a.Concat()
	if got == a {
		t.Error("Concat with no arguments returned the same array, want a copy")
	}
	if got.Len() != 3 {
		t.Errorf("Concat copy length = %v, want 3", got.Len())
	}
}

// TestConcatDoesNotMutate pins that concat leaves its sources unchanged and the
// result aliases none of them, so pushing to the result does not touch a source.
func TestConcatDoesNotMutate(t *testing.T) {
	a := NewArray[float64](1, 2)
	b := NewArray[float64](3, 4)
	out := a.Concat(b)
	out.Push(5)
	if a.Len() != 2 {
		t.Errorf("Concat mutated the receiver, length = %v, want 2", a.Len())
	}
	if b.Len() != 2 {
		t.Errorf("Concat mutated an argument, length = %v, want 2", b.Len())
	}
}
