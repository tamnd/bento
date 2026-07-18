package value

import "testing"

// TestIndexRestTail pins that IndexRest gathers the source's tail from a start index
// into a fresh boxed array, the dense elements past an array pattern's fixed slots.
func TestIndexRestTail(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Number(2), Number(3), Number(4)})
	rest := arr.IndexRest(1)
	if got := rest.Get(FromGoString("length")); got.AsNumber() != 3 {
		t.Fatalf("rest.length = %v, want 3", got)
	}
	for i, want := range []float64{2, 3, 4} {
		if got := rest.GetIndex(float64(i)); got.AsNumber() != want {
			t.Fatalf("rest[%d] = %v, want %v", i, got, want)
		}
	}
}

// TestIndexRestFromZero pins that a start of zero copies the whole source, the rest of
// a pattern with no fixed slots ahead of it.
func TestIndexRestFromZero(t *testing.T) {
	arr := NewArrayValue([]Value{Number(7), Number(8)})
	rest := arr.IndexRest(0)
	if got := rest.Get(FromGoString("length")); got.AsNumber() != 2 {
		t.Fatalf("rest.length = %v, want 2", got)
	}
	if got := rest.GetIndex(0); got.AsNumber() != 7 {
		t.Fatalf("rest[0] = %v, want 7", got)
	}
}

// TestIndexRestPastEnd pins that a start at or past the length yields an empty array,
// the rest of a source shorter than the pattern's fixed slots.
func TestIndexRestPastEnd(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Number(2)})
	rest := arr.IndexRest(5)
	if got := rest.Get(FromGoString("length")); got.AsNumber() != 0 {
		t.Fatalf("rest.length = %v, want 0", got)
	}
}

// TestIndexRestArrayLike pins that IndexRest reads through the dynamic length and index
// protocol, so a plain object carrying a length and numeric keys yields its indexed
// tail the way an array-like source does.
func TestIndexRestArrayLike(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("0"), Number(10))
	o.Set(FromGoString("1"), Number(20))
	o.Set(FromGoString("2"), Number(30))
	o.Set(FromGoString("length"), Number(3))
	rest := o.IndexRest(1)
	if got := rest.Get(FromGoString("length")); got.AsNumber() != 2 {
		t.Fatalf("rest.length = %v, want 2", got)
	}
	if got := rest.GetIndex(0); got.AsNumber() != 20 {
		t.Fatalf("rest[0] = %v, want 20", got)
	}
}
