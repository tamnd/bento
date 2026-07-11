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

// TestSeal proves a sealed object marks every own property non-configurable while
// leaving it writable: a delete fails and the property survives, an assignment still
// takes, and a new key is refused.
func TestSeal(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	o.Seal()

	if d, ok := o.object().getOwnDesc(FromGoString("a")); !ok || d.configurable {
		t.Fatal("seal did not clear the property's configurable flag")
	} else if !d.writable {
		t.Fatal("seal cleared the property's writable flag, which seal leaves alone")
	}
	if o.Delete(FromGoString("a")) {
		t.Fatal("delete of a sealed property reported success")
	}
	if got := o.Get(FromGoString("a")); got.scalar != Number(1).scalar {
		t.Fatalf("a sealed property did not survive delete: got %v, want 1", got)
	}
	o.Set(FromGoString("a"), Number(9))
	if got := o.Get(FromGoString("a")); got.scalar != Number(9).scalar {
		t.Fatalf("a sealed property is not writable: got %v, want 9", got)
	}
	o.Set(FromGoString("b"), Number(2))
	if got := o.Get(FromGoString("b")); got.kind != KindUndefined {
		t.Fatalf("a new key on a sealed object took: got %v, want undefined", got)
	}
}

// TestFreeze proves a frozen object marks every own data property non-writable and
// non-configurable: an assignment drops, the value survives, and a new key is
// refused.
func TestFreeze(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	o.Freeze()

	if d, ok := o.object().getOwnDesc(FromGoString("a")); !ok || d.configurable || d.writable {
		t.Fatal("freeze did not clear the property's writable and configurable flags")
	}
	o.Set(FromGoString("a"), Number(9))
	if got := o.Get(FromGoString("a")); got.scalar != Number(1).scalar {
		t.Fatalf("a frozen property took a write: got %v, want 1", got)
	}
	o.Set(FromGoString("b"), Number(2))
	if got := o.Get(FromGoString("b")); got.kind != KindUndefined {
		t.Fatalf("a new key on a frozen object took: got %v, want undefined", got)
	}
}

// TestFreezeArrayElements proves a frozen array drops an element write and keeps the
// original element.
func TestFreezeArrayElements(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Number(2)})
	arr.Freeze()

	arr.SetKey(FromGoString("0"), Number(9))
	if got := arr.Get(FromGoString("0")); got.scalar != Number(1).scalar {
		t.Fatalf("a frozen array element took a write: got %v, want 1", got)
	}
}

// TestIsExtensible proves the predicate reports a fresh object extensible and a
// prevented, sealed, or frozen object not, and a primitive not.
func TestIsExtensible(t *testing.T) {
	if !NewObject().IsExtensible() {
		t.Fatal("a fresh object reported not extensible")
	}
	if NewObject().PreventExtensions().IsExtensible() {
		t.Fatal("a prevented object reported extensible")
	}
	if NewObject().Seal().IsExtensible() {
		t.Fatal("a sealed object reported extensible")
	}
	if NewObject().Freeze().IsExtensible() {
		t.Fatal("a frozen object reported extensible")
	}
	if Number(1).IsExtensible() {
		t.Fatal("a number reported extensible")
	}
}
