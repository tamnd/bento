package value

import "testing"

// A boxed class instance is a view of the instance rather than a copy of its fields.
// These hold the two halves of that: a read through the box answers what the instance
// holds now, and a write through the box lands in the instance. The view is what lets a
// collection hold instances at all, since a collection's box is itself a view and its
// element boxing has to run in both directions.

// viewClass carries one field of each kind the view converts back on a write: the three
// primitives, a reference to a built-in with a box of its own, and another instance.
type viewClass struct {
	N float64     `json:"n"`
	S BStr        `json:"s"`
	B bool        `json:"b"`
	D *Date       `json:"d"`
	P *pointClass `json:"p"`
}

// opaqueClass carries a field whose Go type has no inbound conversion, the shape a
// write through the view has to refuse rather than store where nobody reads it.
type opaqueClass struct {
	F []float64 `json:"f"`
}

func init() {
	RegisterClass("View", (*viewClass)(nil))
	RegisterClass("Opaque", (*opaqueClass)(nil))
}

// TestABoxedInstanceReadsTheInstanceNow holds the read half. The instance keeps living
// on the typed side after it is boxed, so a field changed after the boxing has to read
// through, which a copy of the fields could not do.
func TestABoxedInstanceReadsTheInstanceNow(t *testing.T) {
	p := newPoint()
	box := ObjectFromStruct(p)

	p.X = 9
	p.Y = FromGoString("t")

	if got := box.Get(FromGoString("x")).AsNumber(); got != 9 {
		t.Errorf("the boxed .x after the instance changed = %v, want 9", got)
	}
	if got := NodeInspect(box).ToGoString(); got != "Point { x: 9, y: 't' }" {
		t.Errorf("the box renders as %s, want Point { x: 9, y: 't' }", got)
	}
	if got := JSONStringify(box).ToGoString(); got != `{"x":9,"y":"t"}` {
		t.Errorf("JSON.stringify of the box = %s", got)
	}
}

// TestAWriteThroughABoxedInstanceLands holds the write half, one field per conversion.
// A reference field takes the very object the value boxes rather than a copy, so the two
// sides do not drift apart after the write.
func TestAWriteThroughABoxedInstanceLands(t *testing.T) {
	inner := newPoint()
	v := &viewClass{}
	box := ObjectFromStruct(v)

	box.Set(FromGoString("n"), Number(3))
	box.Set(FromGoString("s"), StringValue(FromGoString("t")))
	box.Set(FromGoString("b"), True)
	box.Set(FromGoString("d"), NewDateFromMillis(0).ToValue())
	box.Set(FromGoString("p"), ObjectFromStruct(inner))

	if v.N != 3 {
		t.Errorf("the instance's number field = %v, want 3", v.N)
	}
	if got := v.S.ToGoString(); got != "t" {
		t.Errorf("the instance's string field = %q, want \"t\"", got)
	}
	if !v.B {
		t.Error("the instance's boolean field did not take the write")
	}
	if v.D == nil || v.D.ValueOf() != 0 {
		t.Errorf("the instance's date field = %v, want the epoch", v.D)
	}
	if v.P != inner {
		t.Error("the instance's class field did not take the very instance that was written")
	}
}

// TestAFieldOfABoxedInstanceIsADataProperty holds the thing that makes the view
// invisible. A getter and setter pair would read the same way but is observable: Node
// prints [Getter/Setter] rather than the value, and getOwnPropertyDescriptor answers get
// and set. A field of an instance is an ordinary data property and has to look like one.
func TestAFieldOfABoxedInstanceIsADataProperty(t *testing.T) {
	box := ObjectFromStruct(newPoint())

	desc, ok := box.object().getOwnDesc(FromGoString("x"))
	if !ok {
		t.Fatal("the boxed instance owns no x")
	}
	if desc.isAccessor() {
		t.Error("a field of a boxed instance is an accessor property, want a data property")
	}
	if !desc.writable || !desc.enumerable || !desc.configurable {
		t.Errorf("a field of a boxed instance has attributes %v %v %v, want all true",
			desc.writable, desc.enumerable, desc.configurable)
	}
	if got := NodeInspectArgs(box, objectOf("showHidden", Bool(true))).ToGoString(); got != "Point { x: 1, y: 's' }" {
		t.Errorf("the box under showHidden renders as %s, want Point { x: 1, y: 's' }", got)
	}
	// getOwnPropertyDescriptor reports the value the field holds now, the same read the
	// property itself answers, rather than the zero a live slot stores in its value word.
	got := box.GetOwnPropertyDescriptor(StringValue(FromGoString("x")))
	if n := got.Get(FromGoString("value")).AsNumber(); n != 1 {
		t.Errorf("the descriptor's value = %v, want 1", n)
	}
	if !got.Get(FromGoString("writable")).AsBool() {
		t.Error("the descriptor is not writable")
	}
}

// TestAWriteAValueCannotFitRaises holds the two ways a write through the view fails.
// Both raise rather than store the value in the box, where it would read back correctly
// once and never reach the instance, which is a lost write dressed as a successful one.
func TestAWriteAValueCannotFitRaises(t *testing.T) {
	box := ObjectFromStruct(&viewClass{})
	if err := caught(func() { box.Set(FromGoString("n"), StringValue(FromGoString("x"))) }); err == "" {
		t.Error("writing a string to a number field through the box did not raise")
	}

	opaque := ObjectFromStruct(&opaqueClass{})
	if err := caught(func() { opaque.Set(FromGoString("f"), Number(1)) }); err == "" {
		t.Error("writing to a field whose type has no dynamic form did not raise")
	}
	// A field of another class does not take an instance of this one, which arrives at the
	// store as the same back-pointer any instance would.
	if err := caught(func() { box.Set(FromGoString("p"), ObjectFromStruct(&twinClass{})) }); err == "" {
		t.Error("writing an instance of another class to a class field did not raise")
	}
}

// TestABoxedInstanceUnboxesToTheInstance holds what the view carries that a copy could
// not: the pointer back to the instance. It is what a boxed collection reads to turn a
// value handed to it into the element the typed side holds.
func TestABoxedInstanceUnboxesToTheInstance(t *testing.T) {
	p := newPoint()
	box := dynBox(p)

	got, ok := dynUnbox[*pointClass](box)
	if !ok {
		t.Fatal("a boxed instance did not unbox")
	}
	if got != p {
		t.Error("a boxed instance unboxed to another instance")
	}
	// A plain object carrying the same fields is not a member and could never have been:
	// nothing put it in the collection, so it carries no pointer back to anything.
	plain := NewObject()
	plain.Set(FromGoString("x"), Number(1))
	if _, ok := dynUnbox[*pointClass](plain); ok {
		t.Error("a plain object with an instance's fields unboxed as an instance")
	}
	// An instance of another class does not fit either, since the pointer it carries is
	// not of this collection's element type.
	if _, ok := dynUnbox[*pointClass](dynBox(&twinClass{})); ok {
		t.Error("an instance of another class unboxed as this one")
	}
}

// TestACollectionOfInstancesReadsThrough holds the capability the view is for. A typed
// map and set holding instances present them as boxes, find them again by the instance,
// and show what the instance holds now rather than what it held when it was stored.
func TestACollectionOfInstancesReadsThrough(t *testing.T) {
	p := newPoint()

	m := NewStringMap[*pointClass]()
	m.Set(FromGoString("k"), p)
	if got := NodeInspect(m.ToValue()).ToGoString(); got != "Map(1) { 'k' => Point { x: 1, y: 's' } }" {
		t.Errorf("a map of instances renders as %s", got)
	}

	s := NewRefSet[*pointClass]()
	s.Add(p)
	if got := NodeInspect(s.ToValue()).ToGoString(); got != "Set(1) { Point { x: 1, y: 's' } }" {
		t.Errorf("a set of instances renders as %s", got)
	}
	if !s.ToValue().Get(FromGoString("has")).Call(dynBox(p)).AsBool() {
		t.Error("a set of instances did not find a member handed back through its own box")
	}

	p.X = 9
	if got := NodeInspect(m.ToValue()).ToGoString(); got != "Map(1) { 'k' => Point { x: 9, y: 's' } }" {
		t.Errorf("a map of instances after the instance changed renders as %s", got)
	}
}

// cycleClass points at another instance of its own class, which is the shape a view
// makes reachable: the copying box walked the fields eagerly and ran off the stack on a
// cycle, while a view reads a field only when the field is read.
type cycleClass struct {
	V    float64     `json:"v"`
	Next *cycleClass `json:"next"`
}

func init() { RegisterClass("Cycle", (*cycleClass)(nil)) }

// TestACycleThroughInstancesIsCaught holds the cycle marker. A view is made fresh at
// each read, so the walk cannot recognize a repeat by the box; the instance behind the
// view is what repeats and is what it remembers instead.
func TestACycleThroughInstancesIsCaught(t *testing.T) {
	a := &cycleClass{V: 1}
	a.Next = a

	if got := NodeInspect(ObjectFromStruct(a)).ToGoString(); got != "<ref *1> Cycle { v: 1, next: [Circular *1] }" {
		t.Errorf("a self-referential instance renders as %s, want <ref *1> Cycle { v: 1, next: [Circular *1] }", got)
	}

	// A cycle two instances long is the same test with the marker on the outer one, which
	// is where Node puts it: the reference is numbered at the value the cycle points back at.
	b := &cycleClass{V: 2}
	a.Next = b
	b.Next = a
	want := "<ref *1> Cycle { v: 1, next: Cycle { v: 2, next: [Circular *1] } }"
	if got := NodeInspect(ObjectFromStruct(a)).ToGoString(); got != want {
		t.Errorf("a two-step cycle renders as %s, want %s", got, want)
	}
}

// caught runs f and returns the message of the JavaScript exception it threw, or the
// empty string when it threw none, so a test can hold a raise without unwinding itself.
func caught(f func()) (msg string) {
	defer func() {
		if r := recover(); r != nil {
			if e, ok := r.(Thrown); ok {
				msg = ToString(Caught(e).ToValue()).ToGoString()
				return
			}
			msg = "panic"
		}
	}()
	f()
	return ""
}
