package value

import "testing"

// TestReflectGetHas pins that Reflect.get and Reflect.has climb the prototype chain
// and that a symbol key is probed by identity rather than by string coercion.
func TestReflectGetHas(t *testing.T) {
	proto := NewObject()
	proto.Set(FromGoString("inherited"), Number(42))
	child := ObjectCreate(proto)
	child.Set(FromGoString("own"), Number(1))

	if got := ReflectGet(child, StringValue(FromGoString("inherited"))); got.AsNumber() != 42 {
		t.Errorf("Reflect.get did not read the inherited property, got %v", got)
	}
	if !ReflectHas(child, StringValue(FromGoString("inherited"))) {
		t.Error("Reflect.has did not find the inherited property")
	}
	if ReflectHas(child, StringValue(FromGoString("missing"))) {
		t.Error("Reflect.has reported a missing property present")
	}

	sym := NewSymbol(FromGoString("tag"))
	child.SetElem(sym, Number(7))
	if !ReflectHas(child, sym) {
		t.Error("Reflect.has did not find the own symbol key")
	}
	if got := ReflectGet(child, sym); got.AsNumber() != 7 {
		t.Errorf("Reflect.get did not read the symbol-keyed property, got %v", got)
	}
}

// TestReflectSetRefusals pins the false-returning refusals: a non-writable data
// property and a new key on a non-extensible target both refuse the write and leave
// the target unchanged.
func TestReflectSetRefusals(t *testing.T) {
	locked := NewObject()
	locked.DefineProperty(StringValue(FromGoString("x")), dataDesc(Number(1), false))
	if ReflectSet(locked, StringValue(FromGoString("x")), Number(2)) {
		t.Error("Reflect.set on a non-writable data property reported success")
	}
	if got := ReflectGet(locked, StringValue(FromGoString("x"))); got.AsNumber() != 1 {
		t.Errorf("a refused write changed the value, got %v", got)
	}

	sealed := NewObject()
	sealed.PreventExtensions()
	if ReflectSet(sealed, StringValue(FromGoString("y")), Number(3)) {
		t.Error("Reflect.set of a new key on a non-extensible target reported success")
	}
	if ReflectHas(sealed, StringValue(FromGoString("y"))) {
		t.Error("a refused new-key write still created the property")
	}
}

// TestReflectSetInheritedAccessor pins that Reflect.set runs an inherited setter
// with the target as its receiver, the ordinary [[Set]] behavior a plain own-only
// write would miss, and reports success.
func TestReflectSetInheritedAccessor(t *testing.T) {
	var captured Value
	setter := NewFunc(func(args []Value) Value {
		captured = Arg(args, 1)
		return Undefined
	})
	proto := NewObject()
	proto.DefineProperty(StringValue(FromGoString("acc")), accessorDesc(Undefined, setter))
	child := ObjectCreate(proto)

	if !ReflectSet(child, StringValue(FromGoString("acc")), Number(99)) {
		t.Error("Reflect.set through an inherited setter reported failure")
	}
	if captured.AsNumber() != 99 {
		t.Errorf("the inherited setter did not receive the written value, got %v", captured)
	}
	// The write ran the setter, so it must not have created an own data property.
	if child.HasOwnElem(StringValue(FromGoString("acc"))) {
		t.Error("Reflect.set through an inherited setter created a shadowing own property")
	}
}

// TestReflectDeleteProperty pins that a configurable property removes and reports
// true, an absent property reports true, and a non-configurable property survives
// and reports false, the boolean the delete operator gives for each.
func TestReflectDeleteProperty(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	if !ReflectDeleteProperty(o, StringValue(FromGoString("a"))) {
		t.Error("Reflect.deleteProperty on a configurable property reported failure")
	}
	if ReflectHas(o, StringValue(FromGoString("a"))) {
		t.Error("the deleted property is still present")
	}
	if !ReflectDeleteProperty(o, StringValue(FromGoString("missing"))) {
		t.Error("Reflect.deleteProperty on an absent property reported failure")
	}

	locked := NewObject()
	locked.DefineProperty(StringValue(FromGoString("x")), nonConfigDesc(Number(1)))
	if ReflectDeleteProperty(locked, StringValue(FromGoString("x"))) {
		t.Error("Reflect.deleteProperty on a non-configurable property reported success")
	}
	if !ReflectHas(locked, StringValue(FromGoString("x"))) {
		t.Error("a non-configurable property was removed by a refused delete")
	}
}

// TestReflectOwnKeys pins the [[OwnPropertyKeys]] order: integer-index keys first in
// ascending numeric order, then the remaining string keys in insertion order, then
// the symbol keys in insertion order, with a non-enumerable key included.
func TestReflectOwnKeys(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("b"), Number(1))
	o.Set(FromGoString("2"), Number(2))
	o.Set(FromGoString("a"), Number(3))
	o.Set(FromGoString("1"), Number(4))
	o.DefineProperty(StringValue(FromGoString("hidden")), nonEnumDesc(Number(5)))
	sym := NewSymbol(FromGoString("s"))
	o.SetElem(sym, Number(6))

	keys := ReflectOwnKeys(o).Elems()
	wantStr := []string{"1", "2", "b", "a", "hidden"}
	if len(keys) != len(wantStr)+1 {
		t.Fatalf("Reflect.ownKeys returned %d keys, want %d", len(keys), len(wantStr)+1)
	}
	for i, want := range wantStr {
		if keys[i].kind != KindString || keys[i].str().ToGoString() != want {
			t.Errorf("key %d = %v, want %q", i, keys[i], want)
		}
	}
	last := keys[len(keys)-1]
	if last.kind != KindSymbol || last.symbol() != sym.symbol() {
		t.Errorf("last key = %v, want the own symbol", last)
	}
}

// TestReflectOwnKeysArray pins that an array reports its element indices in ascending
// order, then its length as a string key, then any extra string key.
func TestReflectOwnKeysArray(t *testing.T) {
	arr := NewArrayValue([]Value{Number(10), Number(20), Number(30)})
	arr.Set(FromGoString("extra"), StringValue(FromGoString("e")))

	keys := ReflectOwnKeys(arr).Elems()
	want := []string{"0", "1", "2", "length", "extra"}
	if len(keys) != len(want) {
		t.Fatalf("Reflect.ownKeys on an array returned %d keys, want %d", len(keys), len(want))
	}
	for i, w := range want {
		if keys[i].kind != KindString || keys[i].str().ToGoString() != w {
			t.Errorf("array key %d = %v, want %q", i, keys[i], w)
		}
	}
}

// nonConfigDesc builds a non-configurable data descriptor object, the shape the
// deleteProperty refusal test defines onto an object.
func nonConfigDesc(value Value) Value {
	d := NewObject()
	d.Set(FromGoString("value"), value)
	d.Set(FromGoString("writable"), Bool(true))
	d.Set(FromGoString("enumerable"), Bool(true))
	d.Set(FromGoString("configurable"), Bool(false))
	return d
}

// nonEnumDesc builds a non-enumerable data descriptor object, the shape the ownKeys
// test uses to prove a non-enumerable key is still listed.
func nonEnumDesc(value Value) Value {
	d := NewObject()
	d.Set(FromGoString("value"), value)
	d.Set(FromGoString("writable"), Bool(true))
	d.Set(FromGoString("enumerable"), Bool(false))
	d.Set(FromGoString("configurable"), Bool(true))
	return d
}

// dataDesc builds a data descriptor object with an explicit writable flag, the
// descriptor shape a Reflect.set refusal test hands to DefineProperty.
func dataDesc(value Value, writable bool) Value {
	d := NewObject()
	d.Set(FromGoString("value"), value)
	d.Set(FromGoString("writable"), Bool(writable))
	d.Set(FromGoString("enumerable"), Bool(true))
	d.Set(FromGoString("configurable"), Bool(true))
	return d
}

// accessorDesc builds an accessor descriptor object with the given getter and
// setter, the descriptor shape the inherited-setter test defines onto a prototype.
func accessorDesc(get, set Value) Value {
	d := NewObject()
	d.Set(FromGoString("get"), get)
	d.Set(FromGoString("set"), set)
	d.Set(FromGoString("enumerable"), Bool(true))
	d.Set(FromGoString("configurable"), Bool(true))
	return d
}
