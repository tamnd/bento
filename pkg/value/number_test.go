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

// TestToUint16 pins the ECMAScript ToUint16 coercion, the same steps as ToUint32
// reduced modulo 2^16: NaN and infinities go to zero, values truncate toward
// zero, and the result wraps into the unsigned 16-bit range.
func TestToUint16(t *testing.T) {
	cases := []struct {
		in   float64
		want uint16
	}{
		{0, 0},
		{65, 65},          // 'A'
		{-1, 65535},       // wraps to 2^16 - 1
		{65536, 0},        // 2^16 wraps to 0
		{65537, 1},        // 2^16 + 1 wraps to 1
		{3.9, 3},          // truncates toward zero
		{-3.9, 65533},     // trunc to -3 then wrap
		{math.NaN(), 0},   // NaN to 0
		{math.Inf(1), 0},  // +Inf to 0
		{math.Inf(-1), 0}, // -Inf to 0
		{65536*4 + 9, 9},  // large value keeps only the low 16 bits
	}
	for _, c := range cases {
		if got := ToUint16(c.in); got != c.want {
			t.Errorf("ToUint16(%v) = %d, want %d", c.in, got, c.want)
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
// TestRound pins Math.round: nearest integer with a tie broken toward +Infinity,
// the tricky just-under-one-half value staying down, and a zero result keeping the
// sign of the input.
func TestRound(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{2.5, 3},
		{-2.5, -2}, // tie rounds toward +Infinity, not away from zero
		{2.4, 2},
		{-2.4, -2},
		{2.6, 3},
		{-2.6, -3},
		{0.49999999999999994, 0}, // just under one half must not round up
		{0.5, 1},
		{-0.5, 0}, // returns -0, checked for sign below
	}
	for _, c := range cases {
		if got := Round(c.in); got != c.want {
			t.Errorf("Round(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := Round(-0.4); !math.Signbit(got) || got != 0 {
		t.Errorf("Round(-0.4) = %v, want -0", got)
	}
	if got := Round(-0.5); !math.Signbit(got) || got != 0 {
		t.Errorf("Round(-0.5) = %v, want -0", got)
	}
	if got := Round(math.NaN()); !math.IsNaN(got) {
		t.Errorf("Round(NaN) = %v, want NaN", got)
	}
	if got := Round(math.Inf(1)); !math.IsInf(got, 1) {
		t.Errorf("Round(+Inf) = %v, want +Inf", got)
	}
}

// TestSign pins Math.sign: one for positive, minus one for negative, and the
// argument itself for the zeros and NaN, which keeps the signed zeros intact.
func TestSign(t *testing.T) {
	cases := []struct {
		in   float64
		want float64
	}{
		{3, 1},
		{-3, -1},
		{0.0001, 1},
		{-0.0001, -1},
		{0, 0},
	}
	for _, c := range cases {
		if got := Sign(c.in); got != c.want {
			t.Errorf("Sign(%v) = %v, want %v", c.in, got, c.want)
		}
	}
	if got := Sign(math.Copysign(0, -1)); !math.Signbit(got) || got != 0 {
		t.Errorf("Sign(-0) = %v, want -0", got)
	}
	if got := Sign(math.NaN()); !math.IsNaN(got) {
		t.Errorf("Sign(NaN) = %v, want NaN", got)
	}
}

// TestMinMaxN pins the variadic Math.min and Math.max: the no-argument identities
// (+Infinity and -Infinity), the single-argument passthrough, ordinary folds over
// several arguments, the signed-zero order, and NaN propagation.
func TestMinMaxN(t *testing.T) {
	if got := MinN(); !math.IsInf(got, 1) {
		t.Errorf("MinN() = %v, want +Inf", got)
	}
	if got := MaxN(); !math.IsInf(got, -1) {
		t.Errorf("MaxN() = %v, want -Inf", got)
	}
	if got := MinN(5); got != 5 {
		t.Errorf("MinN(5) = %v, want 5", got)
	}
	if got := MinN(3, 1, 2); got != 1 {
		t.Errorf("MinN(3, 1, 2) = %v, want 1", got)
	}
	if got := MaxN(3, 1, 2); got != 3 {
		t.Errorf("MaxN(3, 1, 2) = %v, want 3", got)
	}
	// signed zeros: min keeps -0, max keeps +0.
	negZero := math.Copysign(0, -1)
	if got := MinN(negZero, 0); !math.Signbit(got) || got != 0 {
		t.Errorf("MinN(-0, 0) = %v, want -0", got)
	}
	if got := MaxN(negZero, 0); math.Signbit(got) || got != 0 {
		t.Errorf("MaxN(-0, 0) = %v, want +0", got)
	}
	// any NaN argument makes the whole result NaN.
	if got := MinN(1, math.NaN(), 2); !math.IsNaN(got) {
		t.Errorf("MinN(1, NaN, 2) = %v, want NaN", got)
	}
	if got := MaxN(1, math.NaN(), 2); !math.IsNaN(got) {
		t.Errorf("MaxN(1, NaN, 2) = %v, want NaN", got)
	}
}

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
