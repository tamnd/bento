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

// TestObjectCreatePrimitiveThrows proves Object.create rejects a prototype that is
// neither an object nor null with a TypeError.
func TestObjectCreatePrimitiveThrows(t *testing.T) {
	if !throws(func() { ObjectCreate(Number(1)) }) {
		t.Fatal("Object.create on a number prototype did not throw")
	}
}
