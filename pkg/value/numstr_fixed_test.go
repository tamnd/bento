package value

import (
	"math"
	"testing"
)

// TestNumberToFixed pins n.toFixed(digits) against the exact strings V8 and
// JavaScriptCore produce, including the cases where strconv's ties-to-even 'f'
// format would diverge: the exact tie (0.5).toFixed(0) rounds up to "1", and
// (1.005).toFixed(2) is "1.00" because the double nearest 1.005 sits just below
// it. The padding, sign, zero, non-finite, and 1e21 fallback cases are here too.
func TestNumberToFixed(t *testing.T) {
	cases := []struct {
		in     float64
		digits int
		want   string
	}{
		{0, 0, "0"},
		{0, 2, "0.00"},
		{1, 0, "1"},
		{1, 3, "1.000"},
		{1.5, 0, "2"},
		{0.5, 0, "1"},      // exact tie rounds up
		{2.5, 0, "3"},      // exact tie rounds up
		{1.005, 2, "1.00"}, // nearest double is below 1.005
		{1.255, 2, "1.25"}, // nearest double is below 1.255
		{123.456, 2, "123.46"},
		{123.456, 0, "123"},
		{-1.5, 0, "-2"},
		{-0.5, 0, "-1"},
		{0.1, 1, "0.1"},
		{0.0001, 2, "0.00"},
		{255, 0, "255"},
		{1e21, 2, "1e+21"}, // at 1e21 falls back to Number::toString
		{math.NaN(), 2, "NaN"},
		{math.Inf(1), 2, "Infinity"},
		{math.Copysign(0, -1), 2, "0.00"}, // -0 formats without a sign
	}
	for _, tc := range cases {
		got := NumberToFixed(tc.in, tc.digits).ToGoString()
		if got != tc.want {
			t.Errorf("NumberToFixed(%v, %d) = %q, want %q", tc.in, tc.digits, got, tc.want)
		}
	}
}
