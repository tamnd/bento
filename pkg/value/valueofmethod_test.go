package value

import "testing"

// TestValueOfMethodPrimitives pins that a dynamic x.valueOf() returns each primitive
// unchanged, the way its prototype valueOf does: the receiver is its own primitive
// value, so the method is the identity on a number, boolean, or string.
func TestValueOfMethodPrimitives(t *testing.T) {
	cases := []struct {
		name string
		in   Value
	}{
		{"number", Number(42)},
		{"true", Bool(true)},
		{"false", Bool(false)},
		{"string", StringValue(FromGoString("hi"))},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.in.ValueOfMethod()
			if !StrictEquals(got, tc.in) {
				t.Fatalf("%v.valueOf() = %v, want the receiver unchanged", tc.in, got)
			}
		})
	}
}

// TestValueOfMethodObjectIdentity pins that a plain object valueOf returns the object
// itself, the default Object.prototype.valueOf result, so the receiver and the answer
// are the same reference.
func TestValueOfMethodObjectIdentity(t *testing.T) {
	o := NewObject()
	got := o.ValueOfMethod()
	if !StrictEquals(got, o) {
		t.Fatalf("({}).valueOf() = %v, want the same object", got)
	}
}

// TestValueOfMethodNullish pins that reading valueOf off undefined or null throws a
// TypeError, since neither carries a prototype to read the method from.
func TestValueOfMethodNullish(t *testing.T) {
	for _, v := range []Value{Undefined, Null} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("%v.valueOf() did not throw", v)
				}
			}()
			v.ValueOfMethod()
		}()
	}
}
