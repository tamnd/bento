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
		{"escapedLetter", `\a\z`, u16("az")},
		{"astralLiteral", "a😀b", u16("a😀b")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := decodeJSString(c.in)
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
// out-of-range \u, and a legacy octal escape that is a syntax error in a module.
func TestDecodeJSStringRejects(t *testing.T) {
	for _, in := range []string{
		`a\`,         // dangling backslash
		`\x4`,        // one hex digit where two are needed
		`\xZZ`,       // non-hex digits
		`\uD8`,       // fewer than four hex digits
		`\u{}`,       // empty braces
		`\u{110000}`, // past the Unicode range
		`\u{1F600`,   // no closing brace
		`\01`,        // legacy octal escape
	} {
		if _, ok := decodeJSString(in); ok {
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
