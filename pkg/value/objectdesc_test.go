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

// TestBagReadsThroughGetter proves a read of an accessor property runs its getter.
// The getter is a boxed function value, which carries no this slot in its argument
// vector, so it reaches sibling state through the object it closes over, the way a
// lowered getter does, rather than through a receiver argument.
func TestBagReadsThroughGetter(t *testing.T) {
	o := NewObject()
	oo := o.object()
	getter := NewFunc(func(args []Value) Value {
		return o.Get(FromGoString("x")).mulTwo()
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

// TestBagWriteInvokesAccessorSetterWithValue pins that an assignment to an accessor
// property runs its setter with the assigned value as argument zero, not the
// receiver. A boxed setter reads its declared parameter from argument zero, so
// prepending the receiver would bind that parameter to the object and drop the
// value; this is the regression behind a lowered `o.x = v` setter reading NaN.
func TestBagWriteInvokesAccessorSetterWithValue(t *testing.T) {
	o := NewObject()
	oo := o.object()
	var captured Value
	setter := NewFunc(func(args []Value) Value {
		captured = Arg(args, 0)
		return Undefined
	})
	oo.keys = append(oo.keys, FromGoString("x"))
	oo.descs = append(oo.descs, accessorProperty(Undefined, setter, true, true))

	o.Set(FromGoString("x"), Number(42))
	if captured.scalar != Number(42).scalar {
		t.Fatalf("setter received %v, want the written value 42", captured)
	}
}

// mulTwo is a tiny test helper doubling a number value.
func (v Value) mulTwo() Value { return Number(v.AsNumber() * 2) }
