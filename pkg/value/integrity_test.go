package value

import "testing"

// TestPreventExtensions proves a non-extensible object drops a new key while its
// existing properties stay writable, the state Object.preventExtensions leaves.
func TestPreventExtensions(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	o.PreventExtensions()

	o.Set(FromGoString("b"), Number(2))
	if got := o.Get(FromGoString("b")); got.kind != KindUndefined {
		t.Fatalf("a new key on a non-extensible object took: got %v, want undefined", got)
	}
	o.Set(FromGoString("a"), Number(5))
	if got := o.Get(FromGoString("a")); got.scalar != Number(5).scalar {
		t.Fatalf("an existing key on a non-extensible object did not update: got %v, want 5", got)
	}
}

// TestPreventExtensionsArrayGrowth proves a non-extensible array refuses a write past
// its current length while an in-bounds element stays writable.
func TestPreventExtensionsArrayGrowth(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Number(2)})
	arr.PreventExtensions()

	arr.SetKey(FromGoString("5"), Number(9))
	if got := arr.Get(FromGoString("5")); got.kind != KindUndefined {
		t.Fatalf("a grow write on a non-extensible array took: got %v, want undefined", got)
	}
	arr.SetKey(FromGoString("0"), Number(7))
	if got := arr.Get(FromGoString("0")); got.scalar != Number(7).scalar {
		t.Fatalf("an in-bounds write on a non-extensible array did not update: got %v, want 7", got)
	}
}
