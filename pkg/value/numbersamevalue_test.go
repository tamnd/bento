package value

import (
	"math"
	"testing"
)

// TestNumberSameValue pins the SameValue number cases where it agrees with plain
// equality and the two where it does not: two NaNs are the same value and the
// signed zeros are distinct.
func TestNumberSameValue(t *testing.T) {
	cases := []struct {
		a, b float64
		want bool
	}{
		{1, 1, true},
		{1, 2, false},
		{math.NaN(), math.NaN(), true},
		{math.NaN(), 1, false},
		{0, math.Copysign(0, -1), false},
		{math.Copysign(0, -1), math.Copysign(0, -1), true},
		{0, 0, true},
		{math.Inf(1), math.Inf(1), true},
		{math.Inf(1), math.Inf(-1), false},
	}
	for _, c := range cases {
		if got := NumberSameValue(c.a, c.b); got != c.want {
			t.Errorf("NumberSameValue(%v, %v) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
