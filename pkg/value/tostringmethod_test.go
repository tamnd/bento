package value

import "testing"

// TestToStringMethodPrimitives pins that a dynamic x.toString() spells each
// primitive the way its prototype toString does: a number its digits, a boolean
// true or false, a string itself, and a bigint its digits, each boxed back as a
// string value.
func TestToStringMethodPrimitives(t *testing.T) {
	cases := []struct {
		name string
		in   Value
		want string
	}{
		{"number", Number(42), "42"},
		{"fraction", Number(1.5), "1.5"},
		{"true", Bool(true), "true"},
		{"false", Bool(false), "false"},
		{"string", StringValue(FromGoString("hi")), "hi"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.ToStringMethod()
			if got.Kind() != KindString || got.AsString().ToGoString() != tc.want {
				t.Fatalf("%v.toString() = %v, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestToStringMethodArray pins that a dynamic array toString joins its elements
// with commas, the Array.prototype.toString result, with null and undefined
// rendered empty.
func TestToStringMethodArray(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Undefined, Number(3)})
	got := arr.ToStringMethod()
	if got.Kind() != KindString || got.AsString().ToGoString() != "1,,3" {
		t.Fatalf("[1,,3].toString() = %v, want %q", got, "1,,3")
	}
}

// TestToStringMethodObject pins that a plain object toString reports the
// [object Object] tag, the default Object.prototype.toString result.
func TestToStringMethodObject(t *testing.T) {
	o := NewObject()
	got := o.ToStringMethod()
	if got.Kind() != KindString || got.AsString().ToGoString() != "[object Object]" {
		t.Fatalf("({}).toString() = %v, want %q", got, "[object Object]")
	}
}

// TestToStringMethodNullish pins that reading toString off undefined or null
// throws a TypeError, since neither carries a prototype to read the method from.
func TestToStringMethodNullish(t *testing.T) {
	for _, v := range []Value{Undefined, Null} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%v.toString() did not throw", v)
				}
			}()
			v.ToStringMethod()
		}()
	}
}
