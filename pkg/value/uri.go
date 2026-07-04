// This file owns the four URI codec globals, encodeURIComponent /
// decodeURIComponent and encodeURI / decodeURI (12_json_console_globals). All
// four work on the UTF-16 string the runtime carries and cross to UTF-8 bytes the
// way the ECMAScript Encode and Decode operations do: an encoder leaves a set of
// characters unescaped and percent-encodes every other code point's UTF-8 bytes,
// and a decoder reverses it. The two pairs differ only in that set. The component
// codec escapes everything outside the unreserved set, so it is safe to drop a
// value into one query component; the whole-URI codec also leaves the reserved
// punctuation that structures a URI (the delimiters ; / ? : @ & = + $ , and #)
// alone, so it round-trips a complete URI without dissolving its structure.
//
// An encode rejects a lone surrogate and a decode rejects a malformed escape, each
// with a URIError, matching the specification rather than substituting a
// replacement character the way a naive UTF-8 round-trip would.

package value

import (
	"unicode/utf16"
	"unicode/utf8"
)

// hexDigits is the uppercase hex alphabet the percent-encoder writes, uppercase
// because the URI encoders emit %XX with uppercase digits.
const hexDigits = "0123456789ABCDEF"

// uriUnreserved reports whether an ASCII code point is in the unreserved set every
// URI encoder leaves alone: the letters and digits A-Za-z0-9 and the marks
// -_.!~*'(). Both encoders keep these; they part only on the reserved punctuation.
func uriUnreserved(c rune) bool {
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

// uriReserved reports whether a byte is one of the reserved delimiters that
// structure a URI, the set ; / ? : @ & = + $ , and #. encodeURI adds this set to
// the characters it leaves unescaped, and decodeURI preserves an escape that would
// decode to one of them, so the pair round-trips a URI without escaping or
// unescaping the punctuation that separates its parts. encodeURIComponent and
// decodeURIComponent ignore the set, which is the whole of the difference between
// the component codec and the whole-URI codec.
func uriReserved(c byte) bool {
	switch c {
	case ';', '/', '?', ':', '@', '&', '=', '+', '$', ',', '#':
		return true
	}
	return false
}

// componentUnescaped is the unescaped-character test for encodeURIComponent: the
// unreserved set only, so every reserved delimiter is percent-encoded.
func componentUnescaped(c rune) bool { return uriUnreserved(c) }

// uriUnescaped is the unescaped-character test for encodeURI: the unreserved set
// plus the reserved delimiters, so the punctuation that structures a URI survives
// the encode.
func uriUnescaped(c rune) bool { return uriUnreserved(c) || (c < 0x80 && uriReserved(byte(c))) }

// EncodeURIComponent is encodeURIComponent(s): it keeps the unreserved characters
// and percent-encodes the UTF-8 bytes of every other code point, the encoder for a
// single URI component.
func EncodeURIComponent(s BStr) BStr { return encodeURI(s, componentUnescaped) }

// EncodeURI is encodeURI(s): it keeps the unreserved characters and the reserved
// delimiters and percent-encodes every other code point, the encoder for a whole
// URI that must keep its structure.
func EncodeURI(s BStr) BStr { return encodeURI(s, uriUnescaped) }

// encodeURI is the shared body of the two encoders. It walks the UTF-16 code units
// so it can combine a surrogate pair into its code point and reject a lone
// surrogate with a URIError, the case a plain UTF-8 conversion would silently turn
// into a replacement character. A code point the unescaped test keeps passes
// through as its ASCII byte; every other code point contributes the %XX escapes of
// its UTF-8 bytes.
func encodeURI(s BStr, unescaped func(rune) bool) BStr {
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
		if cp < 0x80 && unescaped(cp) {
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

// hexNibble returns the value of a hex digit code unit and whether it was one, the
// per-digit half of a percent-escape parse.
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

// readEscape reads the %XX escape at units[i], which the caller has checked begins
// with '%', and returns the byte it names and the index just past it. A '%' not
// followed by two hex digits is malformed and raises a URIError.
func readEscape(units []uint16, i int) (byte, int) {
	if i+2 >= len(units) {
		Throw(NewURIError(FromGoString("URI malformed")))
	}
	hi, ok1 := hexNibble(units[i+1])
	lo, ok2 := hexNibble(units[i+2])
	if !ok1 || !ok2 {
		Throw(NewURIError(FromGoString("URI malformed")))
	}
	return hi<<4 | lo, i + 3
}

// utf8Len reports the number of bytes in the UTF-8 sequence a lead byte starts, or
// 0 when the byte is not a valid lead byte (a continuation byte or an out-of-range
// prefix). It is how the decoder learns how many further escapes a multibyte code
// point needs before it has read the whole sequence.
func utf8Len(b byte) int {
	switch {
	case b < 0x80:
		return 1
	case b >= 0xC0 && b < 0xE0:
		return 2
	case b >= 0xE0 && b < 0xF0:
		return 3
	case b >= 0xF0 && b < 0xF8:
		return 4
	}
	return 0
}

// DecodeURIComponent is decodeURIComponent(s): it turns each %XX escape back into
// the byte it names, decodes each UTF-8 sequence to its code point, and passes
// every other code unit through, the reverse of encodeURIComponent.
func DecodeURIComponent(s BStr) BStr { return decodeURI(s, func(byte) bool { return false }) }

// DecodeURI is decodeURI(s): it reverses encodeURI, but leaves an escape that
// decodes to a reserved delimiter as the literal %XX it found, so a decoded URI
// keeps the escaped punctuation that a re-encode would have to restore anyway. This
// preserve rule is the only way decodeURI parts from decodeURIComponent.
func DecodeURI(s BStr) BStr { return decodeURI(s, uriReserved) }

// decodeURI is the shared body of the two decoders. It walks the code units one
// code point at a time: a non-escape unit passes through, and a '%' begins an
// escape run whose first byte fixes the UTF-8 length, so the decoder gathers that
// many escapes and decodes the sequence. A single-byte code point the preserve
// test keeps is re-emitted as its original %XX literal rather than the byte, which
// is how decodeURI keeps the reserved delimiters escaped. A '%' not followed by two
// hex digits, or an escape run that is not valid UTF-8, is malformed and raises a
// URIError rather than yielding a replacement character.
func decodeURI(s BStr, preserved func(byte) bool) BStr {
	units := s.units()
	out := make([]uint16, 0, len(units))
	i := 0
	for i < len(units) {
		if units[i] != '%' {
			out = append(out, units[i])
			i++
			continue
		}
		start := i
		b, next := readEscape(units, i)
		n := utf8Len(b)
		if n == 0 {
			Throw(NewURIError(FromGoString("URI malformed")))
		}
		if n == 1 {
			if preserved(b) {
				out = append(out, units[start], units[start+1], units[start+2])
			} else {
				out = append(out, uint16(b))
			}
			i = next
			continue
		}
		bytes := []byte{b}
		for len(bytes) < n {
			if next >= len(units) || units[next] != '%' {
				Throw(NewURIError(FromGoString("URI malformed")))
			}
			cb, after := readEscape(units, next)
			if cb < 0x80 || cb >= 0xC0 {
				Throw(NewURIError(FromGoString("URI malformed")))
			}
			bytes = append(bytes, cb)
			next = after
		}
		r, size := utf8.DecodeRune(bytes)
		if r == utf8.RuneError || size != n {
			Throw(NewURIError(FromGoString("URI malformed")))
		}
		out = utf16.AppendRune(out, r)
		i = next
	}
	return FromUTF16(out)
}
