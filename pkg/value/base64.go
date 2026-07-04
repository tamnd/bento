// This file owns the two base64 transcode globals, btoa and atob
// (12_json_console_globals). They are the binary-string codec the web platform
// carries: btoa takes a string whose code units are each a byte and returns its
// base64 encoding, and atob reverses it, decoding base64 back to a string whose
// code units are the decoded bytes. Both work on the UTF-16 string the runtime
// carries, reading and writing one byte per code unit, and both raise an
// InvalidCharacterError, the DOMException the platform throws, on input they
// cannot transcode: btoa on a code unit above 0xFF, atob on base64 that is the
// wrong length or holds a character outside the alphabet.

package value

import "encoding/base64"

// Btoa is btoa(s): it reads each code unit as a byte and returns the base64
// encoding of that byte sequence. A code unit above 0xFF is not a byte, so it
// cannot be a binary-string character, and btoa raises an InvalidCharacterError
// rather than truncate it.
func Btoa(s BStr) BStr {
	units := s.units()
	bytes := make([]byte, len(units))
	for i, u := range units {
		if u > 0xFF {
			Throw(NewInvalidCharacterError())
		}
		bytes[i] = byte(u)
	}
	return FromGoString(base64.StdEncoding.EncodeToString(bytes))
}

// base64Sextet returns the six-bit value of a base64 alphabet code unit and
// whether it was one, the per-character half of the atob decode. The padding '='
// is deliberately not one: atob strips the trailing padding before decoding, so a
// '=' that reaches here sits inside the data and is invalid.
func base64Sextet(u uint16) (byte, bool) {
	switch {
	case u >= 'A' && u <= 'Z':
		return byte(u - 'A'), true
	case u >= 'a' && u <= 'z':
		return byte(u-'a') + 26, true
	case u >= '0' && u <= '9':
		return byte(u-'0') + 52, true
	case u == '+':
		return 62, true
	case u == '/':
		return 63, true
	}
	return 0, false
}

// Atob is atob(s): it decodes base64 back to a string whose code units are the
// decoded bytes, following the WHATWG forgiving-base64 rules. It strips ASCII
// whitespace, drops the trailing padding, and rejects input of the wrong length or
// with a character outside the alphabet with an InvalidCharacterError. The final
// group's spare bits are discarded rather than required to be zero, which is the
// "forgiving" part and what parts atob from a strict base64 decoder.
func Atob(s BStr) BStr {
	units := s.units()
	// Remove all ASCII whitespace: tab, line feed, form feed, carriage return, space.
	clean := make([]uint16, 0, len(units))
	for _, u := range units {
		switch u {
		case 0x09, 0x0A, 0x0C, 0x0D, 0x20:
			continue
		}
		clean = append(clean, u)
	}
	// A length that is a multiple of four may carry one or two padding characters,
	// which are dropped before decoding.
	if len(clean)%4 == 0 {
		for k := 0; k < 2 && len(clean) > 0 && clean[len(clean)-1] == '='; k++ {
			clean = clean[:len(clean)-1]
		}
	}
	// A remainder of one leaves a lone six-bit group, which cannot form a byte.
	if len(clean)%4 == 1 {
		Throw(NewInvalidCharacterError())
	}
	out := make([]byte, 0, len(clean)*3/4)
	var acc uint32
	var bits int
	for _, u := range clean {
		v, ok := base64Sextet(u)
		if !ok {
			Throw(NewInvalidCharacterError())
		}
		acc = acc<<6 | uint32(v)
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>uint(bits)))
		}
	}
	// The leftover bits (fewer than eight) are the group's padding and are dropped.
	res := make([]uint16, len(out))
	for i, b := range out {
		res[i] = uint16(b)
	}
	return FromUTF16(res)
}
