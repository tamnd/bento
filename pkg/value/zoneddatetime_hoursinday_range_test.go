package value

import (
	"math/big"
	"testing"
)

// hoursInDay reads the length of the local day as the gap between this day's start and the next
// day's start. When the receiver sits at the upper edge of the representable range the next day's
// start lands past nsMaxInstant, so the day length is unanswerable and the getter throws a
// RangeError rather than returning a wrong number. An interior instant answers the ordinary 24.
func TestZonedDateTimeHoursInDayOutOfRange(t *testing.T) {
	maxNs, _ := new(big.Int).SetString("8640000000000000000000", 10)
	atMax := newZonedDateTime(maxNs, "UTC")
	if got := catchThrow(func() { _ = atMax.HoursInDay() }); got != "RangeError" {
		t.Errorf("hoursInDay at the range maximum: want RangeError, got %q", got)
	}

	minNs := new(big.Int).Neg(maxNs)
	atMin := newZonedDateTime(minNs, "UTC")
	if got := catchThrow(func() {
		if h := atMin.HoursInDay(); h != 24 {
			t.Fatalf("hoursInDay at the range minimum = %v, want 24", h)
		}
	}); got != "" {
		t.Errorf("hoursInDay at the range minimum threw %q", got)
	}

	interior := newZonedDateTime(big.NewInt(0), "UTC")
	if got := catchThrow(func() {
		if h := interior.HoursInDay(); h != 24 {
			t.Fatalf("interior hoursInDay = %v, want 24", h)
		}
	}); got != "" {
		t.Errorf("interior hoursInDay threw %q", got)
	}
}
