package value

import "testing"

// TestObjectCreateSetsProto proves Object.create(proto) returns a new object whose
// [[Prototype]] slot points at the given prototype object, the link a later read
// climbs.
func TestObjectCreateSetsProto(t *testing.T) {
	proto := NewObject()
	proto.Set(FromGoString("shared"), Number(9))

	child := ObjectCreate(proto)
	if child.object().proto != proto.object() {
		t.Fatal("Object.create did not set the prototype slot to the given object")
	}
	if child.object() == proto.object() {
		t.Fatal("Object.create returned the prototype rather than a fresh object")
	}
}

// TestPrototypeChainRead proves a property read climbs the prototype chain: a key
// only the prototype carries reads through the child, an own key of the same name
// shadows the inherited one, and a key on neither reads undefined.
func TestPrototypeChainRead(t *testing.T) {
	proto := NewObject()
	proto.Set(FromGoString("shared"), Number(1))
	proto.Set(FromGoString("shadowed"), Number(2))

	child := ObjectCreate(proto)
	child.Set(FromGoString("shadowed"), Number(3))
	child.Set(FromGoString("own"), Number(4))

	if got := child.Get(FromGoString("shared")); got.scalar != Number(1).scalar {
		t.Fatalf("inherited read = %v, want 1", got)
	}
	if got := child.Get(FromGoString("shadowed")); got.scalar != Number(3).scalar {
		t.Fatalf("own key did not shadow the inherited one: got %v, want 3", got)
	}
	if got := child.Get(FromGoString("own")); got.scalar != Number(4).scalar {
		t.Fatalf("own read = %v, want 4", got)
	}
	if got := child.Get(FromGoString("missing")); got.kind != KindUndefined {
		t.Fatalf("miss off the end of the chain = %v, want undefined", got)
	}
}

// TestPrototypeChainIn proves the in operator climbs the chain, reporting true for
// a key a prototype supplies and false only when no object on the chain carries it.
func TestPrototypeChainIn(t *testing.T) {
	proto := NewObject()
	proto.Set(FromGoString("shared"), Number(1))
	child := ObjectCreate(proto)

	if !child.HasProperty(FromGoString("shared")) {
		t.Fatal("in did not find an inherited key")
	}
	if child.HasProperty(FromGoString("missing")) {
		t.Fatal("in found a key no object on the chain carries")
	}
}

// TestObjectCreateNullProto proves Object.create(null) returns a prototype-less
// object: its slot is nil so a read never climbs past its own bag.
func TestObjectCreateNullProto(t *testing.T) {
	o := ObjectCreate(Null)
	if o.object().proto != nil {
		t.Fatal("Object.create(null) left a non-nil prototype slot")
	}
}

// TestObjectCreatePrimitiveThrows proves Object.create rejects a prototype that is
// neither an object nor null with a TypeError.
func TestObjectCreatePrimitiveThrows(t *testing.T) {
	if !throws(func() { ObjectCreate(Number(1)) }) {
		t.Fatal("Object.create on a number prototype did not throw")
	}
}
