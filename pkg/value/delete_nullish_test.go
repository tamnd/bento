package value

import (
	"strings"
	"testing"
)

// delete base.k and delete base[k] evaluate ToObject(base) before removing
// anything, and ToObject throws on a nullish base, so a delete off null or
// undefined throws a TypeError rather than reporting a boolean. The message
// mirrors V8's "Cannot convert undefined or null to object".
func TestDeleteOnNullishThrows(t *testing.T) {
	for _, recv := range []Value{Null, Undefined} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("Delete on nullish did not throw")
				}
				e, ok := r.(*Error)
				if !ok {
					t.Fatalf("thrown value is %T, want *Error", r)
				}
				if !e.IsA("TypeError") {
					t.Fatalf("thrown error is %s, want TypeError", e.name.ToGoString())
				}
				if got := e.message.ToGoString(); !strings.Contains(got, "Cannot convert undefined or null to object") {
					t.Fatalf("message = %q, want it to mention converting nullish to object", got)
				}
			}()
			recv.Delete(FromGoString("prop"))
		}()
	}
}

// The computed-index and dynamic-key delete paths dispatch through Delete, so a
// numeric or dynamic-string index off a nullish base throws the same TypeError.
func TestDeleteIndexAndElemOnNullishThrow(t *testing.T) {
	cases := []struct {
		name string
		call func(v Value)
	}{
		{"DeleteIndex", func(v Value) { v.DeleteIndex(0) }},
		{"DeleteElem string", func(v Value) { v.DeleteElem(StringValue(FromGoString("k"))) }},
	}
	for _, tc := range cases {
		for _, recv := range []Value{Null, Undefined} {
			t.Run(tc.name, func(t *testing.T) {
				defer func() {
					r := recover()
					if r == nil {
						t.Fatalf("%s on nullish did not throw", tc.name)
					}
					if e, ok := r.(*Error); !ok || !e.IsA("TypeError") {
						t.Fatalf("thrown value %v, want a TypeError", r)
					}
				}()
				tc.call(recv)
			})
		}
	}
}

// A delete off a non-nullish primitive receiver still reports true: the primitive
// coerces to a wrapper object with no own slot, so the removal is a no-op, not a
// throw.
func TestDeleteOnPrimitiveReportsTrue(t *testing.T) {
	if !StringValue(FromGoString("abc")).Delete(FromGoString("foo")) {
		t.Fatalf("Delete on a string primitive reported false, want true")
	}
	if !Number(3).Delete(FromGoString("foo")) {
		t.Fatalf("Delete on a number primitive reported false, want true")
	}
}
