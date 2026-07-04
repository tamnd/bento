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

// TestEncodeURI checks the whole-URI encoder against Node: the reserved
// delimiters that structure a URI pass through where encodeURIComponent would
// escape them, and a space or a multibyte code point still escapes.
func TestEncodeURI(t *testing.T) {
	cases := []struct{ in, want string }{
		{";,/?:@&=+$#", ";,/?:@&=+$#"},
		{"a b", "a%20b"},
		{"http://a.b/c d?x=café&y=z#f", "http://a.b/c%20d?x=caf%C3%A9&y=z#f"},
		{"unreserved-_.!~*'()", "unreserved-_.!~*'()"},
	}
	for _, c := range cases {
		if got := EncodeURI(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("EncodeURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestDecodeURI checks the whole-URI decoder against Node: a non-reserved escape
// decodes, but an escape that names a reserved delimiter is left as its literal
// %XX, the rule that keeps a decoded URI's structure intact.
func TestDecodeURI(t *testing.T) {
	cases := []struct{ in, want string }{
		{"http://a.b/c%20d?x=caf%C3%A9&y=z#f", "http://a.b/c d?x=café&y=z#f"},
		{"%3Bx%2Fy", "%3Bx%2Fy"},
		{"a%20b", "a b"},
		{"caf%C3%A9", "café"},
	}
	for _, c := range cases {
		if got := DecodeURI(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("DecodeURI(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestEncodeURIReservedDiffersFromComponent pins the one behavioral split between
// the two encoders: encodeURI keeps the reserved delimiters, encodeURIComponent
// escapes them.
func TestEncodeURIReservedDiffersFromComponent(t *testing.T) {
	const reserved = ";,/?:@&=+$#"
	if got := EncodeURI(bs(reserved)).ToGoString(); got != reserved {
		t.Fatalf("EncodeURI(%q) = %q, want it unchanged", reserved, got)
	}
	const want = "%3B%2C%2F%3F%3A%40%26%3D%2B%24%23"
	if got := EncodeURIComponent(bs(reserved)).ToGoString(); got != want {
		t.Fatalf("EncodeURIComponent(%q) = %q, want %q", reserved, got, want)
	}
}

// TestDecodeURIMalformedThrows proves the whole-URI decoder raises a URIError on
// the same malformed escapes the component decoder rejects, since both share the
// escape parse.
func TestDecodeURIMalformedThrows(t *testing.T) {
	for _, in := range []string{"%", "%2", "%G0", "%E6%97", "abc%"} {
		func() {
			defer func() {
				r := recover()
				e, ok := r.(*Error)
				if !ok || !e.IsA("URIError") {
					t.Errorf("DecodeURI(%q): expected a URIError, got %v", in, r)
				}
			}()
			DecodeURI(bs(in))
			t.Errorf("DecodeURI(%q): expected a throw", in)
		}()
	}
}
