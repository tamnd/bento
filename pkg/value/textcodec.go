package value

// This file gives the AOT runtime the TextEncoder and TextDecoder pair, the codec
// Node and the web platform expose as globals and the canonical way a JavaScript
// program crosses between a string and bytes. Buffer's utf8 paths delegate to them,
// so they are the layer underneath the Buffer surface rather than a wrapper over it,
// which is also why they implement the transcode directly here instead of routing
// through any other runtime type.
//
// Unlike the Event and AbortController pair, which are boxed property bags a program
// reads through the dynamic path, these are ordinary Go types the compiler proves.
// That is forced by the shape of the surface: encode returns a Uint8Array, which is a
// typed-world value with no boxed kind of its own, so an encoder that lived in a
// value.Value could not hand one back. The compiler knows the receiver at every call
// site, so the monomorphic form is also the direct one.

import "unicode/utf8"

// The encoding labels a TextDecoder accepts. The WHATWG encoding registry lists many
// more, but these three are the ones Node's own code and the buffer surface reach for,
// each with a transcode short enough to state exactly. A label outside the set throws
// the way the specification requires rather than quietly decoding as something else.
const (
	encUTF8    = "utf-8"
	encUTF16LE = "utf-16le"
	encLatin1  = "latin1"
)

// TextEncoder is the string-to-UTF-8 half of the codec, the lowering of
// new TextEncoder(). It carries no state: the specification fixes its encoding at
// utf-8, so every encoder is the same encoder and the type is an empty struct held by
// pointer for the reference identity a JavaScript object has.
type TextEncoder struct{}

// NewTextEncoder builds a TextEncoder. It takes no argument, matching the one
// constructor the specification gives: an encoder is always utf-8.
func NewTextEncoder() *TextEncoder { return &TextEncoder{} }

// Encoding is the encoder's fixed label, the lowering of the encoding getter. It is
// always "utf-8".
func (e *TextEncoder) Encoding() BStr { return FromGoString(encUTF8) }

// Encode transcodes a string to its UTF-8 bytes, the lowering of encoder.encode(input).
// A string that holds a lone surrogate has no UTF-8 spelling, so each unpaired
// surrogate becomes the U+FFFD replacement, which is what ToGoString already does and
// what the specification requires of the encoder.
func (e *TextEncoder) Encode(s BStr) *Uint8Array {
	return Uint8ArrayFromGo([]byte(s.ToGoString()))
}

// TextDecoder is the bytes-to-string half of the codec, the lowering of
// new TextDecoder(label). It carries the encoding it was built with and the two flags
// the constructor's options set, each readable back through its getter.
type TextDecoder struct {
	encoding  string
	fatal     bool
	ignoreBOM bool
}

// NewTextDecoder builds a TextDecoder for a label, the lowering of
// new TextDecoder() and new TextDecoder(label). An absent label is utf-8, the default
// the specification gives. The label is matched case-insensitively against the aliases
// the registry lists for each encoding, and a label naming an encoding this runtime
// does not host throws a RangeError, the specification's answer for an unsupported
// label. Silently decoding as utf-8 instead would hand the program bytes read as the
// wrong text, which is the kind of quiet wrong answer a compiler must not give.
func NewTextDecoder(label ...BStr) *TextDecoder {
	enc := encUTF8
	if len(label) > 0 {
		name, ok := normalizeEncodingLabel(label[0].ToGoString())
		if !ok {
			Throw(NewRangeError(Concat(FromGoString("The \"encoding\" argument is invalid: "), label[0])))
		}
		enc = name
	}
	return &TextDecoder{encoding: enc}
}

// NewTextDecoderWithOptions builds a TextDecoder for a label and the fatal and
// ignoreBOM flags an options dictionary carries, the lowering of
// new TextDecoder(label, { fatal, ignoreBOM }). The flags arrive already coerced to
// booleans, since the caller read them off the dictionary at the call site.
func NewTextDecoderWithOptions(label BStr, fatal, ignoreBOM bool) *TextDecoder {
	d := NewTextDecoder(label)
	d.fatal = fatal
	d.ignoreBOM = ignoreBOM
	return d
}

// normalizeEncodingLabel maps a label to the encoding it names, reporting false for a
// label no hosted encoding claims. The comparison is over the lowercased label, the
// case-insensitive match the registry specifies.
func normalizeEncodingLabel(label string) (string, bool) {
	switch lowerASCII(label) {
	case "utf-8", "utf8", "unicode-1-1-utf-8":
		return encUTF8, true
	case "utf-16le", "utf-16", "utf16le", "unicodefeff":
		return encUTF16LE, true
	case "latin1", "iso-8859-1", "iso8859-1", "windows-1252", "ascii", "us-ascii":
		return encLatin1, true
	}
	return "", false
}

// lowerASCII lowercases the ASCII letters of a label. An encoding label is ASCII by
// construction, so this is the whole of the case folding the match needs and it avoids
// pulling the Unicode tables in for a handful of names.
func lowerASCII(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}

// Encoding is the label the decoder was built with, the lowering of the encoding
// getter. It is the canonical name, not the alias the constructor was given, matching
// what the specification reports.
func (d *TextDecoder) Encoding() BStr { return FromGoString(d.encoding) }

// Fatal reports whether a malformed sequence throws rather than becoming U+FFFD, the
// lowering of the fatal getter.
func (d *TextDecoder) Fatal() bool { return d.fatal }

// IgnoreBOM reports whether a leading byte order mark is kept as a character rather
// than stripped, the lowering of the ignoreBOM getter.
func (d *TextDecoder) IgnoreBOM() bool { return d.ignoreBOM }

// Decode transcodes bytes to a string, the lowering of decoder.decode(input). A
// leading byte order mark is stripped unless ignoreBOM was set, and a malformed
// sequence becomes the U+FFFD replacement, or throws a TypeError when the decoder is
// fatal. Decoding an absent input gives the empty string, the answer decode() with no
// argument has.
func (d *TextDecoder) Decode(a *Uint8Array) BStr {
	if a == nil {
		return FromGoString("")
	}
	switch d.encoding {
	case encUTF16LE:
		return d.decodeUTF16LE(a.Bytes())
	case encLatin1:
		return d.decodeLatin1(a.Bytes())
	default:
		return d.decodeUTF8(a.Bytes())
	}
}

// decodeUTF8 walks the bytes a rune at a time. Go's decoder reports an invalid
// sequence as RuneError with a width of one, which is exactly the specification's
// rule that a malformed prefix consumes one byte and emits one replacement, so the
// loop needs no separate resynchronization. A fatal decoder throws on the first such
// byte instead of emitting the replacement.
func (d *TextDecoder) decodeUTF8(b []byte) BStr {
	if !d.ignoreBOM && len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		b = b[3:]
	}
	if utf8.Valid(b) {
		return FromGoString(string(b))
	}
	out := make([]rune, 0, len(b))
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size <= 1 {
			if d.fatal {
				Throw(NewTypeError(FromGoString("The encoded data was not valid for encoding utf-8")))
			}
			out = append(out, '�')
			i++
			continue
		}
		out = append(out, r)
		i += size
	}
	return FromGoString(string(out))
}

// decodeUTF16LE reads the bytes as little-endian code units. The units are handed to
// the string as they are, without pairing the surrogates, because a JavaScript string
// is a code-unit sequence and a lone surrogate is a value it can hold; pairing here
// would be a round trip through runes that changes nothing for well-formed input and
// loses information for the rest. A trailing odd byte is an incomplete unit, so it
// becomes the replacement, or throws when the decoder is fatal.
func (d *TextDecoder) decodeUTF16LE(b []byte) BStr {
	units := make([]uint16, 0, len(b)/2+1)
	i := 0
	if !d.ignoreBOM && len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE {
		i = 2
	}
	for ; i+1 < len(b); i += 2 {
		units = append(units, uint16(b[i])|uint16(b[i+1])<<8)
	}
	if i < len(b) {
		if d.fatal {
			Throw(NewTypeError(FromGoString("The encoded data was not valid for encoding utf-16le")))
		}
		units = append(units, 0xFFFD)
	}
	return FromUTF16(units)
}

// decodeLatin1 reads each byte as the code point of the same number, the whole of the
// single-byte encoding: every byte is a valid character, so there is no malformed case
// and the fatal flag never fires.
func (d *TextDecoder) decodeLatin1(b []byte) BStr {
	units := make([]uint16, len(b))
	for i, c := range b {
		units[i] = uint16(c)
	}
	return FromUTF16(units)
}
