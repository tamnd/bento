package value

import (
	"math"
	"testing"
)

func TestParseFloat(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		// A bare number, and one with a trailing tail that is ignored.
		{"3.14", 3.14},
		{"3.14 is pi", 3.14},
		{"42px", 42},
		{"1e3rest", 1000},
		{".5", 0.5},
		{"+.5", 0.5},
		{"-2.5", -2.5},
		// Only leading whitespace is trimmed.
		{"  17  ", 17},
		{"\t-3\n", -3},
		// Infinity as a prefix.
		{"Infinity", math.Inf(1)},
		{"-Infinity and beyond", math.Inf(-1)},
		// A dangling exponent marker is left out of the prefix, so the mantissa parses.
		{"1e", 1},
		{"1e+", 1},
		{"2.5eabc", 2.5},
		// parseFloat does not read the radix forms: it takes the leading 0 and stops.
		{"0x1F", 0},
		{"0b101", 0},
		// A large exponent overflows to infinity, a tiny one underflows to zero.
		{"1e999", math.Inf(1)},
		{"1e-999", 0},
	}
	for _, c := range cases {
		if got := ParseFloat(FromGoString(c.in)); got != c.want {
			t.Errorf("ParseFloat(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestParseFloatNaN(t *testing.T) {
	// No leading decimal prefix, so parseFloat is NaN.
	cases := []string{"", "abc", ".", "+", "-", "e3", ".e5", "  x", "+.", "Inf"}
	for _, in := range cases {
		if got := ParseFloat(FromGoString(in)); !math.IsNaN(got) {
			t.Errorf("ParseFloat(%q) = %v, want NaN", in, got)
		}
	}
}
