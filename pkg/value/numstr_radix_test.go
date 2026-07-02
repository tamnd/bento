package value

import (
	"math"
	"testing"
)

// TestNumberToStringRadix pins n.toString(radix) against the exact strings V8 and
// JavaScriptCore produce for a non-decimal radix. The integer cases are
// unambiguous and every engine agrees, and the fractional cases exercise the
// shared dtoa-in-base algorithm: the round-to-even carry, the classic 0.1 in
// binary that never terminates, and the precision cut-off. The non-finite, zero,
// and sign cases match Number::toString, and a radix of 10 delegates to it.
func TestNumberToStringRadix(t *testing.T) {
	cases := []struct {
		in    float64
		radix int
		want  string
	}{
		{255, 16, "ff"},
		{255, 2, "11111111"},
		{-255, 16, "-ff"},
		{0, 16, "0"},
		{1, 2, "1"},
		{10, 2, "1010"},
		{35, 36, "z"},
		{1295, 36, "zz"},
		{1000000, 16, "f4240"},
		{123456789, 16, "75bcd15"},
		{3.5, 2, "11.1"},
		{0.5, 2, "0.1"},
		{255.5, 16, "ff.8"},
		{0.1, 2, "0.0001100110011001100110011001100110011001100110011001101"},
		{0.5, 16, "0.8"},
		{100.25, 8, "144.2"},
		{math.NaN(), 16, "NaN"},
		{math.Inf(1), 16, "Infinity"},
		{math.Inf(-1), 16, "-Infinity"},
		{255, 10, "255"}, // radix 10 delegates to NumberToString
		{1.5, 10, "1.5"},
	}
	for _, tc := range cases {
		got := NumberToStringRadix(tc.in, tc.radix).ToGoString()
		if got != tc.want {
			t.Errorf("NumberToStringRadix(%v, %d) = %q, want %q", tc.in, tc.radix, got, tc.want)
		}
	}
}
