package lower

import (
	"unicode/utf16"
	"unicode/utf8"
)

// This file decodes a JavaScript string literal's source text into UTF-16 code
// units at compile time (05_type_lowering, the string-literal slice). The runtime
// content of a literal is not its source text: the source carries backslash
// escapes that must be resolved, and a \u escape can name a lone surrogate that
// UTF-8 cannot hold, so the decoder produces code units directly rather than a Go
// string. The caller turns a valid (no lone surrogate) result back into a Go
// string for value.FromGoString and keeps the code-unit slice only when it has to,
// for value.FromUTF16.

// decodeJSString resolves the escapes in the content of a JavaScript string
// literal (the text between the quotes) into UTF-16 code units. It returns false
// when the content is not something this slice can soundly decode: a dangling
// backslash, a malformed \x or \u escape, or a legacy octal escape, which is a
// syntax error in a module anyway but is refused here rather than guessed. A
// decoded lone surrogate is kept as its raw code unit, since that is a legal
// JavaScript string this compiler must preserve.
func decodeJSString(inner string) ([]uint16, bool) {
	out := make([]uint16, 0, len(inner))
	i := 0
	for i < len(inner) {
		if inner[i] != '\\' {
			// An ordinary character: decode one code point from the UTF-8 source and
			// append it as one or two code units. This is never a surrogate, since
			// source text is valid UTF-8, so utf16.AppendRune is exact here.
			r, size := utf8.DecodeRuneInString(inner[i:])
			out = utf16.AppendRune(out, r)
			i += size
			continue
		}
		// An escape: step past the backslash and decode by the next character.
		i++
		if i >= len(inner) {
			return nil, false // a backslash with nothing after it
		}
		switch e := inner[i]; e {
		case 'n':
			out = append(out, '\n')
			i++
		case 't':
			out = append(out, '\t')
			i++
		case 'r':
			out = append(out, '\r')
			i++
		case 'b':
			out = append(out, '\b')
			i++
		case 'f':
			out = append(out, '\f')
			i++
		case 'v':
			out = append(out, '\v')
			i++
		case '0':
			// \0 is the null character only when a digit does not follow; \01 would be
			// a legacy octal escape, which is a syntax error in a module.
			if i+1 < len(inner) && inner[i+1] >= '0' && inner[i+1] <= '9' {
				return nil, false
			}
			out = append(out, 0)
			i++
		case 'x':
			// \xHH: exactly two hex digits naming one code unit.
			v, ok := hexN(inner, i+1, 2)
			if !ok {
				return nil, false
			}
			out = append(out, uint16(v))
			i += 3
		case 'u':
			units, next, ok := decodeUnicodeEscape(inner, i+1)
			if !ok {
				return nil, false
			}
			out = append(out, units...)
			i = next
		case '\n':
			// A line continuation: a backslash before a newline produces nothing.
			i++
		case '\r':
			i++
			if i < len(inner) && inner[i] == '\n' {
				i++ // a CRLF line continuation is a single break
			}
		default:
			// Any other escaped character stands for itself (\a is a, \' is '), so
			// append the escaped code point verbatim.
			r, size := utf8.DecodeRuneInString(inner[i:])
			out = utf16.AppendRune(out, r)
			i += size
		}
	}
	return out, true
}

// decodeUnicodeEscape decodes the part of a \u escape after the u, starting at
// index start. It handles both \uHHHH (four hex digits, one code unit that may be
// a lone surrogate) and \u{H...H} (a braced code point that encodes to one or two
// code units). It returns the code units, the index just past the escape, and
// whether the escape was well formed.
func decodeUnicodeEscape(s string, start int) (units []uint16, next int, ok bool) {
	if start < len(s) && s[start] == '{' {
		// \u{H...H}: hex digits up to a closing brace, naming a code point.
		j := start + 1
		v := 0
		digits := 0
		for j < len(s) && s[j] != '}' {
			d, isHex := hexDigit(s[j])
			if !isHex {
				return nil, 0, false
			}
			v = v*16 + d
			digits++
			if v > 0x10FFFF {
				return nil, 0, false // a code point past the Unicode range
			}
			j++
		}
		if digits == 0 || j >= len(s) {
			return nil, 0, false // empty braces or no closing brace
		}
		return codePointUnits(v), j + 1, true
	}
	// \uHHHH: exactly four hex digits naming one code unit, kept raw so a lone
	// surrogate survives.
	v, hexOK := hexN(s, start, 4)
	if !hexOK {
		return nil, 0, false
	}
	return []uint16{uint16(v)}, start + 4, true
}

// codePointUnits turns a code point into its UTF-16 code units. A value in the
// Basic Multilingual Plane, including a surrogate value from an explicit \u{D83D},
// is one code unit kept raw; a value above the BMP is a surrogate pair.
func codePointUnits(cp int) []uint16 {
	if cp <= 0xFFFF {
		return []uint16{uint16(cp)}
	}
	r1, r2 := utf16.EncodeRune(rune(cp))
	return []uint16{uint16(r1), uint16(r2)}
}

// hexN reads exactly n hex digits from s starting at index start and returns their
// value, or false if there are fewer than n digits or a non-hex character.
func hexN(s string, start, n int) (int, bool) {
	if start+n > len(s) {
		return 0, false
	}
	v := 0
	for k := 0; k < n; k++ {
		d, ok := hexDigit(s[start+k])
		if !ok {
			return 0, false
		}
		v = v*16 + d
	}
	return v, true
}

// hexDigit maps a single hex digit character to its value.
func hexDigit(c byte) (int, bool) {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0'), true
	case c >= 'a' && c <= 'f':
		return int(c-'a') + 10, true
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10, true
	default:
		return 0, false
	}
}

// hasLoneSurrogate reports whether units contains a surrogate code unit that is
// not part of a valid high-then-low pair. A slice with a lone surrogate cannot be
// carried as a Go string, so the caller must emit it as a raw code-unit slice.
func hasLoneSurrogate(units []uint16) bool {
	for i := 0; i < len(units); i++ {
		u := units[i]
		if u < 0xD800 || u > 0xDFFF {
			continue
		}
		if u >= 0xDC00 {
			return true // a low surrogate with no high before it
		}
		// A high surrogate: it must be followed by a low surrogate to be a pair.
		if i+1 >= len(units) || units[i+1] < 0xDC00 || units[i+1] > 0xDFFF {
			return true
		}
		i++ // consume the low half of the pair
	}
	return false
}
