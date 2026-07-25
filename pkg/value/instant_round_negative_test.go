package value

import (
	"math/big"
	"testing"
)

func bigDec(s string) *big.Int { n, _ := new(big.Int).SetString(s, 10); return n }

// Temporal.Instant.prototype.round rounds a point on the absolute number line with
// RoundNumberToIncrementAsIfPositive: for a pre-epoch (negative) instant, ceil and expand still
// move toward the future (+inf) and floor and trunc still move toward the Big Bang (-inf), the
// same directions they take for a post-epoch instant.
func TestInstantRoundNegativeAsIfPositive(t *testing.T) {
	inst := NewInstant(bigDec("-1000000000000000000")) // 1938-04-24T22:13:20Z
	const roundedDown = "-1000000800000000000"          // 22:00:00Z, toward -inf
	const roundedUp = "-999997200000000000"             // 23:00:00Z, toward +inf
	cases := []struct {
		mode string
		want string
	}{
		{"halfCeil", roundedDown}, {"halfFloor", roundedDown}, {"halfExpand", roundedDown},
		{"halfTrunc", roundedDown}, {"halfEven", roundedDown}, {"floor", roundedDown},
		{"trunc", roundedDown}, {"ceil", roundedUp}, {"expand", roundedUp},
	}
	for _, c := range cases {
		got := inst.Round("hour", 1, c.mode).EpochNanoseconds().String()
		if got != c.want {
			t.Errorf("round(hour, %s): got %s, want %s", c.mode, got, c.want)
		}
	}
}

// Rounding down is toward the Big Bang, not toward the epoch or 1 BCE.
func TestInstantRoundNegativeDirection(t *testing.T) {
	inst := NewInstant(bigDec("-65261246399500000000")) // -000099-12-15T12:00:00.5Z
	cases := []struct {
		mode string
		want string
	}{
		{"floor", "-65261246400000000000"},      // toward Big Bang
		{"trunc", "-65261246400000000000"},       // as-if-positive: toward -inf
		{"ceil", "-65261246399000000000"},        // away from Big Bang
		{"halfExpand", "-65261246399000000000"},  // tie away, as-if-positive
	}
	for _, c := range cases {
		got := inst.Round("second", 1, c.mode).EpochNanoseconds().String()
		if got != c.want {
			t.Errorf("round(second, %s): got %s, want %s", c.mode, got, c.want)
		}
	}
}
