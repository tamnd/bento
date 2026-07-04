// This file owns the URI-component codec globals, encodeURIComponent and
// decodeURIComponent (12_json_console_globals). Both work on the UTF-16 string
// the runtime carries and cross to UTF-8 bytes the way the ECMAScript Encode and
// Decode operations do: encodeURIComponent leaves the unreserved characters
// alone and percent-encodes every other code point's UTF-8 bytes, and
// decodeURIComponent reverses it. The encode rejects a lone surrogate and the
// decode rejects a malformed escape, each with a URIError, matching the
// specification rather than substituting a replacement character the way a naive
// UTF-8 round-trip would.

package value

import (
	"unicode/utf16"
	"unicode/utf8"
)

// hexDigits is the uppercase hex alphabet the percent-encoder writes, uppercase
// because encodeURIComponent emits %XX with uppercase digits.
const hexDigits = "0123456789ABCDEF"

// uriComponentUnreserved reports whether an ASCII code point is one
// encodeURIComponent leaves unescaped: the unreserved set A-Za-z0-9 and the
// marks -_.!~*'(). Every other code point, including the URI-reserved
// punctuation, is percent-encoded, which is what separates the component encoder
// from encodeURI.
func uriComponentUnreserved(c rune) bool {
	switch {
	case c >= 'A' && c <= 'Z':
		return true
	case c >= 'a' && c <= 'z':
		return true
	case c >= '0' && c <= '9':
		return true
	}
	switch c {
	case '-', '_', '.', '!', '~', '*', '\'', '(', ')':
		return true
	}
	return false
}

// EncodeURIComponent is encodeURIComponent(s): it keeps the unreserved
// characters and percent-encodes the UTF-8 bytes of every other code point. It
// walks the UTF-16 code units so it can combine a surrogate pair into its code
// point and reject a lone surrogate with a URIError, the case a plain UTF-8
// conversion would silently turn into a replacement character.
func EncodeURIComponent(s BStr) BStr {
	units := s.units()
	out := make([]byte, 0, len(units))
	for i := 0; i < len(units); i++ {
		u := units[i]
		var cp rune
		switch {
		case u < 0xD800 || u > 0xDFFF:
			cp = rune(u)
		case u <= 0xDBFF && i+1 < len(units) && units[i+1] >= 0xDC00 && units[i+1] <= 0xDFFF:
			cp = 0x10000 + (rune(u)-0xD800)<<10 + (rune(units[i+1]) - 0xDC00)
			i++
		default:
			Throw(NewURIError(FromGoString("URI malformed")))
		}
		if cp < 0x80 && uriComponentUnreserved(cp) {
			out = append(out, byte(cp))
			continue
		}
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], cp)
		for _, b := range buf[:n] {
			out = append(out, '%', hexDigits[b>>4], hexDigits[b&0xF])
		}
	}
	return FromGoString(string(out))
}

// hexNibble returns the value of a hex digit code unit and whether it was one,
// the per-digit half of a percent-escape parse.
func hexNibble(u uint16) (byte, bool) {
	switch {
	case u >= '0' && u <= '9':
		return byte(u - '0'), true
	case u >= 'A' && u <= 'F':
		return byte(u-'A') + 10, true
	case u >= 'a' && u <= 'f':
		return byte(u-'a') + 10, true
	}
	return 0, false
}

// DecodeURIComponent is decodeURIComponent(s): it turns each run of %XX escapes
// back into the UTF-8 bytes they name and decodes those bytes to code points,
// passing every other code unit through unchanged. A '%' not followed by two hex
// digits, or an escape run whose bytes are not valid UTF-8, is malformed and
// raises a URIError rather than yielding a replacement character.
func DecodeURIComponent(s BStr) BStr {
	units := s.units()
	out := make([]uint16, 0, len(units))
	i := 0
	for i < len(units) {
		if units[i] != '%' {
			out = append(out, units[i])
			i++
			continue
		}
		var bytes []byte
		for i < len(units) && units[i] == '%' {
			if i+2 >= len(units) {
				Throw(NewURIError(FromGoString("URI malformed")))
			}
			hi, ok1 := hexNibble(units[i+1])
			lo, ok2 := hexNibble(units[i+2])
			if !ok1 || !ok2 {
				Throw(NewURIError(FromGoString("URI malformed")))
			}
			bytes = append(bytes, hi<<4|lo)
			i += 3
		}
		for len(bytes) > 0 {
			r, size := utf8.DecodeRune(bytes)
			if r == utf8.RuneError && size <= 1 {
				Throw(NewURIError(FromGoString("URI malformed")))
			}
			out = utf16.AppendRune(out, r)
			bytes = bytes[size:]
		}
	}
	return FromUTF16(out)
}
