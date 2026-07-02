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

// TestPush pins that push appends, returns the new length, and grows the array
// the iteration reads back, including the variadic multi-argument form.
func TestPush(t *testing.T) {
	a := NewArray[float64](1)
	if got := a.Push(2); got != 2 {
		t.Fatalf("Push(2) = %v, want 2", got)
	}
	if got := a.Push(3, 4); got != 4 {
		t.Fatalf("Push(3, 4) = %v, want 4", got)
	}
	want := []float64{1, 2, 3, 4}
	got := a.Elems()
	if len(got) != len(want) {
		t.Fatalf("after pushes len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Elems()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestPushShared pins that push mutates through the pointer, so a second
// reference to the same array sees the appended element. This is the shared
// mutation a const binding does not prevent.
func TestPushShared(t *testing.T) {
	a := NewArray[float64](1)
	b := a
	a.Push(2)
	if got := b.Len(); got != 2 {
		t.Fatalf("shared reference Len() = %v, want 2", got)
	}
}

// TestMap pins that map applies the callback to each element in order and
// returns a fresh array, leaving the receiver unchanged.
func TestMap(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	b := a.Map(func(x float64) float64 { return x * 10 })
	want := []float64{10, 20, 30}
	got := b.Elems()
	if len(got) != len(want) {
		t.Fatalf("Map len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Map()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if a.Elems()[0] != 1 {
		t.Errorf("Map mutated the receiver: Elems()[0] = %v, want 1", a.Elems()[0])
	}
}

// TestFilter pins that filter keeps the elements the predicate accepts, in
// order, and returns a fresh array, leaving the receiver unchanged.
func TestFilter(t *testing.T) {
	a := NewArray[float64](1, 2, 3, 4)
	b := a.Filter(func(x float64) bool { return x > 2 })
	want := []float64{3, 4}
	got := b.Elems()
	if len(got) != len(want) {
		t.Fatalf("Filter len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("Filter()[%d] = %v, want %v", i, got[i], want[i])
		}
	}
	if a.Len() != 4 {
		t.Errorf("Filter mutated the receiver: Len() = %v, want 4", a.Len())
	}
}

// TestFilterNoneKept pins that a predicate that rejects everything yields a
// length-zero array, not a nil header.
func TestFilterNoneKept(t *testing.T) {
	a := NewArray[float64](1, 2, 3)
	b := a.Filter(func(x float64) bool { return x > 100 })
	if got := b.Len(); got != 0 {
		t.Fatalf("Filter len = %v, want 0", got)
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
