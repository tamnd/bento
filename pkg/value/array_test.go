package value

import "testing"

// TestNewArrayLenElems pins the dense array header: length is the element count
// as a float64, and iteration reads the elements back in order and unchanged.
func TestNewArrayLenElems(t *testing.T) {
	a := NewArray[float64](10, 20, 30)
	if got := a.Len(); got != 3 {
		t.Fatalf("Len() = %v, want 3", got)
	}
	want := []float64{10, 20, 30}
	got := a.Elems()
	if len(got) != len(want) {
		t.Fatalf("Elems() len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Elems()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestNewArrayEmpty pins that an empty literal is a length-zero array, not a nil
// header, so .length reads 0 and iteration visits nothing.
func TestNewArrayEmpty(t *testing.T) {
	a := NewArray[float64]()
	if got := a.Len(); got != 0 {
		t.Fatalf("Len() = %v, want 0", got)
	}
	if got := len(a.Elems()); got != 0 {
		t.Fatalf("Elems() len = %d, want 0", got)
	}
}

// TestNewArrayOwnsStorage pins that NewArray copies its elements into its own
// backing store, so mutating the caller's slice after construction does not
// reach into the array.
func TestNewArrayOwnsStorage(t *testing.T) {
	src := []float64{1, 2, 3}
	a := NewArray(src...)
	src[0] = 99
	if got := a.Elems()[0]; got != 1 {
		t.Fatalf("array aliased its argument: Elems()[0] = %v, want 1", got)
	}
}

// TestNewArrayString pins the header at a non-numeric element type, the string[]
// case the lowerer emits as *Array[BStr].
func TestNewArrayString(t *testing.T) {
	a := NewArray(FromGoString("a"), FromGoString("bb"))
	if got := a.Len(); got != 2 {
		t.Fatalf("Len() = %v, want 2", got)
	}
	if got := a.Elems()[1].Length(); got != 2 {
		t.Fatalf("second element length = %v, want 2", got)
	}
}
