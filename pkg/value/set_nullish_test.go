package value

import (
	"testing"
)

// A named write to null or undefined is a TypeError, not the nil dereference the
// property storage would take, and the message matches V8 so a catch reads the same
// text Node reports. This is the PutValue path a base.prop = rhs assignment lowers to
// when the base is nullish.
func TestSetOnNullishThrowsTypeError(t *testing.T) {
	for _, tc := range []struct {
		name string
		recv Value
		want string
	}{
		{"null", Null, "Cannot set properties of null (setting 'prop')"},
		{"undefined", Undefined, "Cannot set properties of undefined (setting 'prop')"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Set on %s did not throw", tc.name)
				}
				e, ok := r.(*Error)
				if !ok {
					t.Fatalf("thrown value is %T, want *Error", r)
				}
				if !e.IsA("TypeError") {
					t.Fatalf("thrown error is %s, want TypeError", e.name.ToGoString())
				}
				if got := e.message.ToGoString(); got != tc.want {
					t.Fatalf("message = %q, want %q", got, tc.want)
				}
			}()
			tc.recv.Set(FromGoString("prop"), Number(1))
		})
	}
}

// The nullish guard must not disturb a real object's write.
func TestSetOnObjectStillWrites(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("x"), Number(2))
	if got := o.Get(FromGoString("x")); got.kind != KindNumber || ToString(got).ToGoString() != "2" {
		t.Fatalf("object write lost: got %v", ToString(got))
	}
}
