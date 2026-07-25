package value

import "testing"

// TestEncodeGivesUTF8Bytes pins the encoder over the three widths a UTF-8 sequence
// takes plus a surrogate pair, the four cases the transcode has.
func TestEncodeGivesUTF8Bytes(t *testing.T) {
	enc := NewTextEncoder()
	if got := enc.Encoding().ToGoString(); got != "utf-8" {
		t.Errorf("encoding = %q, want %q", got, "utf-8")
	}
	cases := []struct {
		in   string
		want []byte
	}{
		{"", nil},
		{"abc", []byte{'a', 'b', 'c'}},
		{"é", []byte{0xC3, 0xA9}},
		{"€", []byte{0xE2, 0x82, 0xAC}},
		{"😀", []byte{0xF0, 0x9F, 0x98, 0x80}},
	}
	for _, c := range cases {
		got := enc.Encode(FromGoString(c.in)).Bytes()
		if string(got) != string(c.want) {
			t.Errorf("encode(%q) = % x, want % x", c.in, got, c.want)
		}
	}
}

// TestEncodeReplacesALoneSurrogate pins the one lossy case: a string holding an
// unpaired surrogate has no UTF-8 spelling, so it encodes as the replacement.
func TestEncodeReplacesALoneSurrogate(t *testing.T) {
	lone := FromUTF16([]uint16{'a', 0xD800, 'b'})
	got := NewTextEncoder().Encode(lone).Bytes()
	want := []byte{'a', 0xEF, 0xBF, 0xBD, 'b'}
	if string(got) != string(want) {
		t.Errorf("encode(lone surrogate) = % x, want % x", got, want)
	}
}

// TestDecodeRoundTripsTheEncoder pins the pair: every string the encoder writes, the
// decoder reads back unchanged.
func TestDecodeRoundTripsTheEncoder(t *testing.T) {
	enc, dec := NewTextEncoder(), NewTextDecoder()
	for _, s := range []string{"", "abc", "héllo", "€ and 😀", "日本語"} {
		if got := dec.Decode(enc.Encode(FromGoString(s))).ToGoString(); got != s {
			t.Errorf("round trip of %q gave %q", s, got)
		}
	}
}

// TestDecodeReplacesMalformedBytes pins the recovery rule: a malformed prefix consumes
// one byte and emits one replacement, so the bytes after it still decode.
func TestDecodeReplacesMalformedBytes(t *testing.T) {
	dec := NewTextDecoder()
	got := dec.Decode(Uint8ArrayFromGo([]byte{'a', 0xFF, 'b'})).ToGoString()
	if got != "a�b" {
		t.Errorf("decode of a bad byte = %q, want %q", got, "a�b")
	}
	// A truncated multi-byte sequence is malformed the same way.
	got = dec.Decode(Uint8ArrayFromGo([]byte{0xE2, 0x82})).ToGoString()
	if got != "��" {
		t.Errorf("decode of a truncated sequence = %q, want %q", got, "��")
	}
}

// TestFatalDecoderThrows pins the fatal flag: the same bytes that become replacements
// for an ordinary decoder raise a TypeError for a fatal one.
func TestFatalDecoderThrows(t *testing.T) {
	dec := NewTextDecoderWithOptions(FromGoString("utf-8"), true, false)
	defer func() {
		if recover() == nil {
			t.Error("a fatal decoder did not throw on malformed bytes")
		}
	}()
	dec.Decode(Uint8ArrayFromGo([]byte{0xFF}))
}

// TestDecoderStripsTheByteOrderMark pins the BOM rule and the flag that turns it off.
func TestDecoderStripsTheByteOrderMark(t *testing.T) {
	bom := Uint8ArrayFromGo([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'})
	if got := NewTextDecoder().Decode(bom).ToGoString(); got != "hi" {
		t.Errorf("decode with a BOM = %q, want %q", got, "hi")
	}
	kept := NewTextDecoderWithOptions(FromGoString("utf-8"), false, true)
	if got := kept.Decode(bom).ToGoString(); got != "\uFEFFhi" {
		t.Errorf("decode with ignoreBOM = %q, want %q", got, "\uFEFFhi")
	}
}

// TestDecoderReadsTheOtherHostedEncodings pins utf-16le and latin1, the two labels
// beyond utf-8 the decoder claims, including the odd trailing byte utf-16le can end on.
func TestDecoderReadsTheOtherHostedEncodings(t *testing.T) {
	le := NewTextDecoder(FromGoString("utf-16le"))
	if got := le.Encoding().ToGoString(); got != "utf-16le" {
		t.Errorf("encoding = %q, want %q", got, "utf-16le")
	}
	if got := le.Decode(Uint8ArrayFromGo([]byte{'h', 0, 'i', 0})).ToGoString(); got != "hi" {
		t.Errorf("utf-16le decode = %q, want %q", got, "hi")
	}
	if got := le.Decode(Uint8ArrayFromGo([]byte{'h', 0, 'i'})).ToGoString(); got != "h�" {
		t.Errorf("utf-16le odd trailing byte = %q, want %q", got, "h�")
	}
	l1 := NewTextDecoder(FromGoString("ISO-8859-1"))
	if got := l1.Encoding().ToGoString(); got != "latin1" {
		t.Errorf("encoding = %q, want %q", got, "latin1")
	}
	if got := l1.Decode(Uint8ArrayFromGo([]byte{0xE9, 'a'})).ToGoString(); got != "éa" {
		t.Errorf("latin1 decode = %q, want %q", got, "éa")
	}
}

// TestUnknownLabelThrows pins the honest answer for a label no hosted encoding claims:
// a RangeError, not a quiet fall back to utf-8 that would read the bytes as the wrong
// text.
func TestUnknownLabelThrows(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("an unsupported encoding label did not throw")
		}
	}()
	NewTextDecoder(FromGoString("shift_jis"))
}
