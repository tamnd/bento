package value

import "testing"

// TestInspect pins the compact spelling Inspect shares with the interpreter's
// prelude inspector: a top-level string is unquoted, a string nested in a
// container is quoted, an object reads its keys unquoted with quoted string
// values, an array brackets its elements, and the empty container forms are []
// and {}.
func TestInspect(t *testing.T) {
	obj := NewObject().
		Set(FromGoString("a"), Number(1)).
		Set(FromGoString("b"), StringValue(FromGoString("hi"))).
		Set(FromGoString("c"), NewArrayValue([]Value{Number(1), Number(2)}))
	cases := []struct {
		name string
		in   Value
		want string
	}{
		{"top string unquoted", StringValue(FromGoString("hi")), "hi"},
		{"number", Number(42), "42"},
		{"bool", True, "true"},
		{"null", Null, "null"},
		{"undefined", Undefined, "undefined"},
		{"empty array", NewArrayValue(nil), "[]"},
		{"empty object", NewObject(), "{}"},
		{"nested string quoted", NewArrayValue([]Value{StringValue(FromGoString("x"))}), `[ "x" ]`},
		{"object with nested array", obj, `{ a: 1, b: "hi", c: [ 1, 2 ] }`},
	}
	for _, c := range cases {
		if got := Inspect(c.in).ToGoString(); got != c.want {
			t.Errorf("%s: Inspect = %q, want %q", c.name, got, c.want)
		}
	}
}

// TestInspectCircular pins that a value that refers to itself renders "[Circular]"
// at the point the cycle closes rather than recurring forever.
func TestInspectCircular(t *testing.T) {
	o := NewObject()
	o.SetKey(FromGoString("self"), o)
	got := Inspect(o).ToGoString()
	if got != "{ self: [Circular] }" {
		t.Fatalf("Inspect cyclic = %q, want %q", got, "{ self: [Circular] }")
	}
}
