package value

import "testing"

// The overflow option has no effect on Temporal.PlainYearMonth add/subtract in the ISO calendar:
// a year-month has no day, so the reference day the arithmetic anchors to is discarded, and a
// month step that would clamp that day (the last of a 31-day month less one month, landing in a
// shorter month) must give the same year-month under constrain and reject, never throwing.
func TestPlainYearMonthAddOverflowNoEffect(t *testing.T) {
	durs := []*Duration{
		NewDuration(1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		NewDuration(-1, 0, 0, 0, 0, 0, 0, 0, 0, 0),
		NewDuration(0, 1, 0, 0, 0, 0, 0, 0, 0, 0),
		NewDuration(0, -1, 0, 0, 0, 0, 0, 0, 0, 0),
	}
	for _, y := range []int{2023, 2024} {
		for m := 1; m <= 12; m++ {
			for _, d := range durs {
				ym := NewPlainYearMonth(float64(y), float64(m))
				var con, rej string
				if n := catchThrow(func() { con = ym.AddDuration(d, "constrain").ToString().ToGoString() }); n != "" {
					t.Fatalf("%d-%02d add under constrain threw %q", y, m, n)
				}
				if n := catchThrow(func() { rej = ym.AddDuration(d, "reject").ToString().ToGoString() }); n != "" {
					t.Fatalf("%d-%02d add under reject threw %q", y, m, n)
				}
				if con != rej {
					t.Errorf("%d-%02d: constrain %q != reject %q", y, m, con, rej)
				}
			}
		}
	}
}

// A genuine out-of-range result (past the representable year) still throws under both options, so
// the constrain-always reference add does not swallow the real range error.
func TestPlainYearMonthAddRangeStillThrows(t *testing.T) {
	ym := NewPlainYearMonth(275760, 9)
	big := NewDuration(1000, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	for _, ov := range []string{"constrain", "reject"} {
		if n := catchThrow(func() { ym.AddDuration(big, ov) }); n != "RangeError" {
			t.Errorf("overflow %s: got %q, want RangeError", ov, n)
		}
	}
}
