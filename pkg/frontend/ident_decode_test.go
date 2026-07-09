package frontend

import "testing"

// TestDecodeIdentEscapes pins that both identifier escape forms decode to the
// name they denote and that an escape-free or malformed spelling passes
// through, so a declaration written with escapes and a reference written plain
// mangle to one Go name.
func TestDecodeIdentEscapes(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"abc", "abc"},               // no escape, untouched
		{"\\u0061bc", "abc"},         // leading \uHHHH decodes to 'a'
		{"a\\u0062c", "abc"},         // interior \uHHHH decodes to 'b'
		{"\\u{61}bc", "abc"},         // braced form, short body
		{"\\u{1F600}", "\U0001F600"}, // braced form past the BMP
		{"\\u0042igInt", "BigInt"},   // decodes to a name lowering matches on
		{"pi\\u00f1ata", "piñata"},   // non-ASCII letter
		{"a\\x62c", "a\\x62c"},       // not a \u escape, kept verbatim
		{"a\\u{}", "a\\u{}"},         // empty braces, malformed, kept verbatim
		{"a\\u12", "a\\u12"},         // short \u body, malformed, kept verbatim
	}
	for _, c := range cases {
		if got := decodeIdentEscapes(c.in); got != c.want {
			t.Errorf("decodeIdentEscapes(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
