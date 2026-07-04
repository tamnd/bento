package value

import "testing"

// TestBtoa checks the base64 encoder against the values Node prints, including the
// one, two, and three byte groups that fix the padding.
func TestBtoa(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"M", "TQ=="},
		{"Ma", "TWE="},
		{"Man", "TWFu"},
		{"hello", "aGVsbG8="},
	}
	for _, c := range cases {
		if got := Btoa(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("Btoa(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestAtob checks the forgiving-base64 decoder against Node: padded and unpadded
// input decode the same, and embedded ASCII whitespace is stripped before the
// decode.
func TestAtob(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", ""},
		{"TQ==", "M"},
		{"TWE=", "Ma"},
		{"TWFu", "Man"},
		{"TWE", "Ma"},
		{"aGVsbG8=", "hello"},
		{"aGVs bG8=", "hello"},
	}
	for _, c := range cases {
		if got := Atob(bs(c.in)).ToGoString(); got != c.want {
			t.Errorf("Atob(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestBase64RoundTrip proves atob reverses btoa for a binary string across the
// full byte range.
func TestBase64RoundTrip(t *testing.T) {
	units := make([]uint16, 256)
	for i := range units {
		units[i] = uint16(i)
	}
	s := FromUTF16(units)
	if got := Atob(Btoa(s)); !got.Equal(s) {
		t.Fatal("atob(btoa(s)) did not round-trip the full byte range")
	}
}

// TestBtoaNonLatin1Throws proves btoa raises an InvalidCharacterError on a code
// unit above the Latin1 range rather than truncating it to a byte.
func TestBtoaNonLatin1Throws(t *testing.T) {
	defer func() {
		r := recover()
		e, ok := r.(*Error)
		if !ok || !e.IsA("InvalidCharacterError") {
			t.Fatalf("expected an InvalidCharacterError, got %v", r)
		}
	}()
	Btoa(FromCharCode(0x100))
	t.Fatal("expected Btoa to throw")
}

// TestAtobInvalidThrows proves atob raises an InvalidCharacterError on input of
// the wrong length or with a character outside the base64 alphabet.
func TestAtobInvalidThrows(t *testing.T) {
	for _, in := range []string{"a", "!!!!", "ab=c", "=abc"} {
		func() {
			defer func() {
				r := recover()
				e, ok := r.(*Error)
				if !ok || !e.IsA("InvalidCharacterError") {
					t.Errorf("Atob(%q): expected an InvalidCharacterError, got %v", in, r)
				}
			}()
			Atob(bs(in))
			t.Errorf("Atob(%q): expected a throw", in)
		}()
	}
}
