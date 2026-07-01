package value

import "testing"

// TestLengthCodeUnits pins that Length reports UTF-16 code units, not bytes or
// runes: an astral character is one rune but two code units, exactly what
// String.prototype.length reports in JavaScript.
func TestLengthCodeUnits(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want float64
	}{
		{"empty", "", 0},
		{"ascii", "abc", 3},
		{"latin1", "café", 4},         // é is one BMP code unit
		{"bmp", "λόγος", 5},           // Greek, all BMP
		{"astral", "a\U0001F600b", 4}, // emoji is a surrogate pair, two units
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromGoString(tc.in).Length(); got != tc.want {
				t.Errorf("Length(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

// TestConcatUTF8FastPath checks that concatenating two fast-path strings keeps
// the UTF-8 backing and the code-unit length adds up, including across an astral
// boundary where a naive byte length would be wrong.
func TestConcatUTF8FastPath(t *testing.T) {
	a := FromGoString("a\U0001F600") // 3 code units (a + surrogate pair)
	b := FromGoString("bc")          // 2 code units
	got := Concat(a, b)
	if got.Length() != 5 {
		t.Errorf("Length after concat = %v, want 5", got.Length())
	}
	if got.ToGoString() != "a\U0001F600bc" {
		t.Errorf("ToGoString after concat = %q", got.ToGoString())
	}
	if got.utf16 != nil {
		t.Error("two fast-path strings should concat on the fast path, utf16 should stay nil")
	}
}

// TestLoneSurrogateSurvives is the reason BStr exists: a lone surrogate is a
// legal JavaScript string that UTF-8 cannot represent, so it must round-trip
// through the code-unit view and count as one code unit.
func TestLoneSurrogateSurvives(t *testing.T) {
	lone := FromUTF16([]uint16{0xD83D}) // high surrogate with no low half
	if lone.Length() != 1 {
		t.Errorf("lone surrogate Length = %v, want 1", lone.Length())
	}
	// Concatenating a lone high surrogate with a lone low surrogate must keep two
	// distinct code units, not silently combine or drop them.
	pair := Concat(FromUTF16([]uint16{0xD83D}), FromUTF16([]uint16{0xDE00}))
	if pair.Length() != 2 {
		t.Errorf("surrogate halves Length = %v, want 2", pair.Length())
	}
	// Read back as UTF-16 the two halves are intact; as a valid pair they also
	// decode to the emoji, which is the whole point of preserving them.
	if pair.ToGoString() != "\U0001F600" {
		t.Errorf("rejoined surrogates ToGoString = %q, want the emoji", pair.ToGoString())
	}
}

// TestEqualCodeUnitWise checks equality is by code unit and is independent of
// how each side is backed, so a fast-path string and a code-unit-backed string
// with the same content compare equal.
func TestEqualCodeUnitWise(t *testing.T) {
	fast := FromGoString("A")
	units := FromUTF16([]uint16{0x0041}) // 'A'
	if !fast.Equal(units) {
		t.Error("same content backed differently should be Equal")
	}
	if fast.Equal(FromGoString("B")) {
		t.Error("different content should not be Equal")
	}
	if FromGoString("ab").Equal(FromGoString("a")) {
		t.Error("different length should not be Equal")
	}
}

// TestConcatMixedBacking checks a fast-path side and a surrogate-backed side
// concatenate into a code-unit result that preserves both, the path Concat takes
// when it cannot stay on the fast path.
func TestConcatMixedBacking(t *testing.T) {
	got := Concat(FromGoString("x"), FromUTF16([]uint16{0xD83D}))
	if got.Length() != 2 {
		t.Errorf("mixed concat Length = %v, want 2", got.Length())
	}
	if got.utf16 == nil {
		t.Error("a surrogate-backed side should force the code-unit result")
	}
}
