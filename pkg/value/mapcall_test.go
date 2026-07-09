package value

import (
	"math"
	"testing"
)

// TestMapCallString pins Array.prototype.map.call(arrayLike, String) as the
// runtime helper implements it: every element is coerced through the abstract
// ToString the String built-in applies, so undefined and null spell their names
// rather than the empty string join alone would give them, and the result is a
// dense array the assert prelude then joins. The cases render through Join the same
// way the lowered compareArray.format does, so the test exercises the exact pair.
func TestMapCallString(t *testing.T) {
	sep := FromGoString(", ")
	cases := []struct {
		name string
		in   Value
		want string
	}{
		{"numbers", NewArrayValue([]Value{Number(1), Number(2), Number(3)}), "1, 2, 3"},
		{"empty", NewArrayValue(nil), ""},
		{"nullish spell their names", NewArrayValue([]Value{Number(1), Null, Undefined, Bool(true)}), "1, null, undefined, true"},
		{"special numbers", NewArrayValue([]Value{Number(math.Copysign(0, -1)), Number(math.NaN()), Number(math.Inf(1))}), "0, NaN, Infinity"},
		{"string receiver indexes to chars", StringValue(FromGoString("abc")), "a, b, c"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := MapCallString(c.in).Join(sep, JoinString).ToGoString()
			if got != c.want {
				t.Fatalf("MapCallString(%s) joined = %q, want %q", c.name, got, c.want)
			}
		})
	}
}
