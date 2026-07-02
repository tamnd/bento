package value

import (
	"math"
	"testing"
)

// TestNumberToString pins String(x) against the exact strings V8 produces,
// especially the cases where strconv's 'g' format would diverge: the exponential
// thresholds (a value at 1e21 goes exponential, 1e20 does not; 1e-6 stays decimal,
// 1e-7 goes exponential) and the unpadded exponent (JavaScript writes "e-7", not
// strconv's "e-07"). The signed zero, non-finite, and shortest-round-trip cases
// are here too.
func TestNumberToString(t *testing.T) {
	cases := []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{math.Copysign(0, -1), "0"}, // -0 stringifies to "0"
		{1, "1"},
		{-1, "-1"},
		{123, "123"},
		{1.5, "1.5"},
		{-1.5, "-1.5"},
		{0.5, "0.5"},
		{100, "100"},
		{0.1, "0.1"},
		{0.001, "0.001"},
		{1e20, "100000000000000000000"}, // still decimal at 1e20
		{1e21, "1e+21"},                 // exponential at 1e21
		{1e-6, "0.000001"},              // decimal at 1e-6
		{1e-7, "1e-7"},                  // exponential at 1e-7, exponent unpadded
		{1.5e-7, "1.5e-7"},
		{1.23e-10, "1.23e-10"},
		{123456789, "123456789"},
		{0.000001, "0.000001"},
		{1234567890123456768, "1234567890123456800"}, // shortest round-trip past 2^53
		{math.MaxFloat64, "1.7976931348623157e+308"},
		{5e-324, "5e-324"}, // smallest positive subnormal
		{math.NaN(), "NaN"},
		{math.Inf(1), "Infinity"},
		{math.Inf(-1), "-Infinity"},
	}
	for _, c := range cases {
		if got := NumberToString(c.in).ToGoString(); got != c.want {
			t.Errorf("NumberToString(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}
