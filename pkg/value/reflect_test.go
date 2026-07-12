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
