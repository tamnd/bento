package value

import "testing"

// TestJSONStringifyScalars checks the leaf encodings against the exact text V8
// produces, since these are the arms every larger shape is built from.
func TestJSONStringifyScalars(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string", FromGoString("hi"), `"hi"`},
		{"emptyString", FromGoString(""), `""`},
		{"true", true, "true"},
		{"false", false, "false"},
		{"zero", float64(0), "0"},
		{"negZero", negZero(), "0"},
		{"int", float64(42), "42"},
		{"neg", float64(-7), "-7"},
		{"frac", float64(1) / float64(7), "0.14285714285714285"},
		{"nan", nan(), "null"},
		{"posInf", inf(1), "null"},
		{"negInf", inf(-1), "null"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JSONStringify(c.in).ToGoString()
			if got != c.want {
				t.Fatalf("JSONStringify(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestJSONStringifyStringEscapes checks that string escaping matches the
// specification's well-formed JSON.stringify: the two structural characters, the
// short control escapes, the \u form for the other control characters, and a
// lone surrogate escaped while a valid pair and a plain non-ASCII rune are left
// literal. V8 does not escape <, >, or &, so those stay literal too.
func TestJSONStringifyStringEscapes(t *testing.T) {
	cases := []struct {
		name string
		in   BStr
		want string
	}{
		{"quote", FromGoString("a\"b"), "\"a\\\"b\""},
		{"backslash", FromGoString("a\\b"), "\"a\\\\b\""},
		{"newline", FromGoString("a\nb"), "\"a\\nb\""},
		{"tab", FromGoString("a\tb"), "\"a\\tb\""},
		{"return", FromGoString("a\rb"), "\"a\\rb\""},
		{"backspace", FromGoString("a\bb"), "\"a\\bb\""},
		{"formfeed", FromGoString("a\fb"), "\"a\\fb\""},
		{"unitSep", FromGoString("a\x1fb"), "\"a\\u001fb\""},
		{"nul", FromGoString("a\x00b"), "\"a\\u0000b\""},
		{"angles", FromGoString("<a>&b"), "\"<a>&b\""},
		{"nonASCII", FromGoString("café"), "\"café\""},
		{"astral", FromGoString("😀"), "\"😀\""},
		{"loneHigh", FromUTF16([]uint16{'a', 0xD800, 'b'}), "\"a\\ud800b\""},
		{"loneLow", FromUTF16([]uint16{'a', 0xDC00, 'b'}), "\"a\\udc00b\""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := JSONStringify(c.in).ToGoString()
			if got != c.want {
				t.Fatalf("JSONStringify(%q) = %q, want %q", c.in.ToGoString(), got, c.want)
			}
		})
	}
}

// TestJSONStringifyArray checks that an array serializes to a bracketed list with
// no spaces, recursing into each element, so the array arm and the recursion are
// both exercised.
func TestJSONStringifyArray(t *testing.T) {
	arr := NewArray(FromGoString("a"), FromGoString("b"), FromGoString("c"))
	if got := JSONStringify(arr).ToGoString(); got != `["a","b","c"]` {
		t.Fatalf("string array = %q", got)
	}
	nums := NewArray(float64(1), float64(2), float64(3))
	if got := JSONStringify(nums).ToGoString(); got != `[1,2,3]` {
		t.Fatalf("number array = %q", got)
	}
	empty := NewArray[float64]()
	if got := JSONStringify(empty).ToGoString(); got != `[]` {
		t.Fatalf("empty array = %q", got)
	}
}

func nan() float64 { z := float64(0); return z / z }

func inf(s int) float64 {
	z := float64(0)
	if s < 0 {
		return -1 / z
	}
	return 1 / z
}

func negZero() float64 { z := float64(0); return -z }
