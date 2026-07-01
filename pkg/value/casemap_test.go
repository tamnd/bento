package value

import "testing"

// TestToUpperCase pins String.prototype.toUpperCase over the cases where the full
// mapping differs from Go's simple ToUpper: the sharp s expands to two letters, a
// ligature expands, and a whole word carries the expansion, plus the ordinary
// ASCII path.
func TestToUpperCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"abc", "ABC"},
		{"ß", "SS"},
		{"straße", "STRASSE"},
		{"ﬀ", "FF"},
		{"aπ😀", "AΠ😀"}, // the emoji has no case and passes through
	}
	for _, c := range cases {
		if got := FromGoString(c.in).ToUpperCase().ToGoString(); got != c.want {
			t.Errorf("ToUpperCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestToLowerCase pins String.prototype.toLowerCase, including the Final_Sigma
// context: a capital sigma at the end of a word lowercases to the final form ς,
// while one followed by another cased letter lowercases to σ.
func TestToLowerCase(t *testing.T) {
	cases := []struct{ in, want string }{
		{"ABC", "abc"},
		{"Σ", "σ"},
		{"ΟΔΟΣ", "οδος"}, // trailing sigma is final
		{"ΣΣ", "σς"},     // first sigma non-final, second final
		{"İ", "i̇"},      // dotted capital I lowercases to i + combining dot
	}
	for _, c := range cases {
		if got := FromGoString(c.in).ToLowerCase().ToGoString(); got != c.want {
			t.Errorf("ToLowerCase(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestCaseMapLoneSurrogate checks a lone surrogate survives case mapping, the
// reason the code-unit path processes valid runs and passes surrogates through
// rather than transcoding the whole string to UTF-8 first, which would replace the
// surrogate with U+FFFD.
func TestCaseMapLoneSurrogate(t *testing.T) {
	// "a" + lone high surrogate + "b" uppercases the letters and keeps the surrogate.
	s := FromUTF16([]uint16{0x61, 0xD83D, 0x62})
	got := s.ToUpperCase()
	want := []uint16{0x41, 0xD83D, 0x42}
	units := got.units()
	if len(units) != len(want) {
		t.Fatalf("ToUpperCase units = %v, want %v", units, want)
	}
	for i := range want {
		if units[i] != want[i] {
			t.Fatalf("ToUpperCase units = %v, want %v", units, want)
		}
	}
}
