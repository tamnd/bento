package value

import "testing"

// TestClassTag pins Object.prototype.toString.call over each kind: the tag reads
// the value's internal class as "[object Type]", the spelling the spec's
// Object.prototype.toString produces and test262 leans on. Symbol and function
// values have no plain constructor here, so their tags are covered by the kinds
// this reaches; the switch in ClassTag names them all the same way.
func TestClassTag(t *testing.T) {
	cases := []struct {
		name string
		v    Value
		want string
	}{
		{"undefined", Undefined, "[object Undefined]"},
		{"null", Null, "[object Null]"},
		{"true", True, "[object Boolean]"},
		{"false", False, "[object Boolean]"},
		{"number", Number(42), "[object Number]"},
		{"nan", Number(nan()), "[object Number]"},
		{"bigint", BigIntFromInt64(7), "[object BigInt]"},
		{"string", StringValue(FromGoString("s")), "[object String]"},
		{"object", NewObject(), "[object Object]"},
		{"array", NewArrayValue([]Value{Number(1), Number(2)}), "[object Array]"},
		{"empty array", NewArrayValue(nil), "[object Array]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassTag(tc.v).ToGoString()
			if got != tc.want {
				t.Errorf("ClassTag(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
