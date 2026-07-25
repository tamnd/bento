package value

import "testing"

// PlusToString coerces a + operand with the default hint, where a plain ToString
// uses the string hint. For an object whose valueOf returns a primitive the two
// disagree: + must take valueOf (default hint), so "" + obj reads "42", while a
// String(obj) coercion takes toString and reads "s".
func TestPlusToStringUsesDefaultHint(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("valueOf"), NewFunc(func(args []Value) Value { return Number(42) }))
	o.Set(FromGoString("toString"), NewFunc(func(args []Value) Value { return str("s") }))

	if got := PlusToString(o).ToGoString(); got != "42" {
		t.Errorf("PlusToString should take the default-hint valueOf: want %q, got %q", "42", got)
	}
	if got := ToString(o).ToGoString(); got != "s" {
		t.Errorf("ToString should take the string-hint toString: want %q, got %q", "s", got)
	}
	// The + branch and PlusToString agree, since both use the default hint.
	if got := Add(StringValue(FromGoString("")), o).str().ToGoString(); got != "42" {
		t.Errorf("Add and PlusToString should agree: want %q, got %q", "42", got)
	}
}

// A Symbol.toPrimitive that reads its hint back proves + passes "default": the
// method returns the hint name, so "" + obj concatenates "default".
func TestPlusToStringPassesDefaultToSymbol(t *testing.T) {
	o := NewObject()
	o.setSymKey(symbolToPrimitive, NewFunc(func(args []Value) Value { return Arg(args, 0) }))
	if got := PlusToString(o).ToGoString(); got != "default" {
		t.Errorf("Symbol.toPrimitive should receive the default hint: got %q", got)
	}
}

// A plain object and an array are hint-insensitive: valueOf is not primitive, so
// both hints fall to toString and PlusToString matches ToString.
func TestPlusToStringPlainObjectAndArrayUnchanged(t *testing.T) {
	if got := PlusToString(NewObject()).ToGoString(); got != "[object Object]" {
		t.Errorf("plain object: got %q", got)
	}
	arr := NewArrayValue([]Value{Number(1), Number(2)})
	if got := PlusToString(arr).ToGoString(); got != "1,2" {
		t.Errorf("array: got %q", got)
	}
}
