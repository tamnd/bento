package value

import (
	"strings"
	"testing"
)

// A strict-mode failed write throws a TypeError where the sloppy Set silently drops
// it. SetStrict throws on two of the failure modes the spec names: a non-writable data
// property and a new key on a non-extensible object (the getter-only accessor case
// needs an accessor descriptor, exercised by the corpus).
func TestSetStrictThrowsOnFailedWrite(t *testing.T) {
	for _, tc := range []struct {
		name  string
		build func() Value
		key   string
		want  string
	}{
		{
			name: "non-writable data property",
			build: func() Value {
				o := NewObject()
				obj := o.object()
				obj.keys = append(obj.keys, FromGoString("x"))
				d := defaultDataProperty(Number(1))
				d.writable = false
				obj.descs = append(obj.descs, d)
				return o
			},
			key:  "x",
			want: "Cannot assign to read only property 'x'",
		},
		{
			name: "new key on non-extensible object",
			build: func() Value {
				o := NewObject()
				o.object().nonExtensible = true
				return o
			},
			key:  "y",
			want: "Cannot add property y, object is not extensible",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := tc.build()
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("SetStrict did not throw for %s", tc.name)
				}
				e, ok := r.(*Error)
				if !ok {
					t.Fatalf("thrown value is %T, want *Error", r)
				}
				if !e.IsA("TypeError") {
					t.Fatalf("thrown error is %s, want TypeError", e.name.ToGoString())
				}
				if got := e.message.ToGoString(); !strings.Contains(got, tc.want) {
					t.Fatalf("message = %q, want it to contain %q", got, tc.want)
				}
			}()
			o.SetStrict(FromGoString(tc.key), Number(9))
		})
	}
}

// A normal writable property still writes under SetStrict, so strict mode does not
// break an ordinary assignment.
func TestSetStrictWritesNormalProperty(t *testing.T) {
	o := NewObject()
	o.SetStrict(FromGoString("a"), Number(1))
	o.SetStrict(FromGoString("a"), Number(2))
	if got := ToString(o.Get(FromGoString("a"))).ToGoString(); got != "2" {
		t.Fatalf("SetStrict left property = %q, want %q", got, "2")
	}
}

// A strict write to a nullish base throws the same TypeError the sloppy Set does, so
// the guard is not lost on the strict path.
func TestSetStrictOnNullishThrows(t *testing.T) {
	for _, recv := range []Value{Null, Undefined} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Fatalf("SetStrict on nullish did not throw")
				}
				if e, ok := r.(*Error); !ok || !e.IsA("TypeError") {
					t.Fatalf("thrown value %v, want a TypeError", r)
				}
			}()
			recv.SetStrict(FromGoString("p"), Number(1))
		}()
	}
}
