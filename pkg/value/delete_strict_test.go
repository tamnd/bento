package value

import (
	"strings"
	"testing"
)

// A strict-mode delete of a non-configurable property throws a TypeError rather
// than reporting false: DeleteStrict calls Delete and, when the removal is
// refused, throws with V8's "Cannot delete property '<key>' of #<Object>".
func TestDeleteStrictThrowsOnNonConfigurable(t *testing.T) {
	obj := NewObject()
	desc := NewObject()
	desc.Set(FromGoString("value"), Number(1))
	desc.Set(FromGoString("configurable"), False)
	obj.DefineProperty(StringValue(FromGoString("x")), desc)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("DeleteStrict on a non-configurable property did not throw")
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("thrown value is %T, want *Error", r)
		}
		if !e.IsA("TypeError") {
			t.Fatalf("thrown error is %s, want TypeError", e.name.ToGoString())
		}
		if got := e.message.ToGoString(); !strings.Contains(got, "Cannot delete property 'x' of #<Object>") {
			t.Fatalf("message = %q, want it to name the refused property", got)
		}
	}()
	obj.DeleteStrict(FromGoString("x"))
}

// A strict-mode delete of a configurable property removes it and returns true,
// identical to Delete, so an ordinary strict delete does not throw.
func TestDeleteStrictConfigurableReturnsTrue(t *testing.T) {
	obj := NewObject()
	obj.Set(FromGoString("y"), Number(2))
	if !obj.DeleteStrict(FromGoString("y")) {
		t.Fatalf("DeleteStrict of a configurable property returned false, want true")
	}
}

// A strict-mode delete off a nullish base throws the same nullish TypeError Delete
// does, since DeleteStrict routes through Delete.
func TestDeleteStrictOnNullishThrows(t *testing.T) {
	for _, recv := range []Value{Null, Undefined} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("DeleteStrict on nullish did not throw")
				}
				if e, ok := r.(*Error); !ok || !e.IsA("TypeError") {
					t.Fatalf("thrown value %v, want a TypeError", r)
				}
			}()
			recv.DeleteStrict(FromGoString("prop"))
		}()
	}
}
