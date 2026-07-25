package value

import "testing"

// A dynamic read of hasOwnProperty on a plain object with no own binding of that
// name resolves the inherited Object.prototype method and reports own membership:
// true for a present key, false for an absent one.
func TestObjectProtoHasOwnPropertyFallback(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	fn := o.Get(FromGoString("hasOwnProperty"))
	if fn.kind != KindFunc {
		t.Fatalf("hasOwnProperty did not resolve to a function: kind %v", fn.kind)
	}
	if got := fn.Call(StringValue(FromGoString("a"))); !ToBoolean(got) {
		t.Errorf("hasOwnProperty(a) = %v, want true", got)
	}
	if got := fn.Call(StringValue(FromGoString("b"))); ToBoolean(got) {
		t.Errorf("hasOwnProperty(b) = %v, want false", got)
	}
}

// An own property of the same name overrides the inherited fallback: the read
// returns the user's function, not the built-in, matching own-before-inherited order.
func TestObjectProtoOwnOverrideWins(t *testing.T) {
	o := NewObject()
	own := NewFunc(func(args []Value) Value { return StringValue(FromGoString("custom")) })
	o.Set(FromGoString("hasOwnProperty"), own)
	got := o.Get(FromGoString("hasOwnProperty")).Call(StringValue(FromGoString("x")))
	if ToString(got).ToGoString() != "custom" {
		t.Errorf("own hasOwnProperty was shadowed by the fallback: got %q", ToString(got).ToGoString())
	}
}

// isPrototypeOf climbs the target's prototype chain and reports whether the receiver
// appears on it; the relation is directional, so a child is not a prototype of its
// own prototype.
func TestObjectProtoIsPrototypeOf(t *testing.T) {
	proto := NewObject()
	child := NewObject()
	child.object().proto = proto.object()
	if got := proto.Get(FromGoString("isPrototypeOf")).Call(child); !ToBoolean(got) {
		t.Errorf("proto.isPrototypeOf(child) = %v, want true", got)
	}
	if got := child.Get(FromGoString("isPrototypeOf")).Call(proto); ToBoolean(got) {
		t.Errorf("child.isPrototypeOf(proto) = %v, want false", got)
	}
}

// propertyIsEnumerable reports the own enumerable attribute: a plain assigned
// property is enumerable, an inherited one is not, and an absent one is not.
func TestObjectProtoPropertyIsEnumerable(t *testing.T) {
	proto := NewObject()
	proto.Set(FromGoString("inherited"), Number(1))
	o := NewObject()
	o.object().proto = proto.object()
	o.Set(FromGoString("own"), Number(2))
	fn := o.Get(FromGoString("propertyIsEnumerable"))
	if got := fn.Call(StringValue(FromGoString("own"))); !ToBoolean(got) {
		t.Errorf("propertyIsEnumerable(own) = %v, want true", got)
	}
	if got := fn.Call(StringValue(FromGoString("inherited"))); ToBoolean(got) {
		t.Errorf("propertyIsEnumerable(inherited) = %v, want false", got)
	}
	if got := fn.Call(StringValue(FromGoString("absent"))); ToBoolean(got) {
		t.Errorf("propertyIsEnumerable(absent) = %v, want false", got)
	}
}
