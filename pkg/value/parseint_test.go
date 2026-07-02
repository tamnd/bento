package value

import (
	"math"
	"testing"
)

func TestParseInt(t *testing.T) {
	cases := []struct {
		in    string
		radix float64
		want  float64
	}{
		// Radix 0 stands for an omitted argument: base 10 with 0x detection.
		{"42", 0, 42},
		{"42px", 0, 42},
		{"  -17  ", 0, -17},
		{"+8", 0, 8},
		{"0x1F", 0, 31},  // the 0x prefix is detected and read as base 16
		{"0Xein", 0, 14}, // uppercase prefix, then the "e" is a base-16 digit
		{"010", 0, 10},   // no legacy octal: a leading zero is still base 10
		{"3.14", 0, 3},   // the fraction stops the parse at the point
		// An explicit radix.
		{"101", 2, 5},
		{"777", 8, 511},
		{"ff", 16, 255},
		{"FF", 16, 255},
		{"z", 36, 35},
		{"zz", 36, 1295},
		{"0x1F", 16, 31}, // base 16 also strips the prefix
		{"0x1F", 10, 0},  // base 10 does not: reads "0", stops at "x"
		{"deadBEEF", 16, 3735928559},
		// A fractional radix is truncated by ToInt32.
		{"101", 2.9, 5},
	}
	for _, c := range cases {
		got := ParseInt(FromGoString(c.in), c.radix)
		if got != c.want {
			t.Errorf("ParseInt(%q, %v) = %v, want %v", c.in, c.radix, got, c.want)
		}
	}
}

func TestParseIntNaN(t *testing.T) {
	// No valid digit, or a radix outside 2..36, so parseInt is NaN.
	cases := []struct {
		in    string
		radix float64
	}{
		{"", 0},
		{"abc", 0}, // no base-10 digit
		{"xyz", 0},
		{"  ", 0},
		{"+", 0},
		{"-", 0},
		{"z", 10},  // z is not a base-10 digit
		{"10", 1},  // radix below 2
		{"10", 37}, // radix above 36
		{"0x", 0},  // the prefix with no digits after it
	}
	for _, c := range cases {
		if got := ParseInt(FromGoString(c.in), c.radix); !math.IsNaN(got) {
			t.Errorf("ParseInt(%q, %v) = %v, want NaN", c.in, c.radix, got)
		}
	}
}
