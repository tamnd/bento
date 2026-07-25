package value

import (
	"math/big"
	"testing"
)

// A time-zone identifier can be a bare offset in several spellings, or a full ISO date-time whose
// own designator names the zone: a Z is UTC and a numeric offset is that offset. withTimeZone
// resolves each to its canonical ±HH:MM (or UTC) form, so distinct spellings of the same zone
// compare equal and spellings of different zones do not.
func TestZonedDateTimeWithTimeZoneStringForms(t *testing.T) {
	inst := newZonedDateTime(big.NewInt(0), "UTC")
	tz := func(id string) string { return inst.WithTimeZone(id).tzID.ToGoString() }

	equal := [][2]string{
		{"+0330", "+03:30"},
		{"-0650", "-06:50"},
		{"-08", "-08:00"},
		{"1994-11-05T08:15:30-05:00", "-05:00"},
		{"1994-11-05T13:15:30Z", "UTC"},
	}
	for _, c := range equal {
		if a, b := tz(c[0]), tz(c[1]); a != b {
			t.Errorf("%q and %q should resolve to the same zone, got %q and %q", c[0], c[1], a, b)
		}
	}

	notEqual := [][2]string{
		{"+0330", "+03:31"},
		{"-0650", "-06:51"},
		{"-08", "-08:01"},
		{"1994-11-05T08:15:30-05:00", "-05:01"},
	}
	for _, c := range notEqual {
		if a, b := tz(c[0]), tz(c[1]); a == b {
			t.Errorf("%q and %q should resolve to different zones, both got %q", c[0], c[1], a)
		}
	}
}
