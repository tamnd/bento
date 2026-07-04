package value

import (
	"math"
	"testing"
)

// TestNumberToExponential pins n.toExponential(digits) against the exact strings
// V8 and JavaScriptCore produce, including the cases where strconv's 'e' format
// would diverge: the exponent is printed with an explicit sign and no leading
// zero (e+3, not e+03), a nine that rounds up carries into a new place and lifts
// the exponent (9.99 to "1.0e+1", 99999 to "1.0e+5"), an exact tie rounds up
// ((2.5).toExponential(0) is "3e+0"), and the double nearest 1.005 sits just
// below it so it rounds down. The sign, zero, and non-finite cases are here too.
func TestNumberToExponential(t *testing.T) {
	cases := []struct {
		in     float64
		digits int
		want   string
	}{
		{1234.5, 2, "1.23e+3"},
		{1234.5, 0, "1e+3"},
		{5, 3, "5.000e+0"},
		{0.0001234, 2, "1.23e-4"},
		{9.99, 1, "1.0e+1"},   // rounding carries a nine up into a new place
		{99999, 1, "1.0e+5"},  // and lifts the exponent
		{123456, 4, "1.2346e+5"},
		{2.5, 0, "3e+0"},      // exact tie rounds up
		{1.005, 2, "1.00e+0"}, // nearest double is below 1.005
		{1, 0, "1e+0"},
		{0, 0, "0e+0"},
		{0, 2, "0.00e+0"},
		{-0.5, 1, "-5.0e-1"},
		{0.1, 20, "1.00000000000000005551e-1"}, // the exact double, not the short form
		{math.NaN(), 2, "NaN"},
		{math.Inf(1), 2, "Infinity"},
		{math.Inf(-1), 2, "-Infinity"},
		{math.Copysign(0, -1), 2, "0.00e+0"}, // -0 formats without a sign
	}
	for _, tc := range cases {
		got := NumberToExponential(tc.in, tc.digits).ToGoString()
		if got != tc.want {
			t.Errorf("NumberToExponential(%v, %d) = %q, want %q", tc.in, tc.digits, got, tc.want)
		}
	}
}
