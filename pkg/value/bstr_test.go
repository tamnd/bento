package value

import (
	"math"
	"testing"
)

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

// TestCharCodeAt pins String.prototype.charCodeAt: in-range indices return the
// UTF-16 code unit, an out-of-range or negative index returns NaN, a fractional
// index truncates, and an astral character reads back as its two surrogate
// halves rather than one code point.
func TestCharCodeAt(t *testing.T) {
	s := FromGoString("aπ😀") // 'a' (1 unit), 'π' (1 unit), emoji (2 units) = 4 units
	cases := []struct {
		name string
		idx  float64
		want float64
	}{
		{"ascii", 0, 0x61},
		{"bmp", 1, 0x3C0},
		{"highSurrogate", 2, 0xD83D},
		{"lowSurrogate", 3, 0xDE00},
		{"fractionTruncates", 0.9, 0x61},
		{"nanIsZero", math.NaN(), 0x61},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s.CharCodeAt(tc.idx); got != tc.want {
				t.Errorf("CharCodeAt(%v) = %v, want %v", tc.idx, got, tc.want)
			}
		})
	}
	for _, idx := range []float64{-1, 4, 100} {
		if got := s.CharCodeAt(idx); !math.IsNaN(got) {
			t.Errorf("CharCodeAt(%v) = %v, want NaN", idx, got)
		}
	}
}

// TestCharAt pins String.prototype.charAt: an in-range index returns the
// one-code-unit string, an out-of-range or negative index returns the empty
// string, and charAt of an astral character returns a lone surrogate half, a
// one-unit string that is not valid UTF-8 but is exactly what JavaScript
// returns. The surrogate case is checked by code unit, not by a Go string
// comparison, because the whole point is that it does not round-trip through
// UTF-8.
func TestCharAt(t *testing.T) {
	s := FromGoString("a😀") // 'a' (1 unit), emoji (2 units, a surrogate pair)

	if got := s.CharAt(0); got.ToGoString() != "a" {
		t.Errorf("CharAt(0) = %q, want \"a\"", got.ToGoString())
	}

	high := s.CharAt(1)
	if high.Length() != 1 {
		t.Errorf("CharAt(1) Length = %v, want 1", high.Length())
	}
	if u := high.units(); len(u) != 1 || u[0] != 0xD83D {
		t.Errorf("CharAt(1) units = %v, want [0xD83D] (high surrogate)", u)
	}
	low := s.CharAt(2)
	if u := low.units(); len(u) != 1 || u[0] != 0xDE00 {
		t.Errorf("CharAt(2) units = %v, want [0xDE00] (low surrogate)", u)
	}
	// The two halves rejoin to the original emoji, so the split is exact.
	if rejoined := Concat(high, low); rejoined.ToGoString() != "😀" {
		t.Errorf("rejoined halves = %q, want the emoji", rejoined.ToGoString())
	}

	for _, idx := range []float64{-1, 3, 100} {
		if got := s.CharAt(idx); got.Length() != 0 {
			t.Errorf("CharAt(%v) = %q, want empty string", idx, got.ToGoString())
		}
	}
}

// TestSearchMethods pins IndexOf, Includes, and StartsWith, including the
// code-unit index of a match after an astral character (the emoji is two units,
// so "b" in "a😀b" is at index 3, not 2) and the empty-search rule that matches
// at 0.
func TestSearchMethods(t *testing.T) {
	s := FromGoString("a😀b")
	if got := s.IndexOf(FromGoString("b")); got != 3 {
		t.Errorf("IndexOf(\"b\") = %v, want 3 (code units, past the surrogate pair)", got)
	}
	if got := s.IndexOf(FromGoString("a")); got != 0 {
		t.Errorf("IndexOf(\"a\") = %v, want 0", got)
	}
	if got := s.IndexOf(FromGoString("z")); got != -1 {
		t.Errorf("IndexOf(\"z\") = %v, want -1", got)
	}
	if got := s.IndexOf(FromGoString("")); got != 0 {
		t.Errorf("IndexOf(\"\") = %v, want 0", got)
	}
	if !s.Includes(FromGoString("😀")) {
		t.Error("Includes(emoji) = false, want true")
	}
	if s.Includes(FromGoString("z")) {
		t.Error("Includes(\"z\") = true, want false")
	}
	if !s.StartsWith(FromGoString("a😀")) {
		t.Error("StartsWith(\"a😀\") = false, want true")
	}
	if s.StartsWith(FromGoString("😀")) {
		t.Error("StartsWith(\"😀\") = true, want false")
	}
	if !s.StartsWith(FromGoString("")) {
		t.Error("StartsWith(\"\") = false, want true")
	}
	// A prefix longer than the string is not a prefix.
	if FromGoString("hi").StartsWith(FromGoString("hello")) {
		t.Error("StartsWith with a longer prefix = true, want false")
	}
}

// TestSlice pins String.prototype.slice: interior ranges, defaulted arguments
// through the variadic arity, negative-from-end bounds, an empty result when
// start reaches end, and a slice that lands between the halves of an astral
// character and returns a lone surrogate.
func TestSlice(t *testing.T) {
	h := FromGoString("hello")
	cases := []struct {
		name string
		args []float64
		want string
	}{
		{"interior", []float64{1, 3}, "el"},
		{"noArgsWholeString", nil, "hello"},
		{"oneArgTail", []float64{2}, "llo"},
		{"negativeFromEnd", []float64{-3, -1}, "ll"},
		{"startPastEndEmpty", []float64{3, 1}, ""},
		{"endPastLengthClamps", []float64{2, 100}, "llo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.Slice(tc.args...).ToGoString(); got != tc.want {
				t.Errorf("Slice(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}

	// "a😀b" has units a(0) high(1) low(2) b(3). Slicing [1,2) is the lone high
	// surrogate; [1,3) is the whole emoji.
	astral := FromGoString("a😀b")
	half := astral.Slice(1, 2)
	if u := half.units(); len(u) != 1 || u[0] != 0xD83D {
		t.Errorf("Slice(1,2) units = %v, want [0xD83D]", u)
	}
	if got := astral.Slice(1, 3).ToGoString(); got != "😀" {
		t.Errorf("Slice(1,3) = %q, want the emoji", got)
	}
}

// TestSubstring pins String.prototype.substring, whose edges differ from slice:
// a negative or NaN argument becomes 0, and a start past end swaps rather than
// yielding the empty string.
func TestSubstring(t *testing.T) {
	h := FromGoString("hello")
	cases := []struct {
		name string
		args []float64
		want string
	}{
		{"interior", []float64{1, 3}, "el"},
		{"swappedWhenStartAfterEnd", []float64{3, 1}, "el"},
		{"negativeBecomesZero", []float64{-2, 3}, "hel"},
		{"endPastLengthClamps", []float64{2, 100}, "llo"},
		{"oneArgToEnd", []float64{2}, "llo"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.Substring(tc.args...).ToGoString(); got != tc.want {
				t.Errorf("Substring(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}
