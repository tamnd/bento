package value

import "testing"

// TestBagStoresDescriptors proves a plain write lands as a default data
// descriptor (writable, enumerable, configurable) and reads its value back
// through the descriptor.
func TestBagStoresDescriptors(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))

	d, ok := o.object().getOwnDesc(FromGoString("a"))
	if !ok {
		t.Fatal("property a not found in bag")
	}
	if d.isAccessor() {
		t.Fatal("plain write stored an accessor descriptor")
	}
	if !d.writable || !d.enumerable || !d.configurable {
		t.Fatalf("plain-write flags = w:%v e:%v c:%v, want all true", d.writable, d.enumerable, d.configurable)
	}
	if got := o.Get(FromGoString("a")); got.scalar != Number(1).scalar {
		t.Fatalf("read = %v, want 1", got)
	}

	// A repeated write keeps the slot and updates the value.
	o.Set(FromGoString("a"), Number(2))
	if got := o.Get(FromGoString("a")); got.scalar != Number(2).scalar {
		t.Fatalf("read after overwrite = %v, want 2", got)
	}
}

// TestBagReadsThroughGetter proves a read of an accessor property runs its getter
// with the object as the receiver, so a getter that leans on this sees the object.
func TestBagReadsThroughGetter(t *testing.T) {
	o := NewObject()
	oo := o.object()
	getter := NewFunc(func(args []Value) Value {
		// this is passed as the sole argument, so a getter can read a sibling prop.
		return Arg(args, 0).Get(FromGoString("x")).mulTwo()
	})
	oo.keys = append(oo.keys, FromGoString("x"), FromGoString("doubled"))
	oo.descs = append(oo.descs,
		defaultDataProperty(Number(21)),
		accessorProperty(getter, Undefined, true, true),
	)

	if got := o.Get(FromGoString("doubled")); got.scalar != Number(42).scalar {
		t.Fatalf("accessor read = %v, want 42", got)
	}
}

// mulTwo is a tiny test helper doubling a number value.
func (v Value) mulTwo() Value { return Number(v.AsNumber() * 2) }
