package value

import (
	"math"
	"testing"
)

func TestFround(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 0},
		{1, 1},
		{-1, -1},
		// 1.1 has no exact float32, so the round trip lands on the nearest one.
		{1.1, float64(float32(1.1))},
		// 2^24 + 1 is the first integer float32 cannot hold, so it rounds to 2^24.
		{16777217, 16777216},
		// a magnitude past the float32 range overflows to infinity, not a clamp.
		{1e39, math.Inf(1)},
		{-1e39, math.Inf(-1)},
	}
	for _, c := range cases {
		if got := Fround(c.in); got != c.want {
			t.Errorf("Fround(%g) = %g, want %g", c.in, got, c.want)
		}
	}
	if got := Fround(math.NaN()); !math.IsNaN(got) {
		t.Errorf("Fround(NaN) = %g, want NaN", got)
	}
	// A negative zero must stay negative through the round trip.
	if got := Fround(math.Copysign(0, -1)); !math.Signbit(got) || got != 0 {
		t.Errorf("Fround(-0) lost its sign: %g", got)
	}
}

func TestClz32(t *testing.T) {
	cases := []struct {
		in, want float64
	}{
		{0, 32},
		{1, 31},
		{2, 30},
		{0x80000000, 0},
		{0xFFFFFFFF, 0},
		// NaN and infinity coerce to 0 through ToUint32, so they count 32.
		{math.NaN(), 32},
		{math.Inf(1), 32},
		// -1 coerces to 0xFFFFFFFF, whose top bit is set, so no leading zero.
		{-1, 0},
		// a fraction truncates before the count, so 3.9 is 3, two bits set.
		{3.9, 30},
	}
	for _, c := range cases {
		if got := Clz32(c.in); got != c.want {
			t.Errorf("Clz32(%g) = %g, want %g", c.in, got, c.want)
		}
	}
}

func TestImul(t *testing.T) {
	cases := []struct {
		a, b, want float64
	}{
		{3, 4, 12},
		{-1, 8, -8},
		// the product overflows 32 bits and keeps only the low half, wrapping to a
		// negative value the way two's-complement multiplication does.
		{0xFFFFFFFF, 5, -5},
		{0x7FFFFFFF, 2, -2},
		// fractions truncate through ToInt32 before the multiply.
		{6.9, 3, 18},
		{0, 100, 0},
	}
	for _, c := range cases {
		if got := Imul(c.a, c.b); got != c.want {
			t.Errorf("Imul(%g, %g) = %g, want %g", c.a, c.b, got, c.want)
		}
	}
}
