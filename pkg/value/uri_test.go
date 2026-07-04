package value

import "testing"

// bs is a shorthand for a string from Go source, used to keep the URI test
// tables readable.
func bs(s string) BStr { return FromGoString(s) }

// TestEncodeURIComponent checks the encoder against the values Node prints: the
// unreserved set passes through, reserved and space bytes percent-encode, and a
// multibyte or astral code point encodes its UTF-8 bytes.
func TestEncodeURIComponent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello world", "hello%20world"},
		{"a+b=c", "a%2Bb%3Dc"},
		{"-_.!~*'()", "-_.!~*'()"},
		{"A-Za-z0-9", "A-Za-z0-9"},
		{"café", "caf%C3%A9"},
		{"日本", "%E6%97%A5%E6%9C%AC"},
		{"😀", "%F0%9F%98%80"},
		{"100%", "100%25"},
	}
	for _, c := range cases {
		if got := EncodeURIComponent(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("EncodeURIComponent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDecodeURIComponent checks the decoder round-trips the encoder's output and
// resolves the multibyte escapes back to their code points.
func TestDecodeURIComponent(t *testing.T) {
	cases := []struct{ in, want string }{
		{"hello%20world", "hello world"},
		{"a%2Bb%3Dc", "a+b=c"},
		{"caf%C3%A9", "café"},
		{"%E6%97%A5%E6%9C%AC", "日本"},
		{"%F0%9F%98%80", "😀"},
		{"plain", "plain"},
		{"%25", "%"},
	}
	for _, c := range cases {
		if got := DecodeURIComponent(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("DecodeURIComponent(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEncodeURIComponentLoneSurrogateThrows proves the encoder raises a URIError
// on a lone surrogate rather than emitting a replacement character.
func TestEncodeURIComponentLoneSurrogateThrows(t *testing.T) {
	defer func() {
		r := recover()
		e, ok := r.(*Error)
		if !ok || !e.IsA("URIError") {
			t.Fatalf("expected a URIError, got %v", r)
		}
	}()
	// a single high surrogate with no following low surrogate
	EncodeURIComponent(FromCharCode(0xD800))
	t.Fatal("expected EncodeURIComponent to throw")
}

// TestDecodeURIComponentMalformedThrows proves the decoder raises a URIError on
// each shape of malformed escape: a truncated %, a non-hex digit, and an
// incomplete UTF-8 escape run.
func TestDecodeURIComponentMalformedThrows(t *testing.T) {
	for _, in := range []string{"%", "%2", "%G0", "%E6%97", "abc%"} {
		func() {
			defer func() {
				r := recover()
				e, ok := r.(*Error)
				if !ok || !e.IsA("URIError") {
					t.Errorf("DecodeURIComponent(%q): expected a URIError, got %v", in, r)
				}
			}()
			DecodeURIComponent(bs(in))
			t.Errorf("DecodeURIComponent(%q): expected a throw", in)
		}()
	}
}
