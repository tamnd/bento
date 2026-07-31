package value

import "testing"

// The four weakly-holding kinds box into a dynamic value the way a Map does, with one
// difference that is the whole point of them: there is nothing to walk. Node prints a
// placeholder where a Map prints entries, and these hold that along with the view, the
// round trip, and the reference-only deep comparison.

// TestAWeakCollectionRendersLikeNode holds what the inspector prints. A WeakMap and a
// WeakSet name themselves and stand in the unreadable items; a WeakRef and a
// FinalizationRegistry claim no items and are the empty pair.
func TestAWeakCollectionRendersLikeNode(t *testing.T) {
	cases := []struct {
		name string
		box  Value
		want string
	}{
		{"weakMap", NewWeakMap[pointClass, float64]().ToValue(), "WeakMap { <items unknown> }"},
		{"weakSet", NewWeakSet[pointClass]().ToValue(), "WeakSet { <items unknown> }"},
		{"weakRef", NewWeakRef(newPoint()).ToValue(), "WeakRef {}"},
		{"registry", NewFinalizationRegistry(func(float64) {}).ToValue(), "FinalizationRegistry {}"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NodeInspect(tc.box).ToGoString(); got != tc.want {
				t.Errorf("renders as %s, want %s", got, tc.want)
			}
		})
	}
}

// TestAWeakMapBoxIsAView holds the read and the write half at once. A WeakMap has one
// box however many times it crosses, and a set made through the box is an entry the
// typed get finds, because the box is the map rather than a snapshot of it.
func TestAWeakMapBoxIsAView(t *testing.T) {
	m := NewWeakMap[pointClass, float64]()
	p, q := newPoint(), newPoint()
	m.Set(p, 1)
	box := m.ToValue()

	if !StrictEquals(box, m.ToValue()) {
		t.Error("two crossings of one WeakMap handed back two objects")
	}
	if got := callMember(t, box, "get", dynBox(p)); got.AsNumber() != 1 {
		t.Errorf("get through the box = %v, want 1", got.AsNumber())
	}
	if got := callMember(t, box, "set", dynBox(q), Number(2)); !StrictEquals(got, box) {
		t.Error("set did not return the map itself")
	}
	if got := m.Get(q); got.IsUndefined() || got.Get() != 2 {
		t.Error("a set through the box did not reach the typed map")
	}
	if got := callMember(t, box, "delete", dynBox(q)); !got.AsBool() {
		t.Error("delete through the box = false, want true")
	}
	if m.Has(q) {
		t.Error("the typed map still holds the key a boxed delete removed")
	}
	// A key the map has never held reads as undefined rather than raising, and a key of a
	// kind it could not hold is absent for the same reason a Map's is.
	if got := callMember(t, box, "get", dynBox(newPoint())); got.Kind() != KindUndefined {
		t.Errorf("get of an absent key = %v, want undefined", got.Kind())
	}
	if got := callMember(t, box, "has", Number(1)); got.AsBool() {
		t.Error("has of a number on an instance-keyed WeakMap = true, want false")
	}
	// A WeakMap has no size, so the read climbs the ordinary chain and ends at undefined
	// the way a read of any unrelated name off one does.
	if got := box.Get(FromGoString("size")); got.Kind() != KindUndefined {
		t.Errorf("a WeakMap box answered a size of %v", got.Kind())
	}
}

// TestAWeakSetAndWeakRefReadThroughTheBox holds the other two member surfaces. A
// WeakSet is the WeakMap case with a member rather than a pair, and a WeakRef's deref
// reads the target the typed side still holds.
func TestAWeakSetAndWeakRefReadThroughTheBox(t *testing.T) {
	s := NewWeakSet[pointClass]()
	p := newPoint()
	sbox := s.ToValue()

	if got := callMember(t, sbox, "add", dynBox(p)); !StrictEquals(got, sbox) {
		t.Error("add did not return the set itself")
	}
	if !s.Has(p) {
		t.Error("an add through the box did not reach the typed set")
	}
	if got := callMember(t, sbox, "has", dynBox(p)); !got.AsBool() {
		t.Error("has through the box = false, want true")
	}
	if got := callMember(t, sbox, "delete", dynBox(p)); !got.AsBool() || s.Has(p) {
		t.Error("a delete through the box did not reach the typed set")
	}

	rbox := NewWeakRef(p).ToValue()
	got := callMember(t, rbox, "deref")
	if inst, ok := got.classInstance(); !ok || inst.(*pointClass) != p {
		t.Errorf("deref through the box = %s, want the target instance", NodeInspect(got).ToGoString())
	}
}

// TestAWeakCollectionUnboxesToItself holds the direction a Map keyed by WeakMaps needs.
// The box is the view kept on the collection, so it comes back as the very collection
// the typed side holds, and a box of one kind is not a member of another.
func TestAWeakCollectionUnboxesToItself(t *testing.T) {
	m := NewWeakMap[pointClass, float64]()
	if got, ok := dynUnbox[*WeakMap[pointClass, float64]](dynBox(m)); !ok || got != m {
		t.Error("a boxed WeakMap did not unbox to itself")
	}
	s := NewWeakSet[pointClass]()
	if got, ok := dynUnbox[*WeakSet[pointClass]](dynBox(s)); !ok || got != s {
		t.Error("a boxed WeakSet did not unbox to itself")
	}
	r := NewWeakRef(newPoint())
	if got, ok := dynUnbox[*WeakRef[pointClass]](dynBox(r)); !ok || got != r {
		t.Error("a boxed WeakRef did not unbox to itself")
	}
	if _, ok := dynUnbox[*WeakSet[pointClass]](dynBox(m)); ok {
		t.Error("a boxed WeakMap unboxed as a WeakSet")
	}
	if _, ok := dynUnbox[*Map[BStr, float64]](dynBox(m)); ok {
		t.Error("a boxed WeakMap unboxed as a Map")
	}
}

// TestAWeakCollectionComparesByReference holds the one branch node could not write any
// other way. Two WeakMaps hold contents nobody may read, so two distinct empty ones are
// not deeply equal however alike they print, while one compared against itself is.
func TestAWeakCollectionComparesByReference(t *testing.T) {
	m := NewWeakMap[pointClass, float64]().ToValue()
	if !DeepStrictEqual(m, m) {
		t.Error("a WeakMap is not deeply equal to itself")
	}
	if DeepStrictEqual(m, NewWeakMap[pointClass, float64]().ToValue()) {
		t.Error("two distinct WeakMaps compared deeply equal")
	}
	if DeepStrictEqual(m, NewWeakSet[pointClass]().ToValue()) {
		t.Error("a WeakMap compared deeply equal to a WeakSet")
	}
	if DeepStrictEqual(m, NewObject()) || DeepStrictEqual(NewObject(), m) {
		t.Error("a WeakMap compared deeply equal to a plain object")
	}
	// The tag is what keeps the plain-object walk from reaching two empty property
	// tables and calling them equal, so it is pinned directly too.
	if got := ClassTag(m).ToGoString(); got != "[object WeakMap]" {
		t.Errorf("a boxed WeakMap tags as %s", got)
	}
}

// TestARegistryRegisterRaisesThroughTheBox holds the one member of the four a box
// cannot serve. Registering wires the target to runtime.AddCleanup, which is generic
// over the target's own Go type, and that type is what a boxed call does not have: the
// emitted typed call site is where it is known. Raising is honest where recording a
// registration whose cleanup could never fire would not be. Unregistering needs no such
// type, since a token is matched by the reference identity a boxed instance carries.
func TestARegistryRegisterRaisesThroughTheBox(t *testing.T) {
	r := NewFinalizationRegistry(func(float64) {})
	box := r.ToValue()

	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("register through the box did not raise")
			}
		}()
		callMember(t, box, "register", dynBox(newPoint()), Number(1))
	}()

	if got := callMember(t, box, "unregister", dynBox(newPoint())); got.AsBool() {
		t.Error("unregister of a token that was never registered = true, want false")
	}
}
