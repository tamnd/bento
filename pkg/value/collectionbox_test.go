package value

import "testing"

// The box a Map or a Set takes when it crosses into a dynamic slot is a view rather
// than a copy, which is the whole of what these check: a write through the box lands
// in the typed storage the generated code keeps reading, one map has one box however
// many times it crosses, and a key the typed map could never hold is refused loudly
// instead of dropped. The rendering and the deep comparison of a box are held to real
// Node in nodeinspect_test.go and nodedeepequal_test.go; what is here is the behavior
// no reference file records.

// boxedMapOfNumbers builds the typed map generated code holds for a Map<number,
// number> and hands back both halves, so a test can write through one and read
// through the other.
func boxedMapOfNumbers(t *testing.T) (*Map[float64, float64], Value) {
	t.Helper()
	m := NewNumberMap[float64]()
	m.Set(1, 2)
	m.Set(3, 4)
	return m, m.ToValue()
}

// call reads a member off a box and calls it, the two steps every dynamic method
// call takes: the read finds the bound method and the call runs it.
func callMember(t *testing.T, box Value, name string, args ...Value) Value {
	t.Helper()
	fn := box.Get(FromGoString(name))
	if fn.Kind() != KindFunc {
		t.Fatalf("%s off the box = %v, want a function", name, fn.Kind())
	}
	return fn.Call(args...)
}

// TestABoxedMapReadsTheLiveMap covers the read half of the member surface against a
// typed map, the instantiation generated code actually holds.
func TestABoxedMapReadsTheLiveMap(t *testing.T) {
	_, box := boxedMapOfNumbers(t)

	if got := box.Get(FromGoString("size")); got.AsNumber() != 2 {
		t.Errorf("size = %v, want 2", got.AsNumber())
	}
	if got := callMember(t, box, "get", Number(1)); got.AsNumber() != 2 {
		t.Errorf("get(1) = %v, want 2", got.AsNumber())
	}
	if got := callMember(t, box, "has", Number(3)); !got.AsBool() {
		t.Error("has(3) = false, want true")
	}
	if got := callMember(t, box, "has", Number(9)); got.AsBool() {
		t.Error("has(9) = true, want false")
	}
	// A key of a kind the typed map could not hold is absent rather than an error: a
	// Map<number, number> has no string keys, so asking about one answers no.
	if got := callMember(t, box, "has", StringValue(FromGoString("1"))); got.AsBool() {
		t.Error(`has("1") on a number-keyed map = true, want false`)
	}
	if got := callMember(t, box, "get", StringValue(FromGoString("1"))); got.Kind() != KindUndefined {
		t.Errorf(`get("1") on a number-keyed map = %v, want undefined`, got.Kind())
	}
	if got := box.Get(FromGoString("nope")); got.Kind() != KindUndefined {
		t.Errorf("a name that is not a Map member = %v, want undefined", got.Kind())
	}
}

// TestAWriteThroughTheBoxLandsInTheTypedMap is the point of a view: the two sides are
// one collection, so the typed code that keeps running after a map was logged or
// passed to an any parameter sees what the dynamic side did to it.
func TestAWriteThroughTheBoxLandsInTheTypedMap(t *testing.T) {
	m, box := boxedMapOfNumbers(t)

	if got := callMember(t, box, "set", Number(5), Number(6)); !StrictEquals(got, box) {
		t.Error("set did not return the map itself")
	}
	if got := m.Get(5); got.IsUndefined() || got.Get() != 6 {
		t.Errorf("the typed map after a boxed set(5, 6) = %v, want 6", got)
	}
	m.Set(7, 8)
	if got := callMember(t, box, "get", Number(7)); got.AsNumber() != 8 {
		t.Errorf("the box after a typed Set(7, 8) = %v, want 8", got.AsNumber())
	}
	if got := callMember(t, box, "delete", Number(7)); !got.AsBool() {
		t.Error("delete(7) = false, want true")
	}
	if m.Has(7) {
		t.Error("the typed map still has 7 after a boxed delete")
	}
	callMember(t, box, "clear")
	if m.Size() != 0 {
		t.Errorf("the typed map after a boxed clear has size %v, want 0", m.Size())
	}
}

// TestAWriteThePlainMapCouldNotHoldThrows pins the loud failure. A Map<number,
// number> asked through its box to store a string key has nowhere to put it, and
// silently dropping the entry would leave a program's map missing a key it just set.
// The lowerer only boxes collections whose element types have a dynamic form, so this
// throw is not reachable from compiled code; it guards a map handed to a boxed
// callback by hand.
func TestAWriteThePlainMapCouldNotHoldThrows(t *testing.T) {
	_, box := boxedMapOfNumbers(t)
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("setting a string key on a number-keyed map did not throw")
			}
		}()
		callMember(t, box, "set", StringValue(FromGoString("x")), Number(1))
	}()
}

// TestOneMapHasOneBox pins the identity a reference type needs. Two crossings of the
// same map produce the same object, so `a === b` holds for two bindings of it and
// console.log of a structure holding it twice prints one value rather than two.
func TestOneMapHasOneBox(t *testing.T) {
	m := NewNumberMap[float64]()
	if !StrictEquals(m.ToValue(), m.ToValue()) {
		t.Error("two crossings of one map are not the same value")
	}
	if StrictEquals(NewNumberMap[float64]().ToValue(), NewNumberMap[float64]().ToValue()) {
		t.Error("two distinct maps share a box")
	}
	s := NewNumberSet()
	if !StrictEquals(s.ToValue(), s.ToValue()) {
		t.Error("two crossings of one set are not the same value")
	}
}

// TestABoxedCollectionLooksLikeAnObject covers what the language asks about a value
// before it asks anything else. A Map is an object, it is truthy, it carries no own
// keys, and it answers the class tag Object.prototype.toString reads.
func TestABoxedCollectionLooksLikeAnObject(t *testing.T) {
	_, box := boxedMapOfNumbers(t)
	set := NewNumberSet().ToValue()

	if box.Kind() != KindObject {
		t.Errorf("a boxed map is kind %v, want an object", box.Kind())
	}
	if !ToBoolean(box) {
		t.Error("a boxed map is falsy, want truthy")
	}
	if got := ClassTag(box).ToGoString(); got != "[object Map]" {
		t.Errorf("ClassTag(map) = %q, want [object Map]", got)
	}
	if got := ClassTag(set).ToGoString(); got != "[object Set]" {
		t.Errorf("ClassTag(set) = %q, want [object Set]", got)
	}
	// A Map has no own enumerable properties, so JSON.stringify writes {} and a key
	// walk finds nothing, both of which Node does and both of which installing the
	// methods as properties would have broken.
	if got := JSONStringify(box).ToGoString(); got != "{}" {
		t.Errorf("JSON.stringify(map) = %q, want {}", got)
	}
	if got := int(box.OwnEnumerableKeys().Len()); got != 0 {
		t.Errorf("a boxed map has %d own enumerable keys, want 0", got)
	}
	// `in` reads the member surface, so 'size' is present and an unrelated name is not.
	if !box.HasProperty(FromGoString("size")) {
		t.Error("'size' in map = false, want true")
	}
	if !set.HasProperty(FromGoString("add")) {
		t.Error("'add' in set = false, want true")
	}
	if box.HasProperty(FromGoString("nope")) {
		t.Error("'nope' in map = true, want false")
	}
}

// TestABoxedCollectionCoercesToItsTag pins String(map). Neither valueOf nor toString
// is on the box, so the coercion falls to the ordinary object form, which is the
// class tag rather than a flat "[object Object]".
func TestABoxedCollectionCoercesToItsTag(t *testing.T) {
	_, box := boxedMapOfNumbers(t)
	if got := ToString(box).ToGoString(); got != "[object Map]" {
		t.Errorf("String(map) = %q, want [object Map]", got)
	}
	if got := ToString(NewNumberSet().ToValue()).ToGoString(); got != "[object Set]" {
		t.Errorf("String(set) = %q, want [object Set]", got)
	}
}

// TestABoxedMethodCarriesItsName pins what console.log of a method read off a box
// prints. Node names its built-ins, so a logged m.get reads "[Function: get]", and an
// unnamed function box would have read "[Function (anonymous)]".
func TestABoxedMethodCarriesItsName(t *testing.T) {
	_, box := boxedMapOfNumbers(t)
	got := box.Get(FromGoString("get")).Get(FromGoString("name")).AsString().ToGoString()
	if got != "get" {
		t.Errorf("map.get.name = %q, want get", got)
	}
	if got := NodeInspect(box.Get(FromGoString("get"))).ToGoString(); got != "[Function: get]" {
		t.Errorf("inspect(map.get) = %q, want [Function: get]", got)
	}
}

// TestIteratingABoxedMap walks the four ways a program reads a map's entries out. The
// default iterator is entries, which is what a for...of and a spread take.
func TestIteratingABoxedMap(t *testing.T) {
	_, box := boxedMapOfNumbers(t)

	entries := IterateToSlice(box, "m")
	if len(entries) != 2 {
		t.Fatalf("for...of over a map yielded %d entries, want 2", len(entries))
	}
	if k := entries[0].GetIndex(0).AsNumber(); k != 1 {
		t.Errorf("the first entry's key = %v, want 1", k)
	}
	if v := entries[1].GetIndex(1).AsNumber(); v != 4 {
		t.Errorf("the second entry's value = %v, want 4", v)
	}

	keys := IterateToSlice(callMember(t, box, "keys"), "m.keys()")
	if len(keys) != 2 || keys[0].AsNumber() != 1 || keys[1].AsNumber() != 3 {
		t.Errorf("keys() = %v, want 1 and 3", keys)
	}
	values := IterateToSlice(callMember(t, box, "values"), "m.values()")
	if len(values) != 2 || values[0].AsNumber() != 2 || values[1].AsNumber() != 4 {
		t.Errorf("values() = %v, want 2 and 4", values)
	}

	// forEach hands its callback (value, key, map), the reverse of the entry order
	// everywhere else, which is the argument order a partial port gets wrong.
	var seen []float64
	var third Value
	callMember(t, box, "forEach", NewFunc(func(args []Value) Value {
		seen = append(seen, Arg(args, 0).AsNumber(), Arg(args, 1).AsNumber())
		third = Arg(args, 2)
		return Undefined
	}))
	if len(seen) != 4 || seen[0] != 2 || seen[1] != 1 || seen[2] != 4 || seen[3] != 3 {
		t.Errorf("forEach saw %v, want value then key for each entry", seen)
	}
	if !StrictEquals(third, box) {
		t.Error("forEach's third argument is not the map itself")
	}
}

// TestABoxedSetReadsAndWritesTheLiveSet is the Set half of the member surface, the
// same shape with one column instead of two.
func TestABoxedSetReadsAndWritesTheLiveSet(t *testing.T) {
	s := NewNumberSet()
	s.Add(1)
	s.Add(2)
	box := s.ToValue()

	if got := box.Get(FromGoString("size")); got.AsNumber() != 2 {
		t.Errorf("size = %v, want 2", got.AsNumber())
	}
	if got := callMember(t, box, "has", Number(1)); !got.AsBool() {
		t.Error("has(1) = false, want true")
	}
	if got := callMember(t, box, "add", Number(3)); !StrictEquals(got, box) {
		t.Error("add did not return the set itself")
	}
	if !s.Has(3) {
		t.Error("the typed set did not see a boxed add")
	}
	if got := callMember(t, box, "delete", Number(1)); !got.AsBool() || s.Has(1) {
		t.Error("a boxed delete did not remove the member")
	}

	members := IterateToSlice(box, "s")
	if len(members) != 2 || members[0].AsNumber() != 2 || members[1].AsNumber() != 3 {
		t.Errorf("for...of over a set = %v, want 2 and 3", members)
	}
	// A set's entries pair each member with itself, which is what makes a Set and a
	// Map read the same way through the entries protocol.
	entries := IterateToSlice(callMember(t, box, "entries"), "s.entries()")
	if len(entries) != 2 || entries[0].GetIndex(0).AsNumber() != 2 || entries[0].GetIndex(1).AsNumber() != 2 {
		t.Errorf("entries() = %v, want each member twice", entries)
	}
	callMember(t, box, "clear")
	if s.Size() != 0 {
		t.Errorf("the typed set after a boxed clear has size %v, want 0", s.Size())
	}
}

// TestTheSetAlgebraIsNotOnTheBox records a deliberate gap rather than an oversight.
// union, intersection and the rest take a set-like argument, and a dynamic argument
// carries no typed member the receiver's storage could take, so they are left off the
// box: a read is undefined and a call raises "is not a function", which is a clear
// failure rather than a wrong answer. They are on the typed side already.
func TestTheSetAlgebraIsNotOnTheBox(t *testing.T) {
	box := NewNumberSet().ToValue()
	for _, name := range []string{"union", "intersection", "difference", "symmetricDifference",
		"isSubsetOf", "isSupersetOf", "isDisjointFrom"} {
		if got := box.Get(FromGoString(name)); got.Kind() != KindUndefined {
			t.Errorf("%s off a boxed set = %v, want undefined", name, got.Kind())
		}
	}
}

// TestTheBoxCarriesEveryKeyKind walks the other typed instantiations, since the
// member surface is written once against an interface and each one bridges its own
// element type through dynBox.
func TestTheBoxCarriesEveryKeyKind(t *testing.T) {
	strs := NewStringMap[BStr]()
	strs.Set(FromGoString("a"), FromGoString("x"))
	if got := callMember(t, strs.ToValue(), "get", StringValue(FromGoString("a"))).AsString().ToGoString(); got != "x" {
		t.Errorf(`a string map's get("a") = %q, want x`, got)
	}

	bools := NewBoolMap[bool]()
	bools.Set(true, false)
	if got := call(t, bools.ToValue(), "get", True); got.Kind() != KindBool || got.AsBool() {
		t.Errorf("a boolean map's get(true) = %v, want false", got)
	}

	// A dynamic map is the one a `new Map()` with no key type lowers to, and it holds
	// every kind at once, an object key beside a number one.
	dyn := NewDynMap[Value]()
	key := NewObject()
	dyn.Set(key, StringValue(FromGoString("v")))
	dyn.Set(Number(1), Number(2))
	box := dyn.ToValue()
	if got := callMember(t, box, "get", key).AsString().ToGoString(); got != "v" {
		t.Errorf("a dynamic map's get(object) = %q, want v", got)
	}
	if got := callMember(t, box, "has", NewObject()); got.AsBool() {
		t.Error("a dynamic map matched a different object as the same key")
	}
	if got := box.Get(FromGoString("size")).AsNumber(); got != 2 {
		t.Errorf("a dynamic map's size = %v, want 2", got)
	}
}

// TestNegativeZeroIsStoredAsPositiveZero pins the one fold SameValueZero asks for.
// Map.prototype.set replaces -0 with +0 before it stores, so a map built from -0
// prints 0 and compares equal to one built from 0, which is what Node does.
func TestNegativeZeroIsStoredAsPositiveZero(t *testing.T) {
	m := NewNumberMap[float64]()
	m.Set(negZero(), 1)
	if got := NodeInspect(m.ToValue()).ToGoString(); got != "Map(1) { 0 => 1 }" {
		t.Errorf("a map keyed by -0 prints %q, want Map(1) { 0 => 1 }", got)
	}
	s := NewNumberSet()
	s.Add(negZero())
	if got := NodeInspect(s.ToValue()).ToGoString(); got != "Set(1) { 0 }" {
		t.Errorf("a set holding -0 prints %q, want Set(1) { 0 }", got)
	}
	d := NewDynSet()
	d.Add(Number(negZero()))
	if got := NodeInspect(d.ToValue()).ToGoString(); got != "Set(1) { 0 }" {
		t.Errorf("a dynamic set holding -0 prints %q, want Set(1) { 0 }", got)
	}
}
