package lower

import (
	"reflect"
	"testing"
	"unicode/utf16"
)

// TestDecodeJSString pins the escape decoder over the whole grammar this slice
// covers: the single-character escapes, \x and \u byte and code-unit escapes, the
// braced \u{...} code-point escape including an astral pair, a lone surrogate that
// must survive, line continuations, and an escaped ordinary character that stands
// for itself. Each case compares the decoded code units against the expected
// UTF-16.
func TestDecodeJSString(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []uint16
	}{
		{"plain", "abc", u16("abc")},
		{"newlineTab", `a\nb\tc`, []uint16{'a', '\n', 'b', '\t', 'c'}},
		{"escapedQuote", `it\'s`, u16("it's")},
		{"backslash", `a\\b`, []uint16{'a', '\\', 'b'}},
		{"null", `a\0b`, []uint16{'a', 0, 'b'}},
		{"hex", `\x41\x42`, []uint16{0x41, 0x42}},
		{"unicodeBMP", `café`, u16("café")},
		{"unicodeBraced", `a\u{1F600}b`, u16("a😀b")},
		{"unicodeBracedBMP", `\u{41}`, []uint16{0x41}},
		{"loneSurrogate", `\uD83D`, []uint16{0xD83D}},
		{"surrogatePairEscaped", `😀`, []uint16{0xD83D, 0xDE00}},
		{"lineContinuation", "a\\\nb", []uint16{'a', 'b'}},
		{"lineContinuationCR", "a\\\rb", []uint16{'a', 'b'}},
		{"lineContinuationCRLF", "a\\\r\nb", []uint16{'a', 'b'}},
		{"lineContinuationLS", "a\\ b", []uint16{'a', 'b'}},
		{"lineContinuationPS", "a\\ b", []uint16{'a', 'b'}},
		{"escapedLetter", `\a\z`, u16("az")},
		{"astralLiteral", "a😀b", u16("a😀b")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decodeJSString(c.in, true)
			if !ok {
				t.Fatalf("decodeJSString(%q) returned ok=false", c.in)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("decodeJSString(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestDecodeJSStringRejects pins the contents the decoder refuses rather than
// guessing at: a dangling backslash, a short or non-hex \x, a malformed or
// out-of-range \u.
func TestDecodeJSStringRejects(t *testing.T) {
	for _, in := range []string{
		`a\`,         // dangling backslash
		`\x4`,        // one hex digit where two are needed
		`\xZZ`,       // non-hex digits
		`\uD8`,       // fewer than four hex digits
		`\u{}`,       // empty braces
		`\u{110000}`, // past the Unicode range
		`\u{1F600`,   // no closing brace
	} {
		if _, ok := decodeJSString(in, true); ok {
			t.Errorf("decodeJSString(%q) = ok, want refused", in)
		}
	}
}

// TestHasLoneSurrogate pins the pair detector: a valid high-then-low pair is not
// lone, but a bare high, a bare low, or a high not followed by a low is.
func TestHasLoneSurrogate(t *testing.T) {
	cases := []struct {
		name  string
		units []uint16
		want  bool
	}{
		{"ascii", []uint16{'a', 'b'}, false},
		{"validPair", []uint16{0xD83D, 0xDE00}, false},
		{"loneHigh", []uint16{0xD83D}, true},
		{"loneLow", []uint16{0xDE00}, true},
		{"highThenAscii", []uint16{0xD83D, 'a'}, true},
		{"pairThenLoneHigh", []uint16{0xD83D, 0xDE00, 0xD83D}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := hasLoneSurrogate(c.units); got != c.want {
				t.Errorf("hasLoneSurrogate(%v) = %v, want %v", c.units, got, c.want)
			}
		})
	}
}

// u16 is a test helper that encodes a Go string (valid UTF-8, no lone surrogates)
// to its UTF-16 code units, so the expected values read as plain strings.
func u16(s string) []uint16 {
	return utf16.Encode([]rune(s))
}

// TestDecodeJSStringLegacyOctal pins the Annex B escapes a sloppy script may
// spell. The grammar's shape is that a leading digit of 3 or less admits a third
// digit and a leading 4 through 7 does not, which is how it keeps every value
// inside one byte, so the cases cover both the longest run each leading digit
// allows and the digit past it that has to be left alone. "\08" and "\8" are the
// two that read wrong under a naive octal scan: the first is NUL followed by the
// character 8, and the second is not an octal escape at all.
func TestDecodeJSStringLegacyOctal(t *testing.T) {
	cases := []struct {
		in   string
		want []uint16
	}{
		{`\251`, []uint16{0o251}},
		{`\1`, []uint16{1}},
		{`\17`, []uint16{0o17}},
		{`\377`, []uint16{255}},
		{`\0`, []uint16{0}},
		{`\08`, []uint16{0, '8'}},
		{`\8`, []uint16{'8'}},
		{`\9`, []uint16{'9'}},
		{`\400`, []uint16{0o40, '0'}}, // 4 through 7 take one more digit, not two
		{`\3777`, []uint16{255, '7'}}, // three digits at most
		{`\0011`, []uint16{1, '1'}},   // likewise, counted from the leading zero
		{`a\251b`, []uint16{'a', 0o251, 'b'}},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			got, ok := decodeJSString(c.in, true)
			if !ok {
				t.Fatalf("decodeJSString(%q, true) returned ok=false", c.in)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("decodeJSString(%q, true) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

// TestDecodeJSStringOctalRefusedInTemplate pins the other half of the flag. A
// legacy octal escape is a SyntaxError inside a template literal in every mode, so
// the template caller passes false and the decoder refuses rather than decoding.
// The one form that stays legal there is \0 with no digit after it.
func TestDecodeJSStringOctalRefusedInTemplate(t *testing.T) {
	for _, in := range []string{`\251`, `\1`, `\08`} {
		if _, ok := decodeJSString(in, false); ok {
			t.Errorf("decodeJSString(%q, false) = ok, want refused", in)
		}
	}
	got, ok := decodeJSString(`a\0b`, false)
	if !ok || !reflect.DeepEqual(got, []uint16{'a', 0, 'b'}) {
		t.Errorf(`decodeJSString("a\0b", false) = %v, %v, want [97 0 98], true`, got, ok)
	}
}
