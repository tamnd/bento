// This file is the encoding layer under Node's Buffer: the eight names a Buffer method
// accepts for an encoding, and the two directions each of them runs, a string to bytes
// and bytes back to a string.
//
// The layer is separate from the member table because every part of Buffer's surface
// goes through it. Buffer.from(string, enc), buf.toString(enc), buf.write(str, enc),
// buf.fill(str, enc), Buffer.byteLength(str, enc) and buf.indexOf(str, enc) are one
// encode or one decode each, so writing them once here is what keeps the six agreeing
// on what 'ucs2' means and on how a bad base64 character is treated.
//
// The names are Node's, aliases included, and they are matched case-insensitively the
// way Node matches them: 'UTF-8', 'utf8' and 'Utf8' are one encoding. Anything else is
// not an encoding, and the caller raises the ERR_UNKNOWN_ENCODING Node raises rather
// than guessing utf8, because a program that misspells an encoding wants to be told.
//
// The two lenient decoders, base64 and hex, are lenient in the exact way Node's are.
// Base64 skips a character outside the alphabet, accepts either the standard +/ or the
// URL-safe -_ pair, stops at the first padding character, and does not require the
// input to be padded at all. Hex takes pairs from the front and stops at the first
// character that is not a hex digit, so a trailing nibble is dropped. Neither throws:
// Buffer.from is a parse of untrusted bytes and Node made both of them total.

package value

import (
	"encoding/binary"
	"strings"
	"unicode/utf8"
)

// bufferEncoding canonicalizes an encoding name, folding Node's aliases onto the seven
// encodings that actually differ. It reports false for a name that is not an encoding,
// which is what Buffer.isEncoding answers and what the ERR_UNKNOWN_ENCODING raise on
// every other entry point is keyed off.
func bufferEncoding(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "utf8", "utf-8":
		return "utf8", true
	case "utf16le", "utf-16le", "ucs2", "ucs-2":
		return "utf16le", true
	case "latin1", "binary":
		return "latin1", true
	case "ascii":
		return "ascii", true
	case "base64":
		return "base64", true
	case "base64url":
		return "base64url", true
	case "hex":
		return "hex", true
	}
	return "", false
}

// bufferEncodingArg reads an encoding out of an argument slot, defaulting to utf8 for a
// missing or undefined one the way every Buffer method that takes an encoding does. An
// argument that is present and is not an encoding raises, so a typo is reported at the
// call rather than silently read as utf8.
func bufferEncodingArg(v Value) string {
	if v.kind == KindUndefined || v.kind == KindNull {
		return "utf8"
	}
	name := ToString(v).ToGoString()
	enc, ok := bufferEncoding(name)
	if !ok {
		Throw(NewNodeError("TypeError", "ERR_UNKNOWN_ENCODING", FromGoString("Unknown encoding: "+name)))
		return "utf8"
	}
	return enc
}

// bufferEncode turns a string into the bytes the named encoding writes for it. The
// encoding is already canonical; a caller holding a raw name runs it through
// bufferEncoding first.
func bufferEncode(s BStr, enc string) []byte {
	switch enc {
	case "utf8":
		// ToGoString is already the lossy transcode Node performs: an unpaired surrogate
		// has no UTF-8 form, so it is written as the U+FFFD replacement.
		return []byte(s.ToGoString())
	case "latin1", "ascii":
		// Writing ascii is writing latin1 in Node: only the read side masks the high bit
		// off, so a round trip through ascii of a byte above 0x7F loses it on the way back
		// and not on the way in.
		units := s.units()
		out := make([]byte, len(units))
		for i, u := range units {
			out[i] = byte(u)
		}
		return out
	case "utf16le":
		units := s.units()
		out := make([]byte, len(units)*2)
		for i, u := range units {
			binary.LittleEndian.PutUint16(out[i*2:], u)
		}
		return out
	case "base64", "base64url":
		return base64DecodeLenient(s.units())
	case "hex":
		return hexDecodeLenient(s.units())
	}
	return nil
}

// bufferDecode turns bytes back into the string the named encoding reads them as.
func bufferDecode(b []byte, enc string) BStr {
	switch enc {
	case "utf8":
		return FromGoString(utf8Lossy(b))
	case "latin1":
		units := make([]uint16, len(b))
		for i, c := range b {
			units[i] = uint16(c)
		}
		return FromUTF16(units)
	case "ascii":
		// The one place ascii differs from latin1: the high bit is dropped before the byte
		// is read as a code point, so 0xE9 reads as U+0069 rather than as U+00E9.
		units := make([]uint16, len(b))
		for i, c := range b {
			units[i] = uint16(c & 0x7F)
		}
		return FromUTF16(units)
	case "utf16le":
		// An odd trailing byte is not a code unit, so it is dropped rather than padded.
		units := make([]uint16, len(b)/2)
		for i := range units {
			units[i] = binary.LittleEndian.Uint16(b[i*2:])
		}
		return FromUTF16(units)
	case "base64":
		return FromGoString(base64EncodeAlphabet(b, base64StdAlphabet, true))
	case "base64url":
		return FromGoString(base64EncodeAlphabet(b, base64URLAlphabet, false))
	case "hex":
		const digits = "0123456789abcdef"
		out := make([]byte, len(b)*2)
		for i, c := range b {
			out[i*2] = digits[c>>4]
			out[i*2+1] = digits[c&0x0F]
		}
		return FromGoString(string(out))
	}
	return FromGoString("")
}

// utf8Lossy reads a byte run as UTF-8, replacing every ill-formed sequence in it with a
// single U+FFFD. Go's own []byte to string conversion would keep the bad bytes, and a
// BStr on the UTF-8 fast path promises its backing is valid, so the replacement happens
// here rather than being discovered later by something counting code units.
//
// How much a bad sequence costs is the part that has to be got right, because the count
// of replacement characters is observable: it is the string's length. The rule is
// WHATWG's maximal subpart, which node follows. A truncated three-byte sequence is one
// mistake and yields one U+FFFD, so Buffer.from([0xe2, 0x82]).toString() has length one,
// not two. A start byte followed by something that could not have continued it is a
// shorter mistake, so [0xe2, 0x41] yields one U+FFFD and then the A. Go's DecodeRune
// reports every bad sequence as one byte wide, which would give one replacement per byte
// and a string node calls shorter, so the width is measured here instead.
func utf8Lossy(b []byte) string {
	if utf8.Valid(b) {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		if r == utf8.RuneError && size <= 1 {
			sb.WriteRune(utf8.RuneError)
			b = b[utf8MaximalSubpart(b):]
			continue
		}
		sb.Write(b[:size])
		b = b[size:]
	}
	return sb.String()
}

// utf8MaximalSubpart returns the width of the ill-formed sequence at the front of b: the
// start byte plus every byte after it that could still have continued the sequence that
// byte began. The ranges are UTF-8's own, and the two narrowed ones are what keep an
// overlong encoding and a surrogate from counting as a good prefix: after 0xe0 only 0xa0
// and up can follow, and after 0xed only 0x9f and below, so [0xed, 0xa0, 0x80] is three
// separate mistakes rather than one, which is what node counts too.
func utf8MaximalSubpart(b []byte) int {
	var want int
	lo, hi := byte(0x80), byte(0xbf)
	switch c := b[0]; {
	case c >= 0xc2 && c <= 0xdf:
		want = 2
	case c == 0xe0:
		want, lo = 3, 0xa0
	case c == 0xed:
		want, hi = 3, 0x9f
	case c >= 0xe1 && c <= 0xef:
		want = 3
	case c == 0xf0:
		want, lo = 4, 0x90
	case c == 0xf4:
		want, hi = 4, 0x8f
	case c >= 0xf1 && c <= 0xf3:
		want = 4
	default:
		// Not a start byte at all, so it begins nothing and the mistake is the one byte.
		return 1
	}
	n := 1
	for ; n < want && n < len(b); n++ {
		if b[n] < lo || b[n] > hi {
			break
		}
		// Only the second byte carries a narrowed range; the rest take the full one.
		lo, hi = 0x80, 0xbf
	}
	return n
}

const (
	base64StdAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	base64URLAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_"
)

// base64EncodeAlphabet writes bytes as base64 over the given alphabet, padding only when
// asked. Node pads the standard encoding and does not pad the URL-safe one, which is the
// only difference between the two beyond the last two characters.
func base64EncodeAlphabet(b []byte, alphabet string, pad bool) string {
	var sb strings.Builder
	sb.Grow((len(b) + 2) / 3 * 4)
	for i := 0; i < len(b); i += 3 {
		var n uint32
		rest := len(b) - i
		n = uint32(b[i]) << 16
		if rest > 1 {
			n |= uint32(b[i+1]) << 8
		}
		if rest > 2 {
			n |= uint32(b[i+2])
		}
		sb.WriteByte(alphabet[(n>>18)&0x3F])
		sb.WriteByte(alphabet[(n>>12)&0x3F])
		switch {
		case rest > 2:
			sb.WriteByte(alphabet[(n>>6)&0x3F])
			sb.WriteByte(alphabet[n&0x3F])
		case rest == 2:
			sb.WriteByte(alphabet[(n>>6)&0x3F])
			if pad {
				sb.WriteByte('=')
			}
		default:
			if pad {
				sb.WriteString("==")
			}
		}
	}
	return sb.String()
}

// base64Value maps one code unit to its six bits, accepting either alphabet, and reports
// false for anything else so the scanner can skip it.
func base64Value(u uint16) (uint32, bool) {
	switch {
	case u >= 'A' && u <= 'Z':
		return uint32(u - 'A'), true
	case u >= 'a' && u <= 'z':
		return uint32(u-'a') + 26, true
	case u >= '0' && u <= '9':
		return uint32(u-'0') + 52, true
	case u == '+' || u == '-':
		return 62, true
	case u == '/' || u == '_':
		return 63, true
	}
	return 0, false
}

// base64DecodeLenient is Node's base64 decode: it collects sextets, skipping anything
// outside the alphabet, stops at the first padding character, and emits whatever whole
// bytes it accumulated. A run of one leftover sextet carries no whole byte and is
// dropped, which is what makes a truncated input decode to the prefix that survives
// rather than throwing.
func base64DecodeLenient(units []uint16) []byte {
	out := make([]byte, 0, len(units)*3/4+3)
	var acc uint32
	bits := 0
	for _, u := range units {
		if u == '=' {
			break
		}
		v, ok := base64Value(u)
		if !ok {
			continue
		}
		acc = acc<<6 | v
		bits += 6
		if bits >= 8 {
			bits -= 8
			out = append(out, byte(acc>>uint(bits)))
		}
	}
	return out
}

// hexDecodeLenient is Node's hex decode: pairs from the front, stopping at the first
// code unit that is not a hex digit and dropping a trailing lone nibble.
func hexDecodeLenient(units []uint16) []byte {
	out := make([]byte, 0, len(units)/2)
	for i := 0; i+1 < len(units); i += 2 {
		hi, ok := hexNibble(units[i])
		if !ok {
			break
		}
		lo, ok := hexNibble(units[i+1])
		if !ok {
			break
		}
		out = append(out, hi<<4|lo)
	}
	return out
}

// bufferByteLength is Buffer.byteLength for a string: the number of bytes the encoding
// writes for it. The three fixed-width encodings are arithmetic over the code-unit
// count, and utf8 is the one that has to look at the string, since a code point takes
// one to four bytes.
func bufferByteLength(s BStr, enc string) int {
	switch enc {
	case "latin1", "ascii":
		return int(s.Length())
	case "utf16le":
		return int(s.Length()) * 2
	case "utf8":
		return len(s.ToGoString())
	}
	return len(bufferEncode(s, enc))
}

// utf8TrimPartial drops a truncated sequence off the end of a utf8 run. buf.write
// encodes the whole string and then takes the prefix that fits the window, and Node's
// utf8 write stops on a code-point boundary rather than leaving half a sequence in the
// buffer, so the utf8 case trims back to one. The fixed-width encodings do not trim:
// Node's utf16le write leaves a half-written code unit exactly where the window ends.
func utf8TrimPartial(b []byte) []byte {
	for len(b) > 0 && !utf8.Valid(b) {
		b = b[:len(b)-1]
	}
	return b
}
