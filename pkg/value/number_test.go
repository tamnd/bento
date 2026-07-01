package value

import (
	"math"
	"testing"
)

// TestToUint32 pins the ECMAScript ToUint32 coercion: NaN and infinities go to
// zero, values truncate toward zero, and the result wraps modulo 2^32 into the
// unsigned range, including for a number too large for an int64.
func TestToUint32(t *testing.T) {
	cases := []struct {
		in   float64
		want uint32
	}{
		{0, 0},
		{5, 5},
		{-1, 4294967295},      // wraps to 2^32 - 1
		{4294967296, 0},       // 2^32 wraps to 0
		{4294967297, 1},       // 2^32 + 1 wraps to 1
		{3.9, 3},              // truncates toward zero
		{-3.9, 4294967293},    // trunc to -3 then wrap
		{math.NaN(), 0},       // NaN to 0
		{math.Inf(1), 0},      // +Inf to 0
		{math.Inf(-1), 0},     // -Inf to 0
		{4294967296 * 3, 0},   // a multiple of 2^32 past int32 range
		{4294967296*5 + 7, 7}, // large value keeps only the low 32 bits
	}
	for _, c := range cases {
		if got := ToUint32(c.in); got != c.want {
			t.Errorf("ToUint32(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}

// TestToInt32 pins the ECMAScript ToInt32 coercion: the same steps as ToUint32
// but a value at or above 2^31 reads back as its negative two's-complement form.
func TestToInt32(t *testing.T) {
	cases := []struct {
		in   float64
		want int32
	}{
		{0, 0},
		{5, 5},
		{-1, -1},
		{2147483647, 2147483647},  // 2^31 - 1, the largest positive
		{2147483648, -2147483648}, // 2^31 wraps to the most negative
		{4294967295, -1},          // 2^32 - 1 is -1 signed
		{-3.9, -3},                // truncates toward zero, stays negative
		{math.NaN(), 0},
		{math.Inf(1), 0},
		{4294967296 * 2, 0}, // a multiple of 2^32
	}
	for _, c := range cases {
		if got := ToInt32(c.in); got != c.want {
			t.Errorf("ToInt32(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
