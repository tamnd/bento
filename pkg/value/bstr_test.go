package value

import (
	"math"
	"strings"
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

// TestConcatRopeAccumulation is the reason the rope exists: a string built by
// repeatedly concatenating onto a growing result must read back correct and must
// not pay the O(n squared) copy a fold of eager Concat would. Each step here
// crosses the eager threshold, so every Concat after the first builds a rope
// node, and the final read flattens once.
func TestConcatRopeAccumulation(t *testing.T) {
	piece := FromGoString("0123456789abcdef") // 16 units, so four pieces cross 64
	var want string
	got := FromGoString("")
	for i := 0; i < 100; i++ {
		got = Concat(got, piece)
		want += "0123456789abcdef"
	}
	if int(got.Length()) != len(want) {
		t.Errorf("rope Length = %v, want %d", got.Length(), len(want))
	}
	if got.ToGoString() != want {
		t.Errorf("rope ToGoString mismatch: got %d bytes, want %d", len(got.ToGoString()), len(want))
	}
	// A second read hits the cached flat, so it must still be identical.
	if got.ToGoString() != want {
		t.Error("second read of a flattened rope differs from the first")
	}
}

// TestConcatRopeLengthBeforeFlatten checks that .length on an unflattened rope is
// correct without forcing materialization, since the rope keeps the total code
// unit count on the node.
func TestConcatRopeLengthBeforeFlatten(t *testing.T) {
	long := FromGoString(strings.Repeat("x", 40))
	r := Concat(long, long) // 80 units, over the threshold, so a rope
	if r.rope == nil {
		t.Fatal("a large concat should build a rope node")
	}
	if r.rope.flat != nil {
		t.Error("Length must not flatten the rope")
	}
	if r.Length() != 80 {
		t.Errorf("rope Length = %v, want 80", r.Length())
	}
	if r.rope.flat != nil {
		t.Error("reading Length flattened the rope")
	}
}

// TestConcatRopeMixedBacking checks a rope whose leaves mix a UTF-8 leaf and a
// surrogate-backed leaf flattens to the code-unit view and preserves the lone
// surrogate, the same guarantee eager Concat gives.
func TestConcatRopeMixedBacking(t *testing.T) {
	filler := FromGoString(strings.Repeat("y", 50))
	surrogate := FromUTF16([]uint16{0xD83D})
	got := Concat(Concat(filler, surrogate), filler) // 101 units, a rope with a surrogate leaf
	if int(got.Length()) != 101 {
		t.Errorf("mixed rope Length = %v, want 101", got.Length())
	}
	units := got.units()
	if units[50] != 0xD83D {
		t.Errorf("lone surrogate at index 50 = %#x, want 0xD83D", units[50])
	}
}

// TestCompare pins the code-unit ordering behind the string relational
// operators: a shorter prefix orders first, equal strings compare zero, ASCII
// orders by byte, and the astral case is the one where a code-unit compare
// diverges from a code-point compare. The emoji U+1F600 is stored as the
// surrogate pair D83D DE00, whose first unit D83D is below the BMP character
// U+E000, so "" orders after the emoji by code unit even though its code
// point is lower.
func TestCompare(t *testing.T) {
	cases := []struct {
		a, b BStr
		want int
	}{
		{FromGoString("a"), FromGoString("b"), -1},
		{FromGoString("b"), FromGoString("a"), 1},
		{FromGoString("abc"), FromGoString("abc"), 0},
		{FromGoString("ab"), FromGoString("abc"), -1},
		{FromGoString("abc"), FromGoString("ab"), 1},
		{FromGoString(""), FromGoString("a"), -1},
		{FromGoString("Z"), FromGoString("a"), -1}, // uppercase orders below lowercase
		{FromGoString("😀"), FromGoString(""), -1},
		{FromGoString(""), FromGoString("😀"), 1},
	}
	for _, c := range cases {
		if got := c.a.Compare(c.b); got != c.want {
			t.Errorf("%q.Compare(%q) = %d, want %d", c.a.ToGoString(), c.b.ToGoString(), got, c.want)
		}
	}
}

// TestConcatN pins String.prototype.concat over its argument arities: no
// arguments returns the receiver, several arguments join in order, and a
// surrogate-backed argument still forces the code-unit result the way a plain
// Concat does.
func TestConcatN(t *testing.T) {
	base := FromGoString("a")
	if got := base.ConcatN(); got.ToGoString() != "a" || got.Length() != 1 {
		t.Errorf("ConcatN() = %q len %v, want \"a\" len 1", got.ToGoString(), got.Length())
	}
	if got := base.ConcatN(FromGoString("b"), FromGoString("c"), FromGoString("d")); got.ToGoString() != "abcd" {
		t.Errorf("ConcatN(b, c, d) = %q, want \"abcd\"", got.ToGoString())
	}
	// A surrogate-backed argument keeps the lone surrogate and moves the whole
	// result off the fast path.
	got := base.ConcatN(FromUTF16([]uint16{0xD83D}))
	if got.Length() != 2 {
		t.Errorf("ConcatN with a surrogate arg Length = %v, want 2", got.Length())
	}
	if got.utf16 == nil {
		t.Error("a surrogate-backed argument should force the code-unit result")
	}
	// A rope-backed receiver stays off the single-pass builder path but still
	// joins its arguments in order through the pairwise fallback.
	rope := Concat(FromGoString("x"), FromGoString("y"))
	if got := rope.ConcatN(FromGoString("z"), FromGoString("w")); got.ToGoString() != "xyzw" {
		t.Errorf("ConcatN over a rope receiver = %q, want \"xyzw\"", got.ToGoString())
	}
	// A longer all-ASCII join runs the single-pass builder and preserves order and
	// length across every piece.
	many := FromGoString("0").ConcatN(
		FromGoString("1"), FromGoString("2"), FromGoString("3"),
		FromGoString("4"), FromGoString("5"), FromGoString("6"),
	)
	if many.ToGoString() != "0123456" || many.Length() != 7 {
		t.Errorf("ConcatN seven ASCII pieces = %q len %v, want \"0123456\" len 7", many.ToGoString(), many.Length())
	}
}

// TestFromCharCode pins String.fromCharCode: no arguments gives the empty
// string, ASCII code units spell out their characters, each argument wraps
// through ToUint16 so a number past 2^16 keeps only the low 16 bits, a fraction
// truncates, and a bare surrogate half is preserved as a lone surrogate rather
// than replaced.
func TestFromCharCode(t *testing.T) {
	if got := FromCharCode(); got.ToGoString() != "" || got.Length() != 0 {
		t.Errorf("FromCharCode() = %q len %v, want empty", got.ToGoString(), got.Length())
	}
	if got := FromCharCode(72, 105); got.ToGoString() != "Hi" {
		t.Errorf("FromCharCode(72, 105) = %q, want \"Hi\"", got.ToGoString())
	}
	// 65536 + 65 wraps to 65, so it reads back as 'A', and 65.9 truncates to 65.
	if got := FromCharCode(65536+65, 65.9); got.ToGoString() != "AA" {
		t.Errorf("FromCharCode(65601, 65.9) = %q, want \"AA\"", got.ToGoString())
	}
	// A surrogate pair supplied as two code units rejoins into one astral rune.
	if got := FromCharCode(0xD83D, 0xDE00); got.ToGoString() != "\U0001F600" {
		t.Errorf("FromCharCode(0xD83D, 0xDE00) = %q, want the emoji", got.ToGoString())
	}
	// A lone high surrogate survives as a single code unit off the fast path.
	lone := FromCharCode(0xD83D)
	if lone.Length() != 1 || lone.utf16 == nil {
		t.Errorf("FromCharCode(0xD83D) len %v utf16 nil %v, want 1 and a code-unit view", lone.Length(), lone.utf16 == nil)
	}
}

// TestFromCodePoint pins String.fromCodePoint: a BMP point is one code unit, an
// astral point above U+FFFF splits into the surrogate pair that spells it, a mix
// concatenates in order, and the empty call is the empty string.
func TestFromCodePoint(t *testing.T) {
	if got := FromCodePoint(); got.ToGoString() != "" || got.Length() != 0 {
		t.Errorf("FromCodePoint() = %q len %v, want empty", got.ToGoString(), got.Length())
	}
	if got := FromCodePoint(72, 105); got.ToGoString() != "Hi" {
		t.Errorf("FromCodePoint(72, 105) = %q, want \"Hi\"", got.ToGoString())
	}
	// An astral code point becomes a surrogate pair, so it occupies two code units
	// but reads back as the single rune.
	astral := FromCodePoint(0x1F600)
	if astral.ToGoString() != "\U0001F600" || astral.Length() != 2 {
		t.Errorf("FromCodePoint(0x1F600) = %q len %v, want the emoji and length 2", astral.ToGoString(), astral.Length())
	}
	if got := FromCodePoint(65, 0x1F4A9, 66); got.ToGoString() != "A\U0001F4A9B" {
		t.Errorf("FromCodePoint(65, 0x1F4A9, 66) = %q, want A<pile>B", got.ToGoString())
	}
}

// TestCodePoints pins the for...of iteration order of a string: one element per
// Unicode code point, so a BMP character is its own one-unit element, an astral
// character is a single two-unit element, and a lone surrogate stands alone. This
// is what makes `for (const c of s)` step by code point rather than by code unit.
func TestCodePoints(t *testing.T) {
	got := FromGoString("abc").CodePoints()
	if len(got) != 3 || got[0].ToGoString() != "a" || got[2].ToGoString() != "c" {
		t.Errorf("CodePoints(\"abc\") = %v, want [a b c]", goStrings(got))
	}

	// "a😀b" is four code units (the emoji is a surrogate pair) but three code
	// points, so it iterates as three elements and the middle one is the emoji.
	astral := FromGoString("a\U0001F600b").CodePoints()
	if len(astral) != 3 {
		t.Fatalf("CodePoints(\"a<emoji>b\") length = %d, want 3", len(astral))
	}
	if astral[0].ToGoString() != "a" || astral[1].ToGoString() != "\U0001F600" || astral[2].ToGoString() != "b" {
		t.Errorf("CodePoints(\"a<emoji>b\") = %v, want [a <emoji> b]", goStrings(astral))
	}
	if astral[1].Length() != 2 {
		t.Errorf("astral element length = %v, want 2 (a surrogate pair)", astral[1].Length())
	}

	if got := FromGoString("").CodePoints(); len(got) != 0 {
		t.Errorf("CodePoints(\"\") = %v, want empty", goStrings(got))
	}

	// A lone high surrogate has no low half to pair with, so it iterates as one
	// one-unit element rather than being dropped or combined.
	lone := FromUTF16([]uint16{0xD83D, 0x0041}).CodePoints()
	if len(lone) != 2 || lone[1].ToGoString() != "A" {
		t.Errorf("CodePoints(lone high surrogate + A) = %v, want two elements ending in A", goStrings(lone))
	}
}

// goStrings renders a slice of BStr as Go strings for a readable test failure.
func goStrings(ss []BStr) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ToGoString()
	}
	return out
}

// TestFromCodePointThrows pins that an out-of-range code point throws a
// RangeError the way JavaScript does, rather than wrapping the way fromCharCode
// does: a negative, a fraction, and a value past U+10FFFF are all rejected.
func TestFromCodePointThrows(t *testing.T) {
	for _, c := range []float64{-1, 1.5, 0x110000} {
		e := codePointThrown(t, c)
		if e.Name().ToGoString() != "RangeError" {
			t.Errorf("FromCodePoint(%v) threw %s, want RangeError", c, e.Name().ToGoString())
		}
	}
}

func codePointThrown(t *testing.T, c float64) *Error {
	t.Helper()
	var caught *Error
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			e, ok := r.(*Error)
			if !ok {
				t.Fatalf("FromCodePoint(%v) threw %T, want *Error", c, r)
			}
			caught = e
		}()
		FromCodePoint(c)
	}()
	if caught == nil {
		t.Fatalf("FromCodePoint(%v) did not throw", c)
	}
	return caught
}

// TestReplace pins String.prototype.replace with two strings: only the first
// occurrence is replaced, a missing search returns the receiver, an empty search
// inserts at the front, and the substitution patterns $$, $&, $` and $' expand
// while a $ before a digit stays literal because a string search has no captures.
func TestReplace(t *testing.T) {
	cases := []struct {
		hay, search, repl, want string
	}{
		{"abcabc", "bc", "X", "aXabc"},           // only the first match
		{"hello", "xyz", "X", "hello"},           // no match returns the receiver
		{"abc", "", "-", "-abc"},                 // empty search inserts at the front
		{"a.b", ".", "[$&]", "a[.]b"},            // $& is the matched text
		{"one two", "two", "[$`]", "one [one ]"}, // $` is the text before
		{"one two", "one", "[$']", "[ two] two"}, // $' is the text after
		{"cost", "cost", "$$5", "$5"},            // $$ is a literal dollar
		{"ab", "a", "$1", "$1b"},                 // $1 has no capture, stays literal
	}
	for _, c := range cases {
		got := FromGoString(c.hay).Replace(FromGoString(c.search), FromGoString(c.repl)).ToGoString()
		if got != c.want {
			t.Errorf("%q.Replace(%q, %q) = %q, want %q", c.hay, c.search, c.repl, got, c.want)
		}
	}
}

// TestReplaceAll pins String.prototype.replaceAll with two strings: every
// non-overlapping occurrence is replaced, a missing search returns the receiver,
// an empty search weaves the replacement between every code unit and at both
// ends, and the substitution patterns expand for each match.
func TestReplaceAll(t *testing.T) {
	cases := []struct {
		hay, search, repl, want string
	}{
		{"abcabc", "bc", "X", "aXaX"},       // every match
		{"aaa", "a", "aa", "aaaaaa"},        // replacement is not rescanned
		{"hello", "xyz", "X", "hello"},      // no match returns the receiver
		{"abc", "", "-", "-a-b-c-"},         // empty search weaves at every gap
		{"a.b.c", ".", "[$&]", "a[.]b[.]c"}, // $& expands per match
		{"", "", "X", "X"},                  // empty over empty is one insertion
	}
	for _, c := range cases {
		got := FromGoString(c.hay).ReplaceAll(FromGoString(c.search), FromGoString(c.repl)).ToGoString()
		if got != c.want {
			t.Errorf("%q.ReplaceAll(%q, %q) = %q, want %q", c.hay, c.search, c.repl, got, c.want)
		}
	}
}

// TestReplaceCodeUnitWise pins that replace works on the UTF-16 code-unit view,
// so a lone surrogate can be the search and the replacement, matching how
// JavaScript treats an unpaired surrogate as an ordinary code unit.
func TestReplaceCodeUnitWise(t *testing.T) {
	hay := Concat(FromUTF16([]uint16{0xD83D}), FromGoString("x"))
	got := hay.Replace(FromUTF16([]uint16{0xD83D}), FromGoString("Y"))
	if got.ToGoString() != "Yx" {
		t.Errorf("replacing a lone surrogate = %q, want \"Yx\"", got.ToGoString())
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

// TestCharAtI pins the integer-index form of CharAt: it agrees with CharAt on
// every index, in range and out, including the lone-surrogate read of an astral
// character's halves. Only the index type differs, so the two reads are compared
// unit for unit rather than re-deriving the expected strings.
func TestCharAtI(t *testing.T) {
	s := FromGoString("a😀b")
	for i := -2; i <= 5; i++ {
		viaI := s.CharAtI(i)
		viaF := s.CharAt(float64(i))
		if viaI.Length() != viaF.Length() {
			t.Errorf("CharAtI(%d) Length = %v, CharAt = %v", i, viaI.Length(), viaF.Length())
			continue
		}
		ui, uf := viaI.units(), viaF.units()
		for k := range ui {
			if ui[k] != uf[k] {
				t.Errorf("CharAtI(%d) units = %v, CharAt units = %v", i, ui, uf)
				break
			}
		}
	}
}

// TestAtOpt pins String.prototype.at: an in-range index yields the present
// optional holding the one-code-unit string, a negative index counts from the
// end, and an index that resolves outside the string reads as the undefined
// optional rather than the empty string CharAt yields. A negative index into an
// astral character returns the surrogate half at that unit, matching charAt.
func TestAtOpt(t *testing.T) {
	s := FromGoString("a😀") // 'a' (1 unit), emoji (2 units, a surrogate pair)

	if got := s.AtOpt(0); got.IsUndefined() || got.Get().ToGoString() != "a" {
		t.Errorf("AtOpt(0) = %+v, want Some(\"a\")", got)
	}
	// -1 resolves to the last code unit, the low surrogate of the emoji.
	last := s.AtOpt(-1)
	if last.IsUndefined() {
		t.Fatalf("AtOpt(-1) = undefined, want the low surrogate")
	}
	if u := last.Get().units(); len(u) != 1 || u[0] != 0xDE00 {
		t.Errorf("AtOpt(-1) units = %v, want [0xDE00] (low surrogate)", u)
	}
	// The string is three code units, so -4 counts back past the start and, like a
	// too-large positive index, resolves out of range and reads undefined.
	for _, idx := range []float64{3, 100, -4, -100} {
		if got := s.AtOpt(idx); !got.IsUndefined() {
			t.Errorf("AtOpt(%v) = %+v, want the undefined optional", idx, got)
		}
	}
}

// TestCodePointAtOpt pins String.prototype.codePointAt: a BMP character reads as
// its own code point, a high surrogate followed by a low surrogate combines into
// the astral code point they encode (the difference from charCodeAt), an unpaired
// surrogate stands for its own unit value, and an index outside the string reads
// as the undefined optional.
func TestCodePointAtOpt(t *testing.T) {
	s := FromGoString("a😀") // 'a' (1 unit), emoji U+1F600 (D83D DE00, two units)

	if got := s.CodePointAtOpt(0); got.IsUndefined() || got.Get() != 97 {
		t.Errorf("CodePointAtOpt(0) = %+v, want Some(97)", got)
	}
	// Index 1 is the high surrogate that starts the pair, so it reads the whole
	// astral code point, not the surrogate value charCodeAt would give.
	if got := s.CodePointAtOpt(1); got.IsUndefined() || got.Get() != 0x1F600 {
		t.Errorf("CodePointAtOpt(1) = %+v, want Some(128512)", got)
	}
	// Index 2 is the trailing low surrogate on its own, so it reads that unit value.
	if got := s.CodePointAtOpt(2); got.IsUndefined() || got.Get() != 0xDE00 {
		t.Errorf("CodePointAtOpt(2) = %+v, want Some(0xDE00)", got)
	}
	for _, idx := range []float64{-1, 3, 100} {
		if got := s.CodePointAtOpt(idx); !got.IsUndefined() {
			t.Errorf("CodePointAtOpt(%v) = %+v, want the undefined optional", idx, got)
		}
	}
	// A lone high surrogate with no following unit reads as its own value, not undefined.
	lone := FromUTF16([]uint16{0xD83D})
	if got := lone.CodePointAtOpt(0); got.IsUndefined() || got.Get() != 0xD83D {
		t.Errorf("CodePointAtOpt on a lone high surrogate = %+v, want Some(0xD83D)", got)
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

// TestEndsWith pins String.prototype.endsWith: the suffix is matched in the window
// that ends at the optional end position (defaulting to the length), so an end
// position shorter than the string checks a prefix window, and a suffix past the
// astral character keeps the code-unit indexing.
func TestEndsWith(t *testing.T) {
	s := FromGoString("hello")
	cases := []struct {
		suffix string
		pos    []float64
		want   bool
	}{
		{"lo", nil, true},
		{"he", nil, false},
		{"hell", []float64{4}, true}, // window ends at 4, so "hell"
		{"lo", []float64{4}, false},  // "lo" does not end at index 4
		{"", nil, true},              // the empty suffix ends everywhere
		{"hello world", nil, false},  // a suffix longer than the string
	}
	for _, c := range cases {
		if got := s.EndsWith(FromGoString(c.suffix), c.pos...); got != c.want {
			t.Errorf("EndsWith(%q, %v) = %v, want %v", c.suffix, c.pos, got, c.want)
		}
	}
	// code units: the emoji ends the string at its low-surrogate boundary.
	if !FromGoString("a😀").EndsWith(FromGoString("😀")) {
		t.Error("EndsWith(emoji) = false, want true")
	}
}

// TestSearchPositions pins the optional position on startsWith and includes: a
// startsWith position moves where the prefix must begin, and an includes position
// skips a match that lies before it.
func TestSearchPositions(t *testing.T) {
	s := FromGoString("abcabc")
	if !s.StartsWith(FromGoString("abc"), 3) {
		t.Error("StartsWith(\"abc\", 3) = false, want true")
	}
	if s.StartsWith(FromGoString("abc"), 1) {
		t.Error("StartsWith(\"abc\", 1) = true, want false")
	}
	if s.Includes(FromGoString("a"), 4) {
		t.Error("Includes(\"a\", 4) = true, want false")
	}
	if !s.Includes(FromGoString("a"), 3) {
		t.Error("Includes(\"a\", 3) = false, want true")
	}
}

// TestIndexOfPosition pins the optional start position on indexOf: the scan
// begins at the clamped position, so a match before it is skipped, and the empty
// search returns the clamped position rather than always 0.
func TestIndexOfPosition(t *testing.T) {
	s := FromGoString("abcabc")
	cases := []struct {
		search string
		pos    []float64
		want   float64
	}{
		{"a", nil, 0},
		{"a", []float64{1}, 3},  // the first "a" is before the start
		{"a", []float64{4}, -1}, // no "a" at or after index 4
		{"c", []float64{2}, 2},  // a match exactly at the start counts
		{"", []float64{2}, 2},   // empty search returns the clamped position
		{"", []float64{99}, 6},  // a position past the end clamps to length
		{"a", []float64{-5}, 0}, // a negative position clamps to 0
	}
	for _, c := range cases {
		if got := s.IndexOf(FromGoString(c.search), c.pos...); got != c.want {
			t.Errorf("IndexOf(%q, %v) = %v, want %v", c.search, c.pos, got, c.want)
		}
	}
}

// TestLastIndexOf pins lastIndexOf: it reports the greatest matching index, the
// optional position narrows the window from the right, a NaN or missing position
// means the end, and an astral character keeps the code-unit indexing.
func TestLastIndexOf(t *testing.T) {
	s := FromGoString("abcabc")
	cases := []struct {
		search string
		pos    []float64
		want   float64
	}{
		{"a", nil, 3},          // the last "a", not the first
		{"a", []float64{2}, 0}, // only the "a" at or before index 2
		{"c", nil, 5},
		{"z", nil, -1},
		{"", nil, 6},                    // empty search returns the length
		{"", []float64{2}, 2},           // empty search returns the clamped position
		{"a", []float64{math.NaN()}, 3}, // NaN position means the end, so the last "a"
	}
	for _, c := range cases {
		if got := s.LastIndexOf(FromGoString(c.search), c.pos...); got != c.want {
			t.Errorf("LastIndexOf(%q, %v) = %v, want %v", c.search, c.pos, got, c.want)
		}
	}
	// code units: "b" in "a😀b" is at index 3, past the surrogate pair.
	if got := FromGoString("a😀b").LastIndexOf(FromGoString("b")); got != 3 {
		t.Errorf("LastIndexOf(\"b\") = %v, want 3", got)
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

// TestTrim pins the trim family against the exact ECMAScript whitespace set,
// including a no-break space and a zero-width no-break space that Go's
// unicode.IsSpace classifies differently, and checks that TrimStart and TrimEnd
// touch only their own end.
func TestTrim(t *testing.T) {
	if got := FromGoString("  hi  ").Trim().ToGoString(); got != "hi" {
		t.Errorf("Trim spaces = %q, want \"hi\"", got)
	}
	if got := FromGoString("\t\n hi \r\n").Trim().ToGoString(); got != "hi" {
		t.Errorf("Trim tabs/newlines = %q, want \"hi\"", got)
	}
	if got := FromGoString("\u00a0x\u00a0").Trim().ToGoString(); got != "x" {
		t.Errorf("Trim no-break space = %q, want \"x\"", got)
	}
	if got := FromGoString("\ufeffy\ufeff").Trim().ToGoString(); got != "y" {
		t.Errorf("Trim zero-width no-break space = %q, want \"y\"", got)
	}
	if got := FromGoString("   ").Trim().ToGoString(); got != "" {
		t.Errorf("Trim all whitespace = %q, want empty", got)
	}
	if got := FromGoString("none").Trim().ToGoString(); got != "none" {
		t.Errorf("Trim clean string = %q, want \"none\"", got)
	}
	// U+0085 (NEL) is whitespace to Go but not to JavaScript trim, so it must
	// survive; this is the divergence the custom predicate exists to avoid.
	if got := FromGoString("\u0085z").Trim().ToGoString(); got != "\u0085z" {
		t.Errorf("Trim NEL = %q, want it kept (JavaScript does not trim NEL)", got)
	}
	if got := FromGoString("  hi  ").TrimStart().ToGoString(); got != "hi  " {
		t.Errorf("TrimStart = %q, want \"hi  \"", got)
	}
	if got := FromGoString("  hi  ").TrimEnd().ToGoString(); got != "  hi" {
		t.Errorf("TrimEnd = %q, want \"  hi\"", got)
	}
}

// TestPad pins String.prototype.padStart and padEnd: the pad repeats and
// truncates to the exact fill length, a target no longer than the string and an
// absent or empty pad are no-ops, and the default pad is a space.
func TestPad(t *testing.T) {
	if got := FromGoString("5").PadStart(3, FromGoString("0")).ToGoString(); got != "005" {
		t.Errorf("PadStart repeat = %q, want \"005\"", got)
	}
	if got := FromGoString("5").PadStart(6, FromGoString("ab")).ToGoString(); got != "ababa5" {
		t.Errorf("PadStart truncated pad = %q, want \"ababa5\"", got)
	}
	if got := FromGoString("5").PadStart(1, FromGoString("0")).ToGoString(); got != "5" {
		t.Errorf("PadStart short target = %q, want \"5\"", got)
	}
	if got := FromGoString("5").PadStart(-2, FromGoString("0")).ToGoString(); got != "5" {
		t.Errorf("PadStart negative target = %q, want \"5\"", got)
	}
	if got := FromGoString("abc").PadStart(5, FromGoString("")).ToGoString(); got != "abc" {
		t.Errorf("PadStart empty pad = %q, want \"abc\"", got)
	}
	if got := FromGoString("7").PadStart(4).ToGoString(); got != "   7" {
		t.Errorf("PadStart default pad = %q, want \"   7\"", got)
	}
	if got := FromGoString("5").PadEnd(3, FromGoString("0")).ToGoString(); got != "500" {
		t.Errorf("PadEnd repeat = %q, want \"500\"", got)
	}
	if got := FromGoString("5").PadEnd(6, FromGoString("ab")).ToGoString(); got != "5ababa" {
		t.Errorf("PadEnd truncated pad = %q, want \"5ababa\"", got)
	}
	if got := FromGoString("7").PadEnd(4).ToGoString(); got != "7   " {
		t.Errorf("PadEnd default pad = %q, want \"7   \"", got)
	}
	// The pad is copied by code unit, so truncating in the middle of an astral pad
	// emits a lone surrogate, exactly as JavaScript does. "😀" is two code units,
	// so padding "x" to length 2 with it takes the high surrogate alone.
	got := FromGoString("x").PadStart(2, FromGoString("😀"))
	if got.Length() != 2 {
		t.Fatalf("PadStart astral pad length = %v, want 2", got.Length())
	}
	if u := got.CharCodeAt(0); u != 0xD83D {
		t.Errorf("PadStart astral pad first unit = %04X, want D83D (lone high surrogate)", uint16(u))
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

// TestSubstr pins the legacy String.prototype.substr, which takes a start and a
// count: an interior run, a negative start counting from the end, a start past
// the end giving empty, an omitted length running to the end, a length past the
// end clamping, and a zero or negative length giving empty.
func TestSubstr(t *testing.T) {
	h := FromGoString("hello")
	cases := []struct {
		name string
		args []float64
		want string
	}{
		{"interior", []float64{1, 3}, "ell"},
		{"negativeStartFromEnd", []float64{-2}, "lo"},
		{"negativeStartClampsToZero", []float64{-10}, "hello"},
		{"startPastEndEmpty", []float64{10}, ""},
		{"oneArgToEnd", []float64{2}, "llo"},
		{"lengthPastEndClamps", []float64{2, 100}, "llo"},
		{"zeroLengthEmpty", []float64{1, 0}, ""},
		{"negativeLengthEmpty", []float64{1, -1}, ""},
		{"fractionalStartTruncates", []float64{1.9, 2}, "el"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.Substr(tc.args...).ToGoString(); got != tc.want {
				t.Errorf("Substr(%v) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}
}

// TestRepeat pins String.prototype.repeat: the string concatenated count times,
// with a count coerced to an integer, zero and empty yielding the empty string,
// and one returning the receiver. The astral case proves the code-unit view is
// repeated whole, so a surrogate pair survives the copy.
func TestRepeat(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		count float64
		want  string
	}{
		{"twice", "ab", 2, "abab"},
		{"thrice", "x", 3, "xxx"},
		{"zero", "ab", 0, ""},
		{"one", "ab", 1, "ab"},
		{"emptyReceiver", "", 5, ""},
		{"fractionTruncates", "ab", 2.9, "abab"},
		{"nanIsZero", "ab", math.NaN(), ""},
		{"astral", "\U0001F600", 2, "\U0001F600\U0001F600"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FromGoString(tc.in).Repeat(tc.count).ToGoString(); got != tc.want {
				t.Errorf("Repeat(%q, %v) = %q, want %q", tc.in, tc.count, got, tc.want)
			}
		})
	}
}

// TestRepeatNegativePanics pins that a negative count is a RangeError in
// JavaScript, which bento surfaces as a panic since the compiled program has no
// exception machinery yet.
func TestRepeatNegativePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Repeat(-1) did not panic")
		}
	}()
	FromGoString("ab").Repeat(-1)
}
