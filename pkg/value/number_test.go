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

// TestNumberPredicates pins Number.isNaN, isFinite, isInteger, and
// isSafeInteger over the values that separate them: NaN, an infinity, a whole
// number, a fraction, and an integer just past the safe range.
func TestNumberPredicates(t *testing.T) {
	cases := []struct {
		in                            float64
		nan, finite, integer, safeInt bool
	}{
		{0, false, true, true, true},
		{5, false, true, true, true},
		{-5, false, true, true, true},
		{3.5, false, true, false, false},
		{math.NaN(), true, false, false, false},
		{math.Inf(1), false, false, false, false},
		{math.Inf(-1), false, false, false, false},
		{maxSafeInteger, false, true, true, true},
		{maxSafeInteger + 2, false, true, true, false}, // integer but not safe
	}
	for _, c := range cases {
		if got := NumberIsNaN(c.in); got != c.nan {
			t.Errorf("NumberIsNaN(%v) = %v, want %v", c.in, got, c.nan)
		}
		if got := NumberIsFinite(c.in); got != c.finite {
			t.Errorf("NumberIsFinite(%v) = %v, want %v", c.in, got, c.finite)
		}
		if got := NumberIsInteger(c.in); got != c.integer {
			t.Errorf("NumberIsInteger(%v) = %v, want %v", c.in, got, c.integer)
		}
		if got := NumberIsSafeInteger(c.in); got != c.safeInt {
			t.Errorf("NumberIsSafeInteger(%v) = %v, want %v", c.in, got, c.safeInt)
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
