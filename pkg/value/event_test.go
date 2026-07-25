package value

import "testing"

// TestEventTargetRemoveByIdentity exercises the runtime EventTarget directly, the
// removeEventListener path the compiled tests cannot reach yet because a function local
// boxes a fresh value at each call site. Here the listener is one shared value.Value, so
// removeEventListener finds it by identity: after removing the first of two listeners the
// dispatch runs only the second. The listeners record their calls in a Go slice the test
// reads, standing in for the console output a compiled program would print.
func TestEventTargetRemoveByIdentity(t *testing.T) {
	var fired []string
	et := NewEventTarget()
	first := NewFunc(func(args []Value) Value { fired = append(fired, "first"); return Undefined })
	second := NewFunc(func(args []Value) Value { fired = append(fired, "second"); return Undefined })

	add := et.Get(FromGoString("addEventListener"))
	add.Call(StringValue(FromGoString("x")), first)
	add.Call(StringValue(FromGoString("x")), second)
	et.Get(FromGoString("removeEventListener")).Call(StringValue(FromGoString("x")), first)
	et.Get(FromGoString("dispatchEvent")).Call(NewEvent(StringValue(FromGoString("x")), Undefined))

	if len(fired) != 1 || fired[0] != "second" {
		t.Fatalf("want [second], got %v", fired)
	}
}

// TestEventTargetDuplicateListenerIgnored pins that a second registration of the same
// listener for the same type is ignored, so the listener fires once per dispatch rather
// than twice. It adds the one shared listener twice and dispatches once.
func TestEventTargetDuplicateListenerIgnored(t *testing.T) {
	var count int
	et := NewEventTarget()
	fn := NewFunc(func(args []Value) Value { count++; return Undefined })

	add := et.Get(FromGoString("addEventListener"))
	add.Call(StringValue(FromGoString("go")), fn)
	add.Call(StringValue(FromGoString("go")), fn)
	et.Get(FromGoString("dispatchEvent")).Call(NewEvent(StringValue(FromGoString("go")), Undefined))

	if count != 1 {
		t.Fatalf("want the listener to fire once, fired %d times", count)
	}
}

// TestEventTargetListenerAddedDuringDispatchWaits pins the snapshot rule: a listener a
// running listener adds during a dispatch does not run in that same dispatch, only in a
// later one. The first listener adds a second on its first run; after one dispatch only
// the first has fired, and after a second dispatch both have.
func TestEventTargetListenerAddedDuringDispatchWaits(t *testing.T) {
	var fired []string
	et := NewEventTarget()
	add := et.Get(FromGoString("addEventListener"))
	late := NewFunc(func(args []Value) Value { fired = append(fired, "late"); return Undefined })
	early := NewFunc(func(args []Value) Value {
		fired = append(fired, "early")
		add.Call(StringValue(FromGoString("x")), late)
		return Undefined
	})
	add.Call(StringValue(FromGoString("x")), early)

	dispatch := et.Get(FromGoString("dispatchEvent"))
	dispatch.Call(NewEvent(StringValue(FromGoString("x")), Undefined))
	if len(fired) != 1 || fired[0] != "early" {
		t.Fatalf("after first dispatch want [early], got %v", fired)
	}
	dispatch.Call(NewEvent(StringValue(FromGoString("x")), Undefined))
	if len(fired) != 3 || fired[1] != "early" || fired[2] != "late" {
		t.Fatalf("after second dispatch want [early early late], got %v", fired)
	}
}
