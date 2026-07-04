package value

import (
	"math"
	"testing"
)

// TestNumberToPrecision pins n.toPrecision(precision) against the exact strings
// V8 and JavaScriptCore produce. It covers both layouts the specification chooses
// between: the fixed form keeps a mid-range value's decimal point and pads with
// trailing significant zeros ((100).toPrecision(5) is "100.00", (123.456)
// .toPrecision(7) is "123.4560"), while a value with an exponent below -6 or at
// least the precision uses exponential notation ((1234.5).toPrecision(2) is
// "1.2e+3", (0.0000001).toPrecision(2) is "1.0e-7"). The exact-rounding cases
// (the 1.5 tie rounds up, 9.99 carries to "10") and the sign, zero, and
// non-finite cases are here too.
func TestNumberToPrecision(t *testing.T) {
	cases := []struct {
		in        float64
		precision int
		want      string
	}{
		{123.456, 4, "123.5"},
		{123.456, 2, "1.2e+2"},
		{123.456, 7, "123.4560"},
		{1234.5, 2, "1.2e+3"},
		{100, 5, "100.00"},
		{1, 3, "1.00"},
		{5, 1, "5"},
		{9.99, 2, "10"}, // rounding carries to a clean power
		{1.5, 1, "2"},   // exact tie rounds up
		{12345, 3, "1.23e+4"},
		{0.00001234, 2, "0.000012"},
		{0.000001, 2, "0.0000010"}, // exponent -6 stays in the fixed window
		{0.0000001, 2, "1.0e-7"},   // exponent -7 tips into exponential
		{0.0001, 1, "0.0001"},
		{-0.5, 1, "-0.5"},
		{0, 1, "0"},
		{0, 3, "0.00"},
		{math.NaN(), 3, "NaN"},
		{math.Inf(1), 3, "Infinity"},
		{math.Inf(-1), 3, "-Infinity"},
		{math.Copysign(0, -1), 2, "0.0"}, // -0 formats without a sign
	}
	for _, tc := range cases {
		got := NumberToPrecision(tc.in, tc.precision).ToGoString()
		if got != tc.want {
			t.Errorf("NumberToPrecision(%v, %d) = %q, want %q", tc.in, tc.precision, got, tc.want)
		}
	}
}
