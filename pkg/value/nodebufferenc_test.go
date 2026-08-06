package value

import (
	"testing"
)

// These cover the encoding layer directly, below the member surface, because every Buffer
// entry point that takes or answers a string funnels through it and a fault here would
// surface far from its cause. Every expectation is the output of node v24.11.0 on the
// same input.

// TestAnEncodingNameIsCanonicalized covers the name table. Node accepts a handful of
// spellings for most encodings and matches them without regard to case, and rejects
// everything else with a named error rather than falling back to utf8.
func TestAnEncodingNameIsCanonicalized(t *testing.T) {
	for _, c := range []struct{ name, want string }{
		{"utf8", "utf8"}, {"utf-8", "utf8"}, {"UTF-8", "utf8"}, {"Utf8", "utf8"},
		{"utf16le", "utf16le"}, {"utf-16le", "utf16le"}, {"UTF-16LE", "utf16le"},
		{"ucs2", "utf16le"}, {"ucs-2", "utf16le"}, {"UCS2", "utf16le"},
		{"latin1", "latin1"}, {"binary", "latin1"}, {"BINARY", "latin1"},
		{"ascii", "ascii"}, {"ASCII", "ascii"},
		{"base64", "base64"}, {"base64url", "base64url"}, {"BASE64URL", "base64url"},
		{"hex", "hex"}, {"HEX", "hex"},
	} {
		got, ok := bufferEncoding(c.name)
		if !ok || got != c.want {
			t.Errorf("bufferEncoding(%q) = %q %v, want %q true", c.name, got, ok, c.want)
		}
	}
	for _, name := range []string{"nope", "utf7", "utf-16be", "", "utf 8", "base 64"} {
		if got, ok := bufferEncoding(name); ok {
			t.Errorf("bufferEncoding(%q) = %q true, want a miss", name, got)
		}
	}
}

// TestAnOmittedEncodingIsUtf8 covers the argument reader. An absent encoding is utf8, and
// a name that is not one raises the error node raises, with the code a program branches
// on rather than a bare TypeError.
func TestAnOmittedEncodingIsUtf8(t *testing.T) {
	if got := bufferEncodingArg(Undefined); got != "utf8" {
		t.Errorf("an omitted encoding = %q, want utf8", got)
	}
	if got := bufferEncodingArg(Null); got != "utf8" {
		t.Errorf("a null encoding = %q, want utf8", got)
	}
	code, msg := catchThrownCode(func() { bufferEncodingArg(StringValue(FromGoString("nope"))) })
	if code != "ERR_UNKNOWN_ENCODING" || msg != "Unknown encoding: nope" {
		t.Errorf("an unknown encoding threw %q %q, want ERR_UNKNOWN_ENCODING", code, msg)
	}
}

// TestAStringEncodesToTheBytesNodeWrites holds each encoding to the bytes node produces
// for the same string. The multi-byte cases are the ones worth pinning: utf8 widens a
// non-ASCII code point, latin1 and ascii both truncate a code unit to its low byte on the
// way in and differ only on the way out, and utf16le writes each unit little-endian.
func TestAStringEncodesToTheBytesNodeWrites(t *testing.T) {
	for _, c := range []struct {
		in, enc string
		want    string
	}{
		{"hello", "utf8", "68656c6c6f"},
		{"héllo", "utf8", "68c3a96c6c6f"},
		{"€uro", "utf8", "e282ac75726f"},
		{"😀", "utf8", "f09f9880"},
		{"héllo", "latin1", "68e96c6c6f"},
		// ascii encodes exactly as latin1 does. The mask is applied on the way out, not in,
		// which is a thing worth holding: the two differ only when a byte is above 0x7f.
		{"héllo", "ascii", "68e96c6c6f"},
		{"ab", "utf16le", "61006200"},
		{"€", "utf16le", "ac20"},
		{"aGVsbG8=", "base64", "68656c6c6f"},
		{"-_8", "base64", "fbff"},
		{"+/8=", "base64url", "fbff"},
		{"48656c6c6f", "hex", "48656c6c6f"},
		{"", "utf8", ""},
	} {
		got := bufferDecode(bufferEncode(FromGoString(c.in), c.enc), "hex").ToGoString()
		if got != c.want {
			t.Errorf("encode(%q, %s) = %s, want %s", c.in, c.enc, got, c.want)
		}
	}
	// A lone surrogate is not a code point, so node writes the replacement character for it
	// rather than the unpaired unit or nothing at all. It is spelled with its code unit
	// because Go rejects a surrogate in a string literal outright.
	lone := FromUTF16([]uint16{0xD800})
	if got := bufferDecode(bufferEncode(lone, "utf8"), "hex").ToGoString(); got != "efbfbd" {
		t.Errorf("encode of a lone surrogate as utf8 = %s, want efbfbd", got)
	}
	// The two fixed-width encodings do not repair it, since neither reads a surrogate pair
	// as a pair in the first place: latin1 keeps the low byte and utf16le keeps the unit.
	if got := bufferDecode(bufferEncode(lone, "utf16le"), "hex").ToGoString(); got != "00d8" {
		t.Errorf("encode of a lone surrogate as utf16le = %s, want 00d8", got)
	}
}

// TestBytesDecodeToTheStringNodeReads is the other direction. The two that are not a
// straight inverse are the point: ascii masks the high bit off every byte as it reads, so
// it is lossy where latin1 is not, and utf8 replaces a byte run that is not a valid
// sequence rather than refusing.
func TestBytesDecodeToTheStringNodeReads(t *testing.T) {
	for _, c := range []struct {
		hex, enc, want string
	}{
		{"68656c6c6f", "utf8", "hello"},
		{"68c3a96c6c6f", "utf8", "héllo"},
		{"ff41", "utf8", "�A"},
		{"68e96c6c6f", "latin1", "héllo"},
		{"e941", "ascii", "iA"},
		{"e941", "latin1", "éA"},
		{"61006200", "utf16le", "ab"},
		// An odd trailing byte cannot make a unit, so node drops it rather than padding.
		{"610062006300", "utf16le", "abc"},
		{"6100620063", "utf16le", "ab"},
		{"68656c6c6f", "base64", "aGVsbG8="},
		{"fbff", "base64", "+/8="},
		{"fbff", "base64url", "-_8"},
		{"66", "base64", "Zg=="},
		{"666f", "base64", "Zm8="},
		{"", "base64", ""},
		{"68656c6c6f", "hex", "68656c6c6f"},
	} {
		in := bufferEncode(FromGoString(c.hex), "hex")
		if got := bufferDecode(in, c.enc).ToGoString(); got != c.want {
			t.Errorf("decode(%s, %s) = %q, want %q", c.hex, c.enc, got, c.want)
		}
	}
}

// TestABadlySpelledPayloadDecodesLeniently covers the two forgiving decoders. Node does
// not reject a base64 string with missing padding, stray whitespace, or a mix of the two
// alphabets, and it does not reject hex with a trailing nibble or trailing garbage: both
// take what they can read and stop. A program that round-trips its own data never sees
// this, and a program parsing something it was handed depends on it.
func TestABadlySpelledPayloadDecodesLeniently(t *testing.T) {
	for _, c := range []struct{ in, enc, want string }{
		{"aGVsbG8=", "base64", "hello"},
		{"aGVsbG8", "base64", "hello"},
		{"a GV s bG8", "base64", "hello"},
		{"aGVs\nbG8=", "base64", "hello"},
		{"48656c6c6f", "hex", "Hello"},
		// The scan stops at the first byte that is not hex, so what came before it survives
		// and what came after is dropped rather than shifting everything.
		{"48656c6c6fzz99", "hex", "Hello"},
		// An odd count leaves a nibble with no partner, and a half byte is not a byte.
		{"616263", "hex", "abc"},
		{"6162630", "hex", "abc"},
		{"zz", "hex", ""},
	} {
		got := bufferDecode(bufferEncode(FromGoString(c.in), c.enc), "utf8").ToGoString()
		if got != c.want {
			t.Errorf("decode of %q as %s = %q, want %q", c.in, c.enc, got, c.want)
		}
	}
}

// TestByteLengthCountsWithoutEncoding covers Buffer.byteLength's arithmetic. Every
// encoding but utf8 has a fixed ratio to the code-unit count, so the answer is computed
// rather than produced by encoding and measuring, and the two must agree.
func TestByteLengthCountsWithoutEncoding(t *testing.T) {
	for _, c := range []struct {
		in, enc string
		want    int
	}{
		{"héllo", "utf8", 6},
		{"€uro", "utf8", 6},
		{"héllo", "latin1", 5},
		{"héllo", "ascii", 5},
		{"héllo", "utf16le", 10},
		{"aGVsbG8=", "base64", 5},
		{"aGVsbG8", "base64", 5},
		{"4865", "hex", 2},
		{"", "utf8", 0},
	} {
		if got := bufferByteLength(FromGoString(c.in), c.enc); got != c.want {
			t.Errorf("byteLength(%q, %s) = %v, want %v", c.in, c.enc, got, c.want)
		}
		// The count is what an encode of the same string actually produces, which is the
		// property the arithmetic exists to shortcut.
		if got := len(bufferEncode(FromGoString(c.in), c.enc)); got != c.want {
			t.Errorf("encode(%q, %s) produced %v bytes, byteLength says %v", c.in, c.enc, got, c.want)
		}
	}
}

// TestAWriteStopsOnACodePointBoundary covers the trim a short buffer needs. A utf8 write
// into a buffer with no room for the last code point writes the code points that do fit
// and leaves the rest of the room untouched, rather than writing a fragment of a sequence
// that would read back as a replacement character.
func TestAWriteStopsOnACodePointBoundary(t *testing.T) {
	// "€uro" is e2 82 ac 75 72 6f, so four bytes hold the euro sign and the u exactly.
	full := bufferEncode(FromGoString("€uro"), "utf8")
	for _, c := range []struct {
		room int
		want string
	}{
		{6, "e282ac75726f"},
		{5, "e282ac7572"},
		{4, "e282ac75"},
		// Three bytes hold the euro sign and nothing else; two hold none of it, because two
		// thirds of a sequence is not a code point.
		{3, "e282ac"},
		{2, ""},
		{1, ""},
		{0, ""},
	} {
		n := c.room
		if n > len(full) {
			n = len(full)
		}
		got := bufferDecode(utf8TrimPartial(full[:n]), "hex").ToGoString()
		if got != c.want {
			t.Errorf("a write of \"€uro\" into %v bytes wrote %s, want %s", c.room, got, c.want)
		}
	}
}

// TestUtf8LossyReplacesEachMaximalSubpart covers the decoder's repair. What it is really
// checking is the count: a program reads the length of what it decoded, so how many
// replacement characters a bad run costs is observable, and the rule is one per maximal
// subpart rather than one per byte. Every expectation is node's own answer for the same
// bytes.
func TestUtf8LossyReplacesEachMaximalSubpart(t *testing.T) {
	for _, c := range []struct {
		in   []byte
		want string
	}{
		{[]byte("hello"), "hello"},
		{[]byte{0xe2, 0x82, 0xac}, "€"},
		{nil, ""},
		// A byte that starts nothing is one mistake each.
		{[]byte{0xff}, "�"},
		{[]byte{0xff, 0xfe}, "��"},
		{[]byte{0x41, 0xff, 0x42}, "A�B"},
		{[]byte{0x80}, "�"},
		{[]byte{0xf5}, "�"},
		// A truncated sequence is one mistake however many bytes of it arrived.
		{[]byte{0xc2}, "�"},
		{[]byte{0xe2, 0x82}, "�"},
		{[]byte{0xe0, 0xa0}, "�"},
		{[]byte{0xf0, 0x9f}, "�"},
		// A start byte followed by something that could not continue it ends the mistake
		// there, so what follows is decoded on its own terms rather than swallowed.
		{[]byte{0xc2, 0x41}, "�A"},
		{[]byte{0xe2, 0x41}, "�A"},
		{[]byte{0xf0, 0x9f, 0x41}, "�A"},
		{[]byte{0xf0, 0x9f, 0x98, 0x41}, "�A"},
		// An overlong encoding and a surrogate are rejected at the second byte, which is
		// what makes each of these three separate mistakes rather than one.
		{[]byte{0xe0, 0x80, 0x80}, "���"},
		{[]byte{0xed, 0xa0, 0x80}, "���"},
		{[]byte{0xed, 0xbf, 0xbf, 0x41}, "���A"},
		{[]byte{0xc0, 0x80}, "��"},
		{[]byte{0xf0, 0x80, 0x80, 0x80}, "����"},
		{[]byte{0xf4, 0x90, 0x80, 0x80}, "����"},
	} {
		if got := utf8Lossy(c.in); got != c.want {
			t.Errorf("utf8Lossy(% x) = %q (%v runes), want %q (%v)", c.in, got, len([]rune(got)), c.want, len([]rune(c.want)))
		}
	}
}
