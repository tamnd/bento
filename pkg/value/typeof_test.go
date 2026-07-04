package value

import "testing"

// TestTypeOf pins the JavaScript typeof tag each boxed kind reports, including the
// two that trip people up: null answers "object", and an array answers "object"
// like any other object rather than a kind of its own. Only a callable answers
// "function".
func TestTypeOf(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"undefined", Undefined, "undefined"},
		{"null", Null, "object"},
		{"true", True, "boolean"},
		{"false", False, "boolean"},
		{"number", Number(1.5), "number"},
		{"bigint", BigIntFromInt64(7), "bigint"},
		{"string", StringValue(FromGoString("hi")), "string"},
		{"symbol", Value{kind: KindSymbol}, "symbol"},
		{"object", NewObject(), "object"},
		{"array", NewArrayValue(nil), "object"},
		{"function", Value{kind: KindFunc}, "function"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.v.TypeOf().ToGoString(); got != c.want {
				t.Errorf("TypeOf() = %q, want %q", got, c.want)
			}
		})
	}
}
