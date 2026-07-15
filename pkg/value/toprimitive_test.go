package value

import "testing"

// methodObj builds a plain object carrying the given named callable, each reading
// its this receiver from args[0] the way a boxed method does, so a coercion that
// looks up the name finds a function it can call with the object as this.
func methodObj(name string, fn func(this Value, args []Value) Value) Value {
	o := NewObject()
	o.Set(FromGoString(name), NewFunc(func(args []Value) Value {
		this := Arg(args, 0)
		return fn(this, args[1:])
	}))
	return o
}

func TestToPrimitiveValueOfCoercesForEquality(t *testing.T) {
	o := methodObj("valueOf", func(this Value, args []Value) Value { return Number(5) })
	if !LooseEquals(o, Number(5)) {
		t.Fatalf("{ valueOf: () => 5 } == 5 should be true")
	}
	if !LooseEquals(Number(5), o) {
		t.Fatalf("5 == { valueOf: () => 5 } should be true (object on the right)")
	}
	if LooseEquals(o, Number(6)) {
		t.Fatalf("{ valueOf: () => 5 } == 6 should be false")
	}
}

func TestToPrimitiveValueOfCoercesForRelational(t *testing.T) {
	o := methodObj("valueOf", func(this Value, args []Value) Value { return Number(5) })
	if !Less(o, Number(10)) {
		t.Fatalf("{ valueOf: () => 5 } < 10 should be true")
	}
	if !Greater(o, Number(2)) {
		t.Fatalf("{ valueOf: () => 5 } > 2 should be true")
	}
	if Less(o, Number(1)) {
		t.Fatalf("{ valueOf: () => 5 } < 1 should be false")
	}
}

func TestToPrimitiveFallsToToStringWhenValueOfIsNotPrimitive(t *testing.T) {
	// valueOf returns an object, so the number hint rejects it and toString wins.
	o := NewObject()
	o.Set(FromGoString("valueOf"), NewFunc(func(args []Value) Value { return NewObject() }))
	o.Set(FromGoString("toString"), NewFunc(func(args []Value) Value { return str("7") }))
	if !LooseEquals(o, Number(7)) {
		t.Fatalf("object whose valueOf is non-primitive should fall to toString and equal 7")
	}
}

func TestToPrimitiveSymbolToPrimitiveWinsOverValueOf(t *testing.T) {
	o := methodObj("valueOf", func(this Value, args []Value) Value { return Number(5) })
	o.setSymKey(symbolToPrimitive, NewFunc(func(args []Value) Value { return Number(42) }))
	if !LooseEquals(o, Number(42)) {
		t.Fatalf("Symbol.toPrimitive should take precedence over valueOf")
	}
}

func TestToPrimitiveSymbolReceivesHint(t *testing.T) {
	// The exotic method returns the hint it was handed, so each coercion context can
	// be read back: a relational number hint, a string coercion's string hint, and a
	// loose-equality default hint.
	o := NewObject()
	o.setSymKey(symbolToPrimitive, NewFunc(func(args []Value) Value {
		hint := Arg(args, 1)
		switch hint.str().ToGoString() {
		case "number":
			return Number(1)
		case "string":
			return str("S")
		default:
			return str("D")
		}
	}))
	if !Less(o, Number(2)) {
		t.Fatalf("relational should pass the number hint (1 < 2)")
	}
	if got := ToString(o).ToGoString(); got != "S" {
		t.Fatalf("ToString should pass the string hint, got %q", got)
	}
	if !LooseEquals(o, str("D")) {
		t.Fatalf("loose equality against a string should pass the default hint")
	}
}

func TestToPrimitivePlainObjectKeepsOrdinaryString(t *testing.T) {
	// A plain object with no coercion methods still spells "[object Object]" and an
	// array still joins, the regression guard that the new lookup does not disturb
	// the values a proto-less dynamic object produced before.
	if got := ToString(NewObject()).ToGoString(); got != "[object Object]" {
		t.Fatalf("plain object ToString = %q, want [object Object]", got)
	}
	arr := NewArrayValue([]Value{Number(1), Number(2), Number(3)})
	if got := ToString(arr).ToGoString(); got != "1,2,3" {
		t.Fatalf("array ToString = %q, want 1,2,3", got)
	}
	if !LooseEquals(arr, str("1,2,3")) {
		t.Fatalf("array should loose-equal its joined string")
	}
}
