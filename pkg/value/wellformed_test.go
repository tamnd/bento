package value

import "testing"

// hi and lo build a lone surrogate string from raw code units, the only way a BStr
// comes to hold one, since FromGoString transcodes valid UTF-8 that cannot encode a
// surrogate.
func surrogates(units ...uint16) BStr {
	return FromUTF16(units)
}

// TestIsWellFormed covers the pairing rules: a plain string and a valid surrogate
// pair are well-formed, a lone high or low surrogate is not, and a UTF-8 fast-path
// string is well-formed without inspecting its units.
func TestIsWellFormed(t *testing.T) {
	cases := []struct {
		name string
		s    BStr
		want bool
	}{
		{"ascii", FromGoString("hello"), true},
		{"astral utf8", FromGoString("a\U0001F600b"), true},
		{"empty", FromGoString(""), true},
		{"valid pair", surrogates(0xD83D, 0xDE00), true},
		{"lone high", surrogates(0x0061, 0xD83D), false},
		{"lone low", surrogates(0xDE00, 0x0062), false},
		{"high then bmp", surrogates(0xD83D, 0x0061), false},
		{"two highs", surrogates(0xD83D, 0xD83D), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.s.IsWellFormed(); got != c.want {
				t.Errorf("IsWellFormed() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestToWellFormed covers that a well-formed string is returned as is and a lone
// surrogate is replaced by U+FFFD while a valid pair is left intact.
func TestToWellFormed(t *testing.T) {
	// A lone high surrogate between two ASCII letters becomes the replacement.
	got := surrogates(0x0061, 0xD83D, 0x0062).ToWellFormed()
	want := surrogates(0x0061, 0xFFFD, 0x0062)
	if !got.Equal(want) {
		t.Errorf("ToWellFormed lone high = %q, want the replacement character", got.ToGoString())
	}
	// A valid pair is preserved, so the round trip is well-formed and unchanged.
	pair := surrogates(0xD83D, 0xDE00)
	if fixed := pair.ToWellFormed(); !fixed.Equal(pair) {
		t.Errorf("ToWellFormed changed a valid pair: %q", fixed.ToGoString())
	}
	if !pair.ToWellFormed().IsWellFormed() {
		t.Error("ToWellFormed output is not well-formed")
	}
	// A UTF-8 string comes back unchanged.
	plain := FromGoString("ok")
	if fixed := plain.ToWellFormed(); !fixed.Equal(plain) {
		t.Errorf("ToWellFormed changed a plain string: %q", fixed.ToGoString())
	}
}
