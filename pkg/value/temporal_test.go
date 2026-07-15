package value

import (
	"math"
	"math/big"
	"testing"
)

// mustPlainDate builds a PlainDate and fails the test if construction threw.
func mustPlainDate(t *testing.T, y, m, d float64) *PlainDate {
	t.Helper()
	var pd *PlainDate
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewPlainDate(%v,%v,%v) threw: %v", y, m, d, r)
			}
		}()
		pd = NewPlainDate(y, m, d)
	}()
	return pd
}

// plainDateThrows reports whether NewPlainDate throws a RangeError for the args.
func plainDateThrows(y, m, d float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainDate(y, m, d)
	return false
}

// TestPlainDateFields checks the clean field getters against the leap date
// 2020-02-29, whose values were taken from @js-temporal/polyfill.
func TestPlainDateFields(t *testing.T) {
	pd := mustPlainDate(t, 2020, 2, 29)
	if got := pd.Year(); got != 2020 {
		t.Errorf("Year = %v, want 2020", got)
	}
	if got := pd.Month(); got != 2 {
		t.Errorf("Month = %v, want 2", got)
	}
	if got := pd.Day(); got != 29 {
		t.Errorf("Day = %v, want 29", got)
	}
	if got := pd.CalendarId(); got.ToGoString() != "iso8601" {
		t.Errorf("CalendarId = %q, want iso8601", got.ToGoString())
	}
	if got := pd.MonthCode(); got.ToGoString() != "M02" {
		t.Errorf("MonthCode = %q, want M02", got.ToGoString())
	}
	if got := pd.DayOfWeek(); got != 6 {
		t.Errorf("DayOfWeek = %v, want 6", got)
	}
	if got := pd.DayOfYear(); got != 60 {
		t.Errorf("DayOfYear = %v, want 60", got)
	}
	if got := pd.DaysInWeek(); got != 7 {
		t.Errorf("DaysInWeek = %v, want 7", got)
	}
	if got := pd.DaysInMonth(); got != 29 {
		t.Errorf("DaysInMonth = %v, want 29", got)
	}
	if got := pd.DaysInYear(); got != 366 {
		t.Errorf("DaysInYear = %v, want 366", got)
	}
	if got := pd.MonthsInYear(); got != 12 {
		t.Errorf("MonthsInYear = %v, want 12", got)
	}
	if got := pd.InLeapYear(); got != true {
		t.Errorf("InLeapYear = %v, want true", got)
	}
}

// TestPlainDateDayOfWeek walks the seven weekdays across a known week: 2024-01-01
// is a Monday (ISO 1) through 2024-01-07 a Sunday (ISO 7).
func TestPlainDateDayOfWeek(t *testing.T) {
	for day := 1; day <= 7; day++ {
		pd := mustPlainDate(t, 2024, 1, float64(day))
		if got := pd.DayOfWeek(); got != float64(day) {
			t.Errorf("2024-01-%02d DayOfWeek = %v, want %d", day, got, day)
		}
	}
}

// TestPlainDateToString checks the ISO string, including the expanded signed
// six-digit year outside 0..9999, against the polyfill.
func TestPlainDateToString(t *testing.T) {
	cases := []struct {
		y, m, d float64
		want    string
	}{
		{2020, 2, 29, "2020-02-29"},
		{2024, 1, 1, "2024-01-01"},
		{-1, 12, 31, "-000001-12-31"},
		{275760, 9, 13, "+275760-09-13"},
		{12345, 6, 7, "+012345-06-07"},
		{0, 1, 1, "0000-01-01"},
	}
	for _, c := range cases {
		pd := mustPlainDate(t, c.y, c.m, c.d)
		if got := pd.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainDate(%v,%v,%v).ToString() = %q, want %q", c.y, c.m, c.d, got, c.want)
		}
		if got := pd.ToJSON().ToGoString(); got != c.want {
			t.Errorf("PlainDate(%v,%v,%v).ToJSON() = %q, want %q", c.y, c.m, c.d, got, c.want)
		}
	}
}

// TestPlainDateCompareAndEquals checks the static comparator and equals.
func TestPlainDateCompareAndEquals(t *testing.T) {
	a := mustPlainDate(t, 2020, 1, 1)
	b := mustPlainDate(t, 2020, 3, 15)
	c := mustPlainDate(t, 2020, 1, 1)
	if got := PlainDateCompare(a, b); got != -1 {
		t.Errorf("compare(a,b) = %v, want -1", got)
	}
	if got := PlainDateCompare(b, a); got != 1 {
		t.Errorf("compare(b,a) = %v, want 1", got)
	}
	if got := PlainDateCompare(a, c); got != 0 {
		t.Errorf("compare(a,c) = %v, want 0", got)
	}
	if !a.Equals(c) {
		t.Error("a.Equals(c) = false, want true")
	}
	if a.Equals(b) {
		t.Error("a.Equals(b) = true, want false")
	}
}

// TestPlainDateFromCopies proves from returns a distinct object that compares equal
// to its source, the copy the specification makes.
func TestPlainDateFromCopies(t *testing.T) {
	a := mustPlainDate(t, 2020, 1, 1)
	b := PlainDateFrom(a)
	if a == b {
		t.Error("PlainDateFrom returned the same pointer, want a copy")
	}
	if !a.Equals(b) {
		t.Error("from copy does not equal its source")
	}
}

// TestPlainDateTruncatesArguments proves a fractional argument truncates toward
// zero, matching ToIntegerWithTruncation.
func TestPlainDateTruncatesArguments(t *testing.T) {
	pd := mustPlainDate(t, 2020.9, 1.9, 1.9)
	if pd.ToString().ToGoString() != "2020-01-01" {
		t.Errorf("truncated PlainDate = %q, want 2020-01-01", pd.ToString().ToGoString())
	}
}

// TestPlainDateRejects checks the RangeError cases against the polyfill: an
// out-of-range month or day, a non-finite component, a NaN component (which throws in
// ToIntegerWithTruncation rather than settling on zero, so even a NaN year that would
// otherwise land on the valid 0000-01-01 raises), and the two representable-range
// boundaries.
func TestPlainDateRejects(t *testing.T) {
	throwing := [][3]float64{
		{2020, 0, 1},
		{2020, 13, 1},
		{2020, 1, 0},
		{2020, 2, 30},
		{2020, nan(), 1},
		{nan(), 1, 1}, // NaN year must throw, not settle on the valid 0000-01-01
		{inf(1), 1, 1},
		{-271821, 4, 18}, // one day before the minimum
		{275760, 9, 14},  // one day after the maximum
	}
	for _, c := range throwing {
		if !plainDateThrows(c[0], c[1], c[2]) {
			t.Errorf("NewPlainDate(%v,%v,%v) did not throw", c[0], c[1], c[2])
		}
	}
	valid := [][3]float64{
		{-271821, 4, 19}, // the minimum
		{275760, 9, 13},  // the maximum
	}
	for _, c := range valid {
		if plainDateThrows(c[0], c[1], c[2]) {
			t.Errorf("NewPlainDate(%v,%v,%v) threw at a valid boundary", c[0], c[1], c[2])
		}
	}
}

// mustPlainTime builds a PlainTime and fails the test if construction threw.
func mustPlainTime(t *testing.T, h, m, s, ms, us, ns float64) *PlainTime {
	t.Helper()
	var pt *PlainTime
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewPlainTime(%v,%v,%v,%v,%v,%v) threw: %v", h, m, s, ms, us, ns, r)
			}
		}()
		pt = NewPlainTime(h, m, s, ms, us, ns)
	}()
	return pt
}

// plainTimeThrows reports whether NewPlainTime throws a RangeError for the args.
func plainTimeThrows(h, m, s, ms, us, ns float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainTime(h, m, s, ms, us, ns)
	return false
}

// TestPlainTimeFields checks the six clean field getters against a time with every
// field set, the values taken from @js-temporal/polyfill.
func TestPlainTimeFields(t *testing.T) {
	pt := mustPlainTime(t, 1, 2, 3, 4, 5, 6)
	if got := pt.Hour(); got != 1 {
		t.Errorf("Hour = %v, want 1", got)
	}
	if got := pt.Minute(); got != 2 {
		t.Errorf("Minute = %v, want 2", got)
	}
	if got := pt.Second(); got != 3 {
		t.Errorf("Second = %v, want 3", got)
	}
	if got := pt.Millisecond(); got != 4 {
		t.Errorf("Millisecond = %v, want 4", got)
	}
	if got := pt.Microsecond(); got != 5 {
		t.Errorf("Microsecond = %v, want 5", got)
	}
	if got := pt.Nanosecond(); got != 6 {
		t.Errorf("Nanosecond = %v, want 6", got)
	}
}

// TestPlainTimeToString checks the ISO time string, including the trimmed fractional
// second, against the polyfill.
func TestPlainTimeToString(t *testing.T) {
	cases := []struct {
		h, m, s, ms, us, ns float64
		want                string
	}{
		{12, 30, 0, 0, 0, 0, "12:30:00"},
		{12, 30, 0, 250, 0, 0, "12:30:00.25"},
		{1, 2, 3, 4, 5, 6, "01:02:03.004005006"},
		{0, 0, 0, 0, 0, 0, "00:00:00"},
		{23, 59, 59, 999, 999, 999, "23:59:59.999999999"},
	}
	for _, c := range cases {
		pt := mustPlainTime(t, c.h, c.m, c.s, c.ms, c.us, c.ns)
		if got := pt.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainTime(%v,%v,%v,%v,%v,%v).ToString() = %q, want %q", c.h, c.m, c.s, c.ms, c.us, c.ns, got, c.want)
		}
		if got := pt.ToJSON().ToGoString(); got != c.want {
			t.Errorf("PlainTime(%v,%v,%v,%v,%v,%v).ToJSON() = %q, want %q", c.h, c.m, c.s, c.ms, c.us, c.ns, got, c.want)
		}
	}
}

// TestPlainTimeCompareAndEquals checks the static comparator and equals.
func TestPlainTimeCompareAndEquals(t *testing.T) {
	a := mustPlainTime(t, 1, 0, 0, 0, 0, 0)
	b := mustPlainTime(t, 2, 0, 0, 0, 0, 0)
	c := mustPlainTime(t, 1, 0, 0, 0, 0, 0)
	if got := PlainTimeCompare(a, b); got != -1 {
		t.Errorf("compare(a,b) = %v, want -1", got)
	}
	if got := PlainTimeCompare(b, a); got != 1 {
		t.Errorf("compare(b,a) = %v, want 1", got)
	}
	if got := PlainTimeCompare(a, c); got != 0 {
		t.Errorf("compare(a,c) = %v, want 0", got)
	}
	// A difference only in the least significant field still orders.
	lo := mustPlainTime(t, 3, 15, 30, 0, 0, 1)
	hi := mustPlainTime(t, 3, 15, 30, 0, 0, 2)
	if got := PlainTimeCompare(lo, hi); got != -1 {
		t.Errorf("compare over the nanosecond = %v, want -1", got)
	}
	if !a.Equals(c) {
		t.Error("a.Equals(c) = false, want true")
	}
	if a.Equals(b) {
		t.Error("a.Equals(b) = true, want false")
	}
}

// TestPlainTimeFromCopies proves from returns a distinct object that compares equal to
// its source, the copy the specification makes.
func TestPlainTimeFromCopies(t *testing.T) {
	a := mustPlainTime(t, 5, 6, 7, 0, 0, 0)
	b := PlainTimeFrom(a)
	if a == b {
		t.Error("PlainTimeFrom returned the same pointer, want a copy")
	}
	if !a.Equals(b) {
		t.Error("from copy does not equal its source")
	}
}

// TestPlainTimeTruncatesArguments proves a fractional argument truncates toward zero,
// matching ToIntegerWithTruncation.
func TestPlainTimeTruncatesArguments(t *testing.T) {
	pt := mustPlainTime(t, 12.9, 30.9, 0.9, 0, 0, 0)
	if pt.ToString().ToGoString() != "12:30:00" {
		t.Errorf("truncated PlainTime = %q, want 12:30:00", pt.ToString().ToGoString())
	}
}

// TestPlainTimeRejects checks the RangeError cases against the polyfill: each field past
// its range, a NaN component (which throws in ToIntegerWithTruncation), and a non-finite
// component.
func TestPlainTimeRejects(t *testing.T) {
	throwing := [][6]float64{
		{24, 0, 0, 0, 0, 0},
		{-1, 0, 0, 0, 0, 0},
		{0, 60, 0, 0, 0, 0},
		{0, 0, 60, 0, 0, 0},
		{0, 0, 0, 1000, 0, 0},
		{0, 0, 0, 0, 1000, 0},
		{0, 0, 0, 0, 0, 1000},
		{nan(), 0, 0, 0, 0, 0},
		{inf(1), 0, 0, 0, 0, 0},
	}
	for _, c := range throwing {
		if !plainTimeThrows(c[0], c[1], c[2], c[3], c[4], c[5]) {
			t.Errorf("NewPlainTime%v did not throw", c)
		}
	}
	// The all-max valid boundary must not throw.
	if plainTimeThrows(23, 59, 59, 999, 999, 999) {
		t.Error("NewPlainTime at the valid maximum threw")
	}
}

// bagThrows reports whether f panics with a Thrown value, the signal a RangeError
// reached the caller.
func bagThrows(f func()) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	f()
	return false
}

// TestPlainTimeFromFields drives Temporal.PlainTime.from over a property bag: absent
// fields fall to zero, present fields set, and overflow chooses between clamp and throw.
// The expected renderings come from @js-temporal/polyfill.
func TestPlainTimeFromFields(t *testing.T) {
	some := Some[float64]
	none := None[float64]()
	cases := []struct {
		name                string
		h, m, s, ms, us, ns Opt[float64]
		overflow            string
		want                string
	}{
		{"three fields", some(12), some(30), some(15), none, none, none, "constrain", "12:30:15"},
		{"hour over constrain", some(25), none, none, none, none, none, "constrain", "23:00:00"},
		{"minute over constrain", none, some(90), none, none, none, none, "constrain", "00:59:00"},
		{"millisecond only", some(5), none, none, some(250), none, none, "constrain", "05:00:00.25"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			pt := PlainTimeFromFields(c.h, c.m, c.s, c.ms, c.us, c.ns, c.overflow)
			if got := pt.ToString().ToGoString(); got != c.want {
				t.Errorf("PlainTimeFromFields = %q, want %q", got, c.want)
			}
		})
	}
	// reject turns an out-of-range field into a RangeError instead of clamping.
	if !bagThrows(func() {
		PlainTimeFromFields(some(25), none, none, none, none, none, timeOverflowReject)
	}) {
		t.Error("PlainTimeFromFields with hour 25 and reject did not throw")
	}
}

// TestPlainTimeWith drives Temporal.PlainTime.prototype.with: absent fields hold the
// receiver's value, present fields replace it, and overflow governs the out-of-range case.
func TestPlainTimeWith(t *testing.T) {
	some := Some[float64]
	none := None[float64]()
	base := mustPlainTime(t, 12, 30, 15, 0, 0, 0)
	if got := base.With(none, some(45), none, none, none, none, "constrain").ToString().ToGoString(); got != "12:45:15" {
		t.Errorf("with minute 45 = %q, want 12:45:15", got)
	}
	if got := base.With(some(23), some(90), none, none, none, none, "constrain").ToString().ToGoString(); got != "23:59:15" {
		t.Errorf("with hour 23 minute 90 constrain = %q, want 23:59:15", got)
	}
	if !bagThrows(func() {
		base.With(some(23), some(90), none, none, none, none, timeOverflowReject)
	}) {
		t.Error("with minute 90 and reject did not throw")
	}
}

// TestPlainTimeAddDuration folds a Duration into a PlainTime and checks the wall-clock
// wraps mod 24h, only the time units count, and subtract is add over a negated Duration.
// The expected strings are taken from @js-temporal/polyfill.
func TestPlainTimeAddDuration(t *testing.T) {
	base := mustPlainTime(t, 12, 30, 15, 0, 0, 0)
	dur := func(a ...float64) *Duration {
		t.Helper()
		var d *Duration
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("NewDuration(%v) threw: %v", a, r)
				}
			}()
			d = NewDuration(a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9])
		}()
		return d
	}
	cases := []struct {
		name string
		d    *Duration
		want string
	}{
		{"hours 25 wraps", dur(0, 0, 0, 0, 25, 0, 0, 0, 0, 0), "13:30:15"},
		{"days ignored", dur(0, 0, 0, 1, 0, 0, 0, 0, 0, 0), "12:30:15"},
		{"months ignored", dur(0, 1, 0, 0, 0, 0, 0, 0, 0, 0), "12:30:15"},
		{"hours 90 minutes 90", dur(0, 0, 0, 0, 90, 90, 0, 0, 0, 0), "08:00:15"},
		{"negative hour wraps", dur(0, 0, 0, 0, -1, 0, 0, 0, 0, 0), "11:30:15"},
		{"fractional millis", dur(0, 0, 0, 0, 0, 0, 0, 1500, 0, 0), "12:30:16.5"},
	}
	for _, tc := range cases {
		if got := base.AddDuration(tc.d).ToString().ToGoString(); got != tc.want {
			t.Errorf("%s: add = %q, want %q", tc.name, got, tc.want)
		}
	}
	// subtract 13 hours from 12:30:15 wraps back to the previous day at 23:30:15.
	sub := dur(0, 0, 0, 0, 13, 0, 0, 0, 0, 0).Negated()
	if got := base.AddDuration(sub).ToString().ToGoString(); got != "23:30:15" {
		t.Errorf("subtract 13h = %q, want 23:30:15", got)
	}
}

// TestPlainTimeRound checks the wall-clock rounding: the six smallest units, an increment
// above one, every rounding mode over a tie, and the wrap past midnight. The reject cases
// throw a RangeError. The expected strings are taken from @js-temporal/polyfill.
func TestPlainTimeRound(t *testing.T) {
	base := mustPlainTime(t, 3, 34, 56, 987, 654, 321)
	cases := []struct {
		name      string
		pt        *PlainTime
		unit      string
		increment float64
		mode      string
		want      string
	}{
		{"hour halfExpand", base, "hour", 1, "halfExpand", "04:00:00"},
		{"minute", base, "minute", 1, "halfExpand", "03:35:00"},
		{"second", base, "second", 1, "halfExpand", "03:34:57"},
		{"millisecond", base, "millisecond", 1, "halfExpand", "03:34:56.988"},
		{"microsecond", base, "microsecond", 1, "halfExpand", "03:34:56.987654"},
		{"nanosecond", base, "nanosecond", 1, "halfExpand", "03:34:56.987654321"},
		{"minute increment 15", base, "minute", 15, "halfExpand", "03:30:00"},
		{"minute increment 30 ceil", base, "minute", 30, "ceil", "04:00:00"},
		{"hour floor", base, "hour", 1, "floor", "03:00:00"},
		{"hour trunc", base, "hour", 1, "trunc", "03:00:00"},
		{"hour expand", base, "hour", 1, "expand", "04:00:00"},
		{"wrap to next day", mustPlainTime(t, 23, 59, 0, 0, 0, 0), "hour", 1, "ceil", "00:00:00"},
	}
	for _, tc := range cases {
		if got := tc.pt.Round(tc.unit, tc.increment, tc.mode).ToString().ToGoString(); got != tc.want {
			t.Errorf("%s: round = %q, want %q", tc.name, got, tc.want)
		}
	}

	// The 3:30 tie resolves by mode: floor and the half-toward-low modes keep 03, the
	// others advance to 04, and halfEven advances because 3 is odd.
	tie := mustPlainTime(t, 3, 30, 0, 0, 0, 0)
	ties := map[string]string{
		"halfCeil": "04:00:00", "halfFloor": "03:00:00", "halfExpand": "04:00:00",
		"halfTrunc": "03:00:00", "halfEven": "04:00:00",
	}
	for mode, want := range ties {
		if got := tie.Round("hour", 1, mode).ToString().ToGoString(); got != want {
			t.Errorf("3:30 %s: round = %q, want %q", mode, got, want)
		}
	}
	// The 4:30 tie flips halfEven the other way, since 4 is even.
	if got := mustPlainTime(t, 4, 30, 0, 0, 0, 0).Round("hour", 1, "halfEven").ToString().ToGoString(); got != "04:00:00" {
		t.Errorf("4:30 halfEven: round = %q, want 04:00:00", got)
	}

	// An increment that does not divide the unit's dividend, or equals it, throws.
	for _, inc := range []float64{5, 24} {
		if !bagThrows(func() { base.Round("hour", inc, "halfExpand") }) {
			t.Errorf("hour increment %v did not throw", inc)
		}
	}
	// An increment below one throws.
	if !bagThrows(func() { base.Round("minute", 0, "halfExpand") }) {
		t.Error("minute increment 0 did not throw")
	}
}

// TestPlainTimeUntilSince pins the wall-clock difference: until measures other minus the
// receiver and since the reverse, both balanced from largestUnit down and rounded at
// smallestUnit under the mode, with the default largestUnit hour, smallestUnit nanosecond,
// and mode trunc. The values were checked against @js-temporal/polyfill.
func TestPlainTimeUntilSince(t *testing.T) {
	a := mustPlainTime(t, 12, 30, 15, 0, 0, 0)
	b := mustPlainTime(t, 14, 45, 30, 0, 0, 0)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"until default", a.Until(b, "hour", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT2H15M15S"},
		{"since default", a.Since(b, "hour", "nanosecond", 1, "trunc").ToString().ToGoString(), "-PT2H15M15S"},
		{"until reversed", b.Until(a, "hour", "nanosecond", 1, "trunc").ToString().ToGoString(), "-PT2H15M15S"},
		{"until largestUnit minute", a.Until(b, "minute", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT135M15S"},
		{"until largestUnit second", a.Until(b, "second", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT8115S"},
		{"until smallestUnit minute trunc", a.Until(b, "hour", "minute", 1, "trunc").ToString().ToGoString(), "PT2H15M"},
		{"until smallestUnit minute ceil", a.Until(b, "hour", "minute", 1, "ceil").ToString().ToGoString(), "PT2H16M"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: difference = %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// A 2:45 gap rounded to the hour resolves by the signed mode. until truncs toward zero
	// to 2 hours, ceil advances to 3, and since carries the same rounding on the negated
	// difference, so ceil on a negative gap trims toward zero to two hours.
	c := mustPlainTime(t, 12, 0, 0, 0, 0, 0)
	d := mustPlainTime(t, 14, 45, 0, 0, 0, 0)
	signed := map[string]string{
		c.Until(d, "hour", "hour", 1, "trunc").ToString().ToGoString(): "PT2H",
		c.Until(d, "hour", "hour", 1, "ceil").ToString().ToGoString():  "PT3H",
		c.Since(d, "hour", "hour", 1, "trunc").ToString().ToGoString(): "-PT2H",
		c.Since(d, "hour", "hour", 1, "ceil").ToString().ToGoString():  "-PT2H",
		c.Since(d, "hour", "hour", 1, "floor").ToString().ToGoString(): "-PT3H",
	}
	for got, want := range signed {
		if got != want {
			t.Errorf("signed difference = %q, want %q", got, want)
		}
	}

	// Two equal times differ by zero.
	if got := a.Until(a, "hour", "nanosecond", 1, "trunc").ToString().ToGoString(); got != "PT0S" {
		t.Errorf("equal times: until = %q, want PT0S", got)
	}

	// largestUnit smaller than smallestUnit throws, as does an out-of-range increment.
	if !bagThrows(func() { a.Until(b, "second", "hour", 1, "trunc") }) {
		t.Error("largestUnit second below smallestUnit hour did not throw")
	}
	if !bagThrows(func() { a.Until(b, "hour", "minute", 7, "trunc") }) {
		t.Error("minute increment 7 did not throw")
	}
}

// TestPlainDateAddSubtract checks the calendar date arithmetic against values taken from
// @js-temporal/polyfill: the month and year carry with the day clamped to a short month under
// constrain, the week and day balance across a month and year boundary, the time components
// folding into a whole-day carry truncated toward zero, and the reject overflow throwing when
// the clamped day does not fit.
func TestPlainDateAddSubtract(t *testing.T) {
	// dur builds a Duration from the named-ish positional components.
	dur := func(y, mo, w, d, h, mi, s float64) *Duration {
		return NewDuration(y, mo, w, d, h, mi, s, 0, 0, 0)
	}
	jan31 := mustPlainDate(t, 2024, 1, 31)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"month carry clamps to short month", jan31.AddDate(dur(0, 1, 0, 0, 0, 0, 0), "constrain").ToString().ToGoString(), "2024-02-29"},
		{"year onto a leap day clamps", mustPlainDate(t, 2020, 2, 29).AddDate(dur(1, 0, 0, 0, 0, 0, 0), "constrain").ToString().ToGoString(), "2021-02-28"},
		{"plain year add", mustPlainDate(t, 2024, 1, 31).AddDate(dur(1, 0, 0, 0, 0, 0, 0), "constrain").ToString().ToGoString(), "2025-01-31"},
		{"two weeks cross the month", mustPlainDate(t, 2024, 1, 15).AddDate(dur(0, 0, 2, 0, 0, 0, 0), "constrain").ToString().ToGoString(), "2024-01-29"},
		{"days roll into March", jan31.AddDate(dur(0, 0, 0, 30, 0, 0, 0), "constrain").ToString().ToGoString(), "2024-03-01"},
		{"day past year end", mustPlainDate(t, 2024, 12, 31).AddDate(dur(0, 0, 0, 1, 0, 0, 0), "constrain").ToString().ToGoString(), "2025-01-01"},
		{"years months days compose", jan31.AddDate(dur(1, 1, 0, 1, 0, 0, 0), "constrain").ToString().ToGoString(), "2025-03-01"},
		{"25 hours carry one day", jan31.AddDate(dur(0, 0, 0, 0, 25, 0, 0), "constrain").ToString().ToGoString(), "2024-02-01"},
		{"day and 25 hours carry two", jan31.AddDate(dur(0, 0, 0, 1, 25, 0, 0), "constrain").ToString().ToGoString(), "2024-02-02"},
		{"23 hours carry nothing", mustPlainDate(t, 2024, 6, 15).AddDate(dur(0, 0, 0, 0, 23, 0, 0), "constrain").ToString().ToGoString(), "2024-06-15"},
		{"negative hours trunc toward zero", mustPlainDate(t, 2024, 6, 15).AddDate(dur(0, 0, 0, 0, -25, 0, 0), "constrain").ToString().ToGoString(), "2024-06-14"},
		{"subtract via negated duration", mustPlainDate(t, 2024, 3, 31).AddDate(dur(0, 1, 0, 0, 0, 0, 0).Negated(), "constrain").ToString().ToGoString(), "2024-02-29"},
		{"subtract a month negative", mustPlainDate(t, 2024, 3, 31).AddDate(dur(0, -1, 0, 0, 0, 0, 0), "constrain").ToString().ToGoString(), "2024-02-29"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: add = %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// reject throws when the day does not fit the target month.
	if !bagThrows(func() { jan31.AddDate(dur(0, 1, 0, 0, 0, 0, 0), "reject") }) {
		t.Error("Jan 31 + 1 month under reject did not throw")
	}
	if !bagThrows(func() { mustPlainDate(t, 2020, 2, 29).AddDate(dur(1, 0, 0, 0, 0, 0, 0), "reject") }) {
		t.Error("Feb 29 + 1 year under reject did not throw")
	}
	// A date pushed past the representable range throws.
	if !bagThrows(func() { mustPlainDate(t, 275760, 9, 13).AddDate(dur(0, 0, 0, 1, 0, 0, 0), "constrain") }) {
		t.Error("a day past the maximum date did not throw")
	}
}

// TestPlainDateUntilSince pins the calendar difference: the largestUnit balancing from days
// up to years, the month anchoring that settles a short-month remainder in days, since as the
// negation of until, and the RangeError when the two dates carry different calendars. Every
// expected value was checked against @js-temporal/polyfill.
func TestPlainDateUntilSince(t *testing.T) {
	a := mustPlainDate(t, 2020, 1, 31)
	b := mustPlainDate(t, 2021, 3, 30)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"until day default", a.Until(b, "day").ToString().ToGoString(), "P424D"},
		{"until year", a.Until(b, "year").ToString().ToGoString(), "P1Y1M30D"},
		{"until month", a.Until(b, "month").ToString().ToGoString(), "P13M30D"},
		{"until week", a.Until(b, "week").ToString().ToGoString(), "P60W4D"},
		{"since year negates until", a.Since(b, "year").ToString().ToGoString(), "-P1Y1M30D"},
		{"since day negates until", a.Since(b, "day").ToString().ToGoString(), "-P424D"},
		{"reversed until year", b.Until(a, "year").ToString().ToGoString(), "-P1Y1M29D"},
		{"jan31 to feb29 year settles in days", mustPlainDate(t, 2020, 1, 31).Until(mustPlainDate(t, 2020, 2, 29), "year").ToString().ToGoString(), "P29D"},
		{"feb29 to mar1 month", mustPlainDate(t, 2020, 2, 29).Until(mustPlainDate(t, 2020, 3, 1), "month").ToString().ToGoString(), "P1D"},
		{"long span year", mustPlainDate(t, 2000, 1, 1).Until(mustPlainDate(t, 2025, 6, 15), "year").ToString().ToGoString(), "P25Y5M14D"},
		{"within a month", mustPlainDate(t, 2020, 6, 10).Until(mustPlainDate(t, 2020, 6, 25), "month").ToString().ToGoString(), "P15D"},
		{"year boundary as year", mustPlainDate(t, 2020, 12, 31).Until(mustPlainDate(t, 2021, 1, 1), "year").ToString().ToGoString(), "P1D"},
		{"since across months", mustPlainDate(t, 2024, 3, 31).Since(mustPlainDate(t, 2024, 1, 31), "month").ToString().ToGoString(), "P2M"},
		{"equal dates", a.Until(a, "year").ToString().ToGoString(), "PT0S"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// until and since throw when the two dates carry different calendars.
	iso := mustPlainDate(t, 2020, 6, 1)
	greg := PlainDateWithCalendar(mustPlainDate(t, 2020, 6, 15), "gregory")
	if !bagThrows(func() { iso.Until(greg, "day") }) {
		t.Error("until across two calendars did not throw")
	}
}

// TestPlainDateWithFields checks with lays a bag's fields over the receiver and regulates
// the result: an omitted field keeps its value, constrain clamps a short month and an
// out-of-range month or day, reject throws, and under roc the bag year maps through the
// 1911 offset to the ISO year the date stores.
func TestPlainDateWithFields(t *testing.T) {
	d := mustPlainDate(t, 2020, 1, 31)
	some := func(v float64) Opt[float64] { return Some(v) }
	none := None[float64]()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"month constrains the day", d.WithFields(none, some(2), none, "constrain").ToString().ToGoString(), "2020-02-29"},
		{"day only", d.WithFields(none, none, some(15), "constrain").ToString().ToGoString(), "2020-01-15"},
		{"all three fields", d.WithFields(some(2021), some(6), some(10), "constrain").ToString().ToGoString(), "2021-06-10"},
		{"month over twelve clamps", d.WithFields(none, some(13), none, "constrain").ToString().ToGoString(), "2020-12-31"},
		{"day over the month clamps", d.WithFields(none, none, some(40), "constrain").ToString().ToGoString(), "2020-01-31"},
		{"leap day settles on a common year", mustPlainDate(t, 2020, 2, 29).WithFields(some(2021), none, none, "constrain").ToString().ToGoString(), "2021-02-28"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// roc reads the bag year in Minguo reckoning, so year 100 is ISO 2011.
	roc := PlainDateWithCalendar(mustPlainDate(t, 2024, 5, 15), "roc")
	if got := roc.WithFields(some(100), none, none, "constrain").ToString().ToGoString(); got != "2011-05-15[u-ca=roc]" {
		t.Errorf("roc with year: got %q, want %q", got, "2011-05-15[u-ca=roc]")
	}

	// reject throws when a field does not fit the resulting month.
	if !bagThrows(func() { d.WithFields(none, none, some(40), "reject") }) {
		t.Error("with day 40 under reject did not throw")
	}
	if !bagThrows(func() { d.WithFields(none, some(13), none, "reject") }) {
		t.Error("with month 13 under reject did not throw")
	}
}

func TestPlainDateFromFields(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"plain date", PlainDateFromFields(2020, 3, 14, "iso8601", "constrain").ToString().ToGoString(), "2020-03-14"},
		{"day constrains to the leap February", PlainDateFromFields(2020, 2, 31, "iso8601", "constrain").ToString().ToGoString(), "2020-02-29"},
		{"day constrains to the common February", PlainDateFromFields(2021, 2, 31, "iso8601", "constrain").ToString().ToGoString(), "2021-02-28"},
		{"month over twelve clamps to December", PlainDateFromFields(2020, 13, 5, "iso8601", "constrain").ToString().ToGoString(), "2020-12-05"},
		{"gregory carries its calendar", PlainDateFromFields(2020, 5, 15, "gregory", "constrain").ToString().ToGoString(), "2020-05-15[u-ca=gregory]"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// roc reads the bag year in Minguo reckoning, so year 109 is ISO 2020.
	roc := PlainDateFromFields(109, 5, 15, "roc", "constrain")
	if got := roc.ToString().ToGoString(); got != "2020-05-15[u-ca=roc]" {
		t.Errorf("roc from fields: got %q, want %q", got, "2020-05-15[u-ca=roc]")
	}
	if got := roc.Year(); got != 109 {
		t.Errorf("roc year: got %v, want 109", got)
	}
	if got := roc.CalendarId().ToGoString(); got != "roc" {
		t.Errorf("roc calendarId: got %q, want roc", got)
	}

	// reject throws when a field does not fit the resulting month.
	if !bagThrows(func() { PlainDateFromFields(2020, 2, 31, "iso8601", "reject") }) {
		t.Error("from fields day 31 in February under reject did not throw")
	}
}

// TestPlainDateTimeFromFields checks Temporal.PlainDateTime.from over a property bag: the required
// date fields pair with the optional time fields, an omitted time field defaults to midnight, each
// half regulates under the overflow option, and the calendar carries. Every value was checked
// against @js-temporal/polyfill.
func TestPlainDateTimeFromFields(t *testing.T) {
	some := func(v float64) Opt[float64] { return Some(v) }
	none := None[float64]()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"date only defaults to midnight", PlainDateTimeFromFields(2020, 1, 31, none, none, none, none, none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-01-31T00:00:00"},
		{"date and time", PlainDateTimeFromFields(2020, 1, 31, some(13), some(30), some(45), none, none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-01-31T13:30:45"},
		{"day constrains to the leap February", PlainDateTimeFromFields(2020, 2, 31, none, none, none, none, none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-02-29T00:00:00"},
		{"month over twelve clamps to December", PlainDateTimeFromFields(2020, 13, 5, none, none, none, none, none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-12-05T00:00:00"},
		{"hour over the day clamps", PlainDateTimeFromFields(2020, 1, 31, some(25), none, none, none, none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-01-31T23:00:00"},
		{"subsecond time carries", PlainDateTimeFromFields(2020, 1, 31, some(5), none, none, some(250), none, none, "iso8601", "constrain").ToString().ToGoString(), "2020-01-31T05:00:00.25"},
		{"gregory carries its calendar", PlainDateTimeFromFields(2020, 5, 15, none, none, none, none, none, none, "gregory", "constrain").ToString().ToGoString(), "2020-05-15T00:00:00[u-ca=gregory]"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// roc reads the bag year in Minguo reckoning, so year 109 is ISO 2020, and the calendar carries.
	roc := PlainDateTimeFromFields(109, 5, 15, some(12), none, none, none, none, none, "roc", "constrain")
	if got := roc.ToString().ToGoString(); got != "2020-05-15T12:00:00[u-ca=roc]" {
		t.Errorf("roc from fields: got %q, want %q", got, "2020-05-15T12:00:00[u-ca=roc]")
	}

	// reject throws when a field does not fit its half.
	if !bagThrows(func() { PlainDateTimeFromFields(2020, 2, 31, none, none, none, none, none, none, "iso8601", "reject") }) {
		t.Error("from fields day 31 in February under reject did not throw")
	}
	if !bagThrows(func() {
		PlainDateTimeFromFields(2020, 1, 31, some(25), none, none, none, none, none, "iso8601", "reject")
	}) {
		t.Error("from fields hour 25 under reject did not throw")
	}
}

func TestPlainDateToPlainDateTime(t *testing.T) {
	d := mustPlainDate(t, 2020, 3, 14)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"no time defaults to midnight", d.ToPlainDateTime(nil).ToString().ToGoString(), "2020-03-14T00:00:00"},
		{"a plain time pairs in", d.ToPlainDateTime(NewPlainTime(15, 30, 45, 0, 0, 0)).ToString().ToGoString(), "2020-03-14T15:30:45"},
		{"subsecond components carry", d.ToPlainDateTime(NewPlainTime(1, 2, 3, 4, 5, 6)).ToString().ToGoString(), "2020-03-14T01:02:03.004005006"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// The result keeps the date's calendar, so a roc date stays under roc.
	roc := PlainDateWithCalendar(mustPlainDate(t, 2024, 5, 15), "roc")
	if got := roc.ToPlainDateTime(nil).ToString().ToGoString(); got != "2024-05-15T00:00:00[u-ca=roc]" {
		t.Errorf("roc toPlainDateTime: got %q, want %q", got, "2024-05-15T00:00:00[u-ca=roc]")
	}
}

func TestPlainDateToPlainYearMonth(t *testing.T) {
	iso := mustPlainDate(t, 2020, 5, 15)
	if got := iso.ToPlainYearMonth().ToString().ToGoString(); got != "2020-05" {
		t.Errorf("iso toPlainYearMonth: got %q, want %q", got, "2020-05")
	}
	if got := iso.ToPlainYearMonth().Year(); got != 2020 {
		t.Errorf("iso toPlainYearMonth year: got %v, want 2020", got)
	}

	// A non-ISO year-month keeps its calendar: the year reads in the calendar's reckoning,
	// the toString carries the reference day and the annotation, and roc counts from 1912.
	roc := PlainDateWithCalendar(iso, "roc")
	if got := roc.ToPlainYearMonth().ToString().ToGoString(); got != "2020-05-01[u-ca=roc]" {
		t.Errorf("roc toPlainYearMonth: got %q, want %q", got, "2020-05-01[u-ca=roc]")
	}
	if got := roc.ToPlainYearMonth().Year(); got != 109 {
		t.Errorf("roc toPlainYearMonth year: got %v, want 109", got)
	}
	greg := PlainDateWithCalendar(iso, "gregory")
	if got := greg.ToPlainYearMonth().ToString().ToGoString(); got != "2020-05-01[u-ca=gregory]" {
		t.Errorf("gregory toPlainYearMonth: got %q, want %q", got, "2020-05-01[u-ca=gregory]")
	}
	if got := greg.ToPlainYearMonth().CalendarId().ToGoString(); got != "gregory" {
		t.Errorf("gregory toPlainYearMonth calendarId: got %q, want gregory", got)
	}

	// equals splits on the calendar even when the ISO year and month match.
	if roc.ToPlainYearMonth().Equals(iso.ToPlainYearMonth()) {
		t.Error("a roc year-month should not equal the ISO year-month with the same fields")
	}
}

func TestPlainDateToPlainMonthDay(t *testing.T) {
	iso := mustPlainDate(t, 2020, 5, 15)
	if got := iso.ToPlainMonthDay().ToString().ToGoString(); got != "05-15" {
		t.Errorf("iso toPlainMonthDay toString: got %q, want 05-15", got)
	}
	if got := iso.ToPlainMonthDay().Day(); got != 15 {
		t.Errorf("iso toPlainMonthDay day: got %v, want 15", got)
	}

	// A non-ISO month-day keeps its calendar: toString shows the reference year 1972,
	// the actual month and day, and the annotation.
	roc := PlainDateWithCalendar(iso, "roc")
	if got := roc.ToPlainMonthDay().ToString().ToGoString(); got != "1972-05-15[u-ca=roc]" {
		t.Errorf("roc toPlainMonthDay toString: got %q, want 1972-05-15[u-ca=roc]", got)
	}
	greg := PlainDateWithCalendar(iso, "gregory")
	if got := greg.ToPlainMonthDay().ToString().ToGoString(); got != "1972-05-15[u-ca=gregory]" {
		t.Errorf("gregory toPlainMonthDay toString: got %q, want 1972-05-15[u-ca=gregory]", got)
	}
	if got := greg.ToPlainMonthDay().CalendarId().ToGoString(); got != "gregory" {
		t.Errorf("gregory toPlainMonthDay calendarId: got %q, want gregory", got)
	}

	// A leap-day month-day round-trips the day in both the ISO and non-ISO forms.
	leap := mustPlainDate(t, 2020, 2, 29)
	if got := leap.ToPlainMonthDay().ToString().ToGoString(); got != "02-29" {
		t.Errorf("iso leap toPlainMonthDay toString: got %q, want 02-29", got)
	}
	if got := PlainDateWithCalendar(leap, "roc").ToPlainMonthDay().ToString().ToGoString(); got != "1972-02-29[u-ca=roc]" {
		t.Errorf("roc leap toPlainMonthDay toString: got %q, want 1972-02-29[u-ca=roc]", got)
	}

	// equals splits on the calendar even when the month and day match.
	if roc.ToPlainMonthDay().Equals(iso.ToPlainMonthDay()) {
		t.Error("a roc month-day should not equal the ISO month-day with the same fields")
	}
}

func TestPlainDateToZonedDateTime(t *testing.T) {
	d := mustPlainDate(t, 2020, 3, 14)

	// A bare time zone pins the date at midnight and resolves the exact instant.
	utc := d.ToZonedDateTime("UTC", nil)
	if got := utc.ToString().ToGoString(); got != "2020-03-14T00:00:00+00:00[UTC]" {
		t.Errorf("utc toZonedDateTime toString: got %q, want 2020-03-14T00:00:00+00:00[UTC]", got)
	}
	if got := utc.EpochMilliseconds(); got != 1584144000000 {
		t.Errorf("utc toZonedDateTime epochMilliseconds: got %v, want 1584144000000", got)
	}

	// A named zone reads the offset in force at the instant.
	ny := d.ToZonedDateTime("America/New_York", nil)
	if got := ny.ToString().ToGoString(); got != "2020-03-14T00:00:00-04:00[America/New_York]" {
		t.Errorf("new york toZonedDateTime toString: got %q, want ...-04:00[America/New_York]", got)
	}

	// A plain time places the wall clock.
	at := d.ToZonedDateTime("America/New_York", NewPlainTime(15, 30, 45, 0, 0, 0))
	if got := at.ToString().ToGoString(); got != "2020-03-14T15:30:45-04:00[America/New_York]" {
		t.Errorf("new york at 15:30:45 toString: got %q, want ...15:30:45-04:00[America/New_York]", got)
	}

	// A spring-forward gap shifts the reading forward under compatible disambiguation:
	// 2020-03-08 02:30 does not exist in America/New_York, so it becomes 03:30.
	gap := mustPlainDate(t, 2020, 3, 8).ToZonedDateTime("America/New_York", NewPlainTime(2, 30, 0, 0, 0, 0))
	if got := gap.ToString().ToGoString(); got != "2020-03-08T03:30:00-04:00[America/New_York]" {
		t.Errorf("gap toZonedDateTime toString: got %q, want ...03:30:00-04:00[America/New_York]", got)
	}

	// A non-ISO date keeps its calendar: the annotation follows the zone bracket, the year
	// getter reads in the calendar's reckoning, and the era reports the calendar.
	roc := PlainDateWithCalendar(d, "roc").ToZonedDateTime("UTC", nil)
	if got := roc.ToString().ToGoString(); got != "2020-03-14T00:00:00+00:00[UTC][u-ca=roc]" {
		t.Errorf("roc toZonedDateTime toString: got %q, want ...[UTC][u-ca=roc]", got)
	}
	if got := roc.Year(); got != 109 {
		t.Errorf("roc toZonedDateTime year: got %v, want 109", got)
	}
	if got := roc.CalendarId().ToGoString(); got != "roc" {
		t.Errorf("roc toZonedDateTime calendarId: got %q, want roc", got)
	}

	// equals splits on the calendar even when the instant and zone match.
	if roc.Equals(utc) {
		t.Error("a roc zoned date-time should not equal the ISO zoned date-time at the same instant and zone")
	}
}

// mustPlainDateTime builds a PlainDateTime and fails the test if construction threw.
func mustPlainDateTime(t *testing.T, a ...float64) *PlainDateTime {
	t.Helper()
	var pdt *PlainDateTime
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewPlainDateTime(%v) threw: %v", a, r)
			}
		}()
		pdt = NewPlainDateTime(a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8])
	}()
	return pdt
}

// plainDateTimeThrows reports whether NewPlainDateTime throws a RangeError for the args.
func plainDateTimeThrows(a [9]float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainDateTime(a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8])
	return false
}

// TestPlainDateTimeFields checks the date and time getters against a date-time with every
// field set, the values taken from @js-temporal/polyfill.
func TestPlainDateTimeFields(t *testing.T) {
	pdt := mustPlainDateTime(t, 1976, 11, 18, 15, 23, 30, 123, 456, 789)
	fields := []struct {
		name string
		got  float64
		want float64
	}{
		{"Year", pdt.Year(), 1976},
		{"Month", pdt.Month(), 11},
		{"Day", pdt.Day(), 18},
		{"Hour", pdt.Hour(), 15},
		{"Minute", pdt.Minute(), 23},
		{"Second", pdt.Second(), 30},
		{"Millisecond", pdt.Millisecond(), 123},
		{"Microsecond", pdt.Microsecond(), 456},
		{"Nanosecond", pdt.Nanosecond(), 789},
		{"DayOfWeek", pdt.DayOfWeek(), 4},
		{"DayOfYear", pdt.DayOfYear(), 323},
		{"DaysInMonth", pdt.DaysInMonth(), 30},
		{"DaysInYear", pdt.DaysInYear(), 366},
		{"MonthsInYear", pdt.MonthsInYear(), 12},
		{"DaysInWeek", pdt.DaysInWeek(), 7},
	}
	for _, f := range fields {
		if f.got != f.want {
			t.Errorf("%s = %v, want %v", f.name, f.got, f.want)
		}
	}
	if got := pdt.MonthCode().ToGoString(); got != "M11" {
		t.Errorf("MonthCode = %q, want M11", got)
	}
	if got := pdt.CalendarId().ToGoString(); got != "iso8601" {
		t.Errorf("CalendarId = %q, want iso8601", got)
	}
	if !pdt.InLeapYear() {
		t.Error("InLeapYear = false, want true")
	}
}

// TestPlainDateTimeToString checks the ISO date-time string, the date and time joined by
// "T", against the polyfill.
func TestPlainDateTimeToString(t *testing.T) {
	cases := []struct {
		args [9]float64
		want string
	}{
		{[9]float64{2020, 1, 1, 12, 30, 0, 0, 0, 0}, "2020-01-01T12:30:00"},
		{[9]float64{1976, 11, 18, 15, 23, 30, 123, 456, 789}, "1976-11-18T15:23:30.123456789"},
		{[9]float64{2020, 2, 29, 0, 0, 0, 0, 0, 0}, "2020-02-29T00:00:00"},
		{[9]float64{2020, 1, 1, 1, 2, 3, 250, 0, 0}, "2020-01-01T01:02:03.25"},
		{[9]float64{-1, 1, 1, 0, 0, 0, 0, 0, 0}, "-000001-01-01T00:00:00"},
		{[9]float64{12345, 1, 1, 0, 0, 0, 0, 0, 0}, "+012345-01-01T00:00:00"},
	}
	for _, c := range cases {
		pdt := mustPlainDateTime(t, c.args[:]...)
		if got := pdt.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainDateTime%v.ToString() = %q, want %q", c.args, got, c.want)
		}
		if got := pdt.ToJSON().ToGoString(); got != c.want {
			t.Errorf("PlainDateTime%v.ToJSON() = %q, want %q", c.args, got, c.want)
		}
	}
}

// TestPlainDateTimeConversions checks toPlainDate and toPlainTime split the date-time into its
// two halves. toPlainDate keeps the calendar and drops the clock; toPlainTime keeps the clock and
// drops the date. Both return a fresh object, verified against @js-temporal/polyfill.
func TestPlainDateTimeConversions(t *testing.T) {
	pdt := mustPlainDateTime(t, 2020, 5, 15, 13, 30, 45, 500, 250, 125)
	if got := pdt.ToPlainDate().ToString().ToGoString(); got != "2020-05-15" {
		t.Errorf("toPlainDate = %q, want %q", got, "2020-05-15")
	}
	if got := pdt.ToPlainTime().ToString().ToGoString(); got != "13:30:45.500250125" {
		t.Errorf("toPlainTime = %q, want %q", got, "13:30:45.500250125")
	}
	// Each conversion returns a distinct object, not an alias of the receiver's halves.
	if &pdt.date == pdt.ToPlainDate() {
		t.Error("toPlainDate aliased the receiver's date half, want a copy")
	}
	if &pdt.time == pdt.ToPlainTime() {
		t.Error("toPlainTime aliased the receiver's time half, want a copy")
	}
}

// TestPlainDateTimeWithPlainTime checks that withPlainTime keeps the calendar date and replaces
// the wall clock, defaulting to midnight when no time is given, and carries the calendar through.
// Every value was checked against @js-temporal/polyfill.
func TestPlainDateTimeWithPlainTime(t *testing.T) {
	pdt := mustPlainDateTime(t, 2020, 5, 15, 13, 30, 45, 500, 250, 125)
	if got := pdt.WithPlainTime(nil).ToString().ToGoString(); got != "2020-05-15T00:00:00" {
		t.Errorf("withPlainTime() = %q, want %q", got, "2020-05-15T00:00:00")
	}
	if got := pdt.WithPlainTime(mustPlainTime(t, 9, 15, 0, 0, 0, 0)).ToString().ToGoString(); got != "2020-05-15T09:15:00" {
		t.Errorf("withPlainTime(09:15) = %q, want %q", got, "2020-05-15T09:15:00")
	}
	// The date half is copied, so the result shares no state with the receiver.
	if &pdt.date == &pdt.WithPlainTime(nil).date {
		t.Error("withPlainTime aliased the receiver's date half, want a copy")
	}
	// The calendar carries through the reshape.
	g := PlainDateTimeWithCalendar(pdt, "gregory")
	if got := g.WithPlainTime(mustPlainTime(t, 9, 15, 0, 0, 0, 0)).ToString().ToGoString(); got != "2020-05-15T09:15:00[u-ca=gregory]" {
		t.Errorf("gregory withPlainTime = %q, want %q", got, "2020-05-15T09:15:00[u-ca=gregory]")
	}
}

// TestPlainDateTimeWithFields checks that with overlays the bag's date and time fields on the
// receiver and regulates each half, keeps the calendar, and throws under reject. Every value was
// checked against @js-temporal/polyfill.
func TestPlainDateTimeWithFields(t *testing.T) {
	dt := mustPlainDateTime(t, 2020, 1, 31, 13, 30, 45, 500, 250, 125)
	some := func(v float64) Opt[float64] { return Some(v) }
	none := None[float64]()
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"month constrains the day", dt.WithFields(none, some(2), none, none, none, none, none, none, none, "constrain").ToString().ToGoString(), "2020-02-29T13:30:45.500250125"},
		{"day only", dt.WithFields(none, none, some(15), none, none, none, none, none, none, "constrain").ToString().ToGoString(), "2020-01-15T13:30:45.500250125"},
		{"time fields", dt.WithFields(none, none, none, some(6), some(0), none, none, none, none, "constrain").ToString().ToGoString(), "2020-01-31T06:00:45.500250125"},
		{"date and time", dt.WithFields(some(2021), some(6), some(10), some(8), some(5), some(3), none, none, none, "constrain").ToString().ToGoString(), "2021-06-10T08:05:03.500250125"},
		{"month over twelve clamps", dt.WithFields(none, some(13), none, none, none, none, none, none, none, "constrain").ToString().ToGoString(), "2020-12-31T13:30:45.500250125"},
		{"hour over the day clamps", dt.WithFields(none, none, none, some(25), none, none, none, none, none, "constrain").ToString().ToGoString(), "2020-01-31T23:30:45.500250125"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// roc reads the bag year in Minguo reckoning, so year 100 is ISO 2011, and the calendar carries.
	roc := PlainDateTimeWithCalendar(mustPlainDateTime(t, 2020, 5, 15, 12, 0, 0, 0, 0, 0), "roc")
	if got := roc.WithFields(some(100), none, none, none, none, none, none, none, none, "constrain").ToString().ToGoString(); got != "2011-05-15T12:00:00[u-ca=roc]" {
		t.Errorf("roc with year: got %q, want %q", got, "2011-05-15T12:00:00[u-ca=roc]")
	}

	// reject throws when a field does not fit its half.
	if !bagThrows(func() { dt.WithFields(none, some(13), none, none, none, none, none, none, none, "reject") }) {
		t.Error("with month 13 under reject did not throw")
	}
	if !bagThrows(func() { dt.WithFields(none, none, none, some(25), none, none, none, none, none, "reject") }) {
		t.Error("with hour 25 under reject did not throw")
	}
}

// TestPlainDateTimeToZonedDateTime checks the wall clock pins to a zone under each
// disambiguation, including a spring-forward gap and a fall-back overlap. Every value was checked
// against @js-temporal/polyfill.
func TestPlainDateTimeToZonedDateTime(t *testing.T) {
	dt := mustPlainDateTime(t, 2020, 3, 14, 15, 30, 45, 0, 0, 0)
	utc := dt.ToZonedDateTime("UTC", "compatible")
	if got := utc.ToString().ToGoString(); got != "2020-03-14T15:30:45+00:00[UTC]" {
		t.Errorf("utc toString = %q, want 2020-03-14T15:30:45+00:00[UTC]", got)
	}
	ny := dt.ToZonedDateTime("America/New_York", "compatible")
	if got := ny.EpochMilliseconds(); got != 1584214245000 {
		t.Errorf("new york epochMilliseconds = %v, want 1584214245000", got)
	}

	// A spring-forward gap: 2020-03-08 02:30 does not exist in America/New_York.
	gap := mustPlainDateTime(t, 2020, 3, 8, 2, 30, 0, 0, 0, 0)
	if got := gap.ToZonedDateTime("America/New_York", "compatible").ToString().ToGoString(); got != "2020-03-08T03:30:00-04:00[America/New_York]" {
		t.Errorf("gap compatible = %q, want ...03:30:00-04:00[America/New_York]", got)
	}
	if got := gap.ToZonedDateTime("America/New_York", "earlier").ToString().ToGoString(); got != "2020-03-08T01:30:00-05:00[America/New_York]" {
		t.Errorf("gap earlier = %q, want ...01:30:00-05:00[America/New_York]", got)
	}
	if got := gap.ToZonedDateTime("America/New_York", "later").ToString().ToGoString(); got != "2020-03-08T03:30:00-04:00[America/New_York]" {
		t.Errorf("gap later = %q, want ...03:30:00-04:00[America/New_York]", got)
	}
	if !plainDateTimeCallThrows(func() { gap.ToZonedDateTime("America/New_York", "reject") }) {
		t.Error("gap reject did not throw")
	}

	// A fall-back overlap: 2020-11-01 01:30 happens twice in America/New_York.
	dup := mustPlainDateTime(t, 2020, 11, 1, 1, 30, 0, 0, 0, 0)
	if got := dup.ToZonedDateTime("America/New_York", "compatible").ToString().ToGoString(); got != "2020-11-01T01:30:00-04:00[America/New_York]" {
		t.Errorf("overlap compatible = %q, want ...01:30:00-04:00[America/New_York]", got)
	}
	if got := dup.ToZonedDateTime("America/New_York", "later").ToString().ToGoString(); got != "2020-11-01T01:30:00-05:00[America/New_York]" {
		t.Errorf("overlap later = %q, want ...01:30:00-05:00[America/New_York]", got)
	}
	if !plainDateTimeCallThrows(func() { dup.ToZonedDateTime("America/New_York", "reject") }) {
		t.Error("overlap reject did not throw")
	}

	// A non-ISO date-time keeps its calendar through the zone bracket.
	g := PlainDateTimeWithCalendar(mustPlainDateTime(t, 2020, 3, 14, 15, 30, 0, 0, 0, 0), "gregory")
	if got := g.ToZonedDateTime("UTC", "compatible").ToString().ToGoString(); got != "2020-03-14T15:30:00+00:00[UTC][u-ca=gregory]" {
		t.Errorf("gregory toString = %q, want ...[UTC][u-ca=gregory]", got)
	}
}

// TestPlainDateTimeCompareAndEquals checks the static comparator and equals, including a
// difference that lives only in the time so the date-first fall-through is exercised.
func TestPlainDateTimeCompareAndEquals(t *testing.T) {
	a := mustPlainDateTime(t, 2020, 1, 1, 12, 30, 0, 0, 0, 0)
	b := mustPlainDateTime(t, 1976, 11, 18, 15, 23, 30, 123, 456, 789)
	c := mustPlainDateTime(t, 2020, 1, 1, 12, 30, 0, 0, 0, 0)
	if got := PlainDateTimeCompare(a, b); got != 1 {
		t.Errorf("compare(a,b) = %v, want 1", got)
	}
	if got := PlainDateTimeCompare(b, a); got != -1 {
		t.Errorf("compare(b,a) = %v, want -1", got)
	}
	if got := PlainDateTimeCompare(a, c); got != 0 {
		t.Errorf("compare(a,c) = %v, want 0", got)
	}
	// Same date, the time alone orders.
	early := mustPlainDateTime(t, 2020, 6, 15, 8, 0, 0, 0, 0, 0)
	late := mustPlainDateTime(t, 2020, 6, 15, 9, 0, 0, 0, 0, 0)
	if got := PlainDateTimeCompare(early, late); got != -1 {
		t.Errorf("compare over the time = %v, want -1", got)
	}
	if !a.Equals(c) {
		t.Error("a.Equals(c) = false, want true")
	}
	if a.Equals(b) {
		t.Error("a.Equals(b) = true, want false")
	}
}

// TestPlainDateTimeFromCopies proves from returns a distinct object that compares equal to
// its source.
func TestPlainDateTimeFromCopies(t *testing.T) {
	a := mustPlainDateTime(t, 2020, 1, 1, 12, 30, 0, 0, 0, 0)
	b := PlainDateTimeFrom(a)
	if a == b {
		t.Error("PlainDateTimeFrom returned the same pointer, want a copy")
	}
	if !a.Equals(b) {
		t.Error("from copy does not equal its source")
	}
}

// TestPlainDateTimeTruncatesArguments proves a fractional argument truncates toward zero,
// matching ToIntegerWithTruncation.
func TestPlainDateTimeTruncatesArguments(t *testing.T) {
	pdt := mustPlainDateTime(t, 2020.9, 1.9, 1.9, 12.9, 30.9, 0, 0, 0, 0)
	if got := pdt.ToString().ToGoString(); got != "2020-01-01T12:30:00" {
		t.Errorf("truncated PlainDateTime = %q, want 2020-01-01T12:30:00", got)
	}
}

// TestPlainDateTimeRejects checks the RangeError cases against the polyfill: an out-of-range
// date component, an out-of-range time component, and a NaN in either half.
func TestPlainDateTimeRejects(t *testing.T) {
	throwing := [][9]float64{
		{2020, 13, 1, 0, 0, 0, 0, 0, 0},
		{2021, 2, 30, 0, 0, 0, 0, 0, 0},
		{2020, 1, 1, 24, 0, 0, 0, 0, 0},
		{2020, 1, 1, 0, 60, 0, 0, 0, 0},
		{2020, 1, 1, 0, 0, 0, 0, 0, 1000},
		{nan(), 1, 1, 0, 0, 0, 0, 0, 0},
		{2020, 1, 1, nan(), 0, 0, 0, 0, 0},
	}
	for _, c := range throwing {
		if !plainDateTimeThrows(c) {
			t.Errorf("NewPlainDateTime%v did not throw", c)
		}
	}
	// The all-max valid boundary must not throw.
	if plainDateTimeThrows([9]float64{2020, 12, 31, 23, 59, 59, 999, 999, 999}) {
		t.Error("NewPlainDateTime at the valid maximum threw")
	}
}

// TestPlainDateTimeArithmetic checks add, subtract, until, since, and round against the
// polyfill: the time part folds into the wall clock and carries a whole day into the date, the
// difference borrows a day when the time part points against the calendar direction, and round
// carries a day past midnight. The reject overflow throws.
func TestPlainDateTimeArithmetic(t *testing.T) {
	ck := func(got BStr, want string) {
		t.Helper()
		if got.ToGoString() != want {
			t.Errorf("got %q, want %q", got.ToGoString(), want)
		}
	}
	base := mustPlainDateTime(t, 2020, 1, 31, 12, 30, 45, 0, 0, 0)
	ck(base.AddDateTime(mustDuration(t, 0, 1), "constrain").ToString(), "2020-02-29T12:30:45")
	ck(base.AddDateTime(mustDuration(t, 4, 1), "constrain").ToString(), "2024-02-29T12:30:45")
	ck(base.AddDateTime(mustDuration(t, 0, 0, 0, 0, 13), "constrain").ToString(), "2020-02-01T01:30:45")
	ck(base.AddDateTime(mustDuration(t, 0, 0, 0, 1, 25), "constrain").ToString(), "2020-02-02T13:30:45")
	ck(base.AddDateTime(mustDuration(t, 0, 0, 0, 0, -13), "constrain").ToString(), "2020-01-30T23:30:45")
	ck(base.AddDateTime(mustDuration(t, 0, 1).Negated(), "constrain").ToString(), "2019-12-31T12:30:45")

	a := mustPlainDateTime(t, 2020, 1, 1, 12, 0, 0, 0, 0, 0)
	b := mustPlainDateTime(t, 2020, 1, 2, 6, 0, 0, 0, 0, 0)
	ck(a.Until(b, "day").ToString(), "PT18H")
	ck(a.Since(b, "day").ToString(), "-PT18H")

	c := mustPlainDateTime(t, 2020, 1, 31, 12, 30, 45, 0, 0, 0)
	d := mustPlainDateTime(t, 2021, 3, 30, 18, 45, 50, 500, 0, 0)
	ck(c.Until(d, "day").ToString(), "P424DT6H15M5.5S")
	ck(c.Until(d, "year").ToString(), "P1Y1M30DT6H15M5.5S")
	ck(c.Until(d, "month").ToString(), "P13M30DT6H15M5.5S")
	ck(c.Until(d, "week").ToString(), "P60W4DT6H15M5.5S")
	ck(c.Until(d, "hour").ToString(), "PT10182H15M5.5S")
	ck(d.Since(c, "year").ToString(), "P1Y1M29DT6H15M5.5S")
	ck(c.Until(c, "day").ToString(), "PT0S")

	e := mustPlainDateTime(t, 2020, 3, 1, 6, 0, 0, 0, 0, 0)
	f := mustPlainDateTime(t, 2020, 4, 1, 2, 0, 0, 0, 0, 0)
	ck(e.Until(f, "month").ToString(), "P30DT20H")

	rb := mustPlainDateTime(t, 2020, 1, 31, 3, 34, 56, 987, 654, 321)
	ck(rb.Round("day", 1, "halfExpand").ToString(), "2020-01-31T00:00:00")
	ck(mustPlainDateTime(t, 2020, 1, 31, 18, 0, 0, 0, 0, 0).Round("day", 1, "halfExpand").ToString(), "2020-02-01T00:00:00")
	ck(rb.Round("hour", 1, "halfExpand").ToString(), "2020-01-31T04:00:00")
	ck(rb.Round("minute", 15, "halfExpand").ToString(), "2020-01-31T03:30:00")
	ck(mustPlainDateTime(t, 2020, 1, 31, 23, 59, 59, 0, 0, 0).Round("minute", 1, "halfExpand").ToString(), "2020-02-01T00:00:00")

	if !plainDateTimeCallThrows(func() { base.AddDateTime(mustDuration(t, 0, 1), "reject") }) {
		t.Error("add with overflow reject did not throw")
	}
	if !plainDateTimeCallThrows(func() { rb.Round("day", 2, "halfExpand") }) {
		t.Error("round to day with increment 2 did not throw")
	}
	if !plainDateTimeCallThrows(func() { rb.Round("hour", 5, "halfExpand") }) {
		t.Error("round to hour with increment 5 did not throw")
	}
}

// plainDateTimeCallThrows reports whether fn throws a Temporal RangeError.
func plainDateTimeCallThrows(fn func()) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	fn()
	return false
}

// mustDuration builds a Duration and fails the test if construction threw.
func mustDuration(t *testing.T, a ...float64) *Duration {
	t.Helper()
	f := [10]float64{}
	copy(f[:], a)
	var d *Duration
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewDuration(%v) threw: %v", a, r)
			}
		}()
		d = NewDuration(f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9])
	}()
	return d
}

// durationThrows reports whether NewDuration throws a RangeError for the args.
func durationThrows(a [10]float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewDuration(a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9])
	return false
}

// TestDurationFields checks the ten field getters against a duration with every field set,
// the values taken from @js-temporal/polyfill.
func TestDurationFields(t *testing.T) {
	d := mustDuration(t, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	fields := []struct {
		name string
		got  float64
		want float64
	}{
		{"Years", d.Years(), 1},
		{"Months", d.Months(), 2},
		{"Weeks", d.Weeks(), 3},
		{"Days", d.Days(), 4},
		{"Hours", d.Hours(), 5},
		{"Minutes", d.Minutes(), 6},
		{"Seconds", d.Seconds(), 7},
		{"Milliseconds", d.Milliseconds(), 8},
		{"Microseconds", d.Microseconds(), 9},
		{"Nanoseconds", d.Nanoseconds(), 10},
		{"Sign", d.Sign(), 1},
	}
	for _, f := range fields {
		if f.got != f.want {
			t.Errorf("%s = %v, want %v", f.name, f.got, f.want)
		}
	}
	if d.Blank() {
		t.Error("Blank = true, want false")
	}
}

// TestDurationSignAndBlank checks sign and blank across a positive, a negative, and an
// empty duration, and that the sign follows the first non-zero field.
func TestDurationSignAndBlank(t *testing.T) {
	empty := mustDuration(t)
	if empty.Sign() != 0 || !empty.Blank() {
		t.Errorf("empty: sign = %v, blank = %v, want 0 and true", empty.Sign(), empty.Blank())
	}
	pos := mustDuration(t, 0, 0, 1)
	if pos.Sign() != 1 || pos.Blank() {
		t.Errorf("positive: sign = %v, blank = %v, want 1 and false", pos.Sign(), pos.Blank())
	}
	neg := mustDuration(t, 0, 0, 0, -4)
	if neg.Sign() != -1 || neg.Blank() {
		t.Errorf("negative: sign = %v, blank = %v, want -1 and false", neg.Sign(), neg.Blank())
	}
	// A negative-zero component counts as zero, so a duration of only -0 is blank.
	negZero := mustDuration(t, 0, 0, 0, math.Copysign(0, -1))
	if negZero.Sign() != 0 || !negZero.Blank() {
		t.Errorf("negative zero: sign = %v, blank = %v, want 0 and true", negZero.Sign(), negZero.Blank())
	}
}

// TestDurationToString checks the ISO 8601 duration string, including the combined
// fractional seconds and the empty "PT0S", against the polyfill.
func TestDurationToString(t *testing.T) {
	cases := []struct {
		args []float64
		want string
	}{
		{[]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, "P1Y2M3W4DT5H6M7.00800901S"},
		{[]float64{5}, "P5Y"},
		{[]float64{0, 0, 3}, "P3W"},
		{[]float64{0, 0, 0, 4, 0, 0, 0, 500}, "P4DT0.5S"},
		{[]float64{0, 0, 0, 0, 0, 0, 0, 0, 0, 500}, "PT0.0000005S"},
		{[]float64{0, 0, 0, 0, 0, 90}, "PT90M"},
		{[]float64{0, 0, 0, 0, 0, 0, -1, -500}, "-PT1.5S"},
		{[]float64{0, 0, 0, 0, 0, 0, 0, 0, 7}, "PT0.000007S"},
		{[]float64{0, 0, 0, 0, 0, 0, 0, 7}, "PT0.007S"},
		{[]float64{0, 0, 0, 0, 0, 0, 0, 1500}, "PT1.5S"},
		{nil, "PT0S"},
	}
	for _, c := range cases {
		d := mustDuration(t, c.args...)
		if got := d.ToString().ToGoString(); got != c.want {
			t.Errorf("Duration(%v).ToString() = %q, want %q", c.args, got, c.want)
		}
		if got := d.ToJSON().ToGoString(); got != c.want {
			t.Errorf("Duration(%v).ToJSON() = %q, want %q", c.args, got, c.want)
		}
	}
}

// TestDurationNegatedAndAbs checks that negated flips every sign and abs makes every field
// non-negative, and that neither mutates the receiver.
func TestDurationNegatedAndAbs(t *testing.T) {
	d := mustDuration(t, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10)
	neg := d.Negated()
	if got := neg.ToString().ToGoString(); got != "-P1Y2M3W4DT5H6M7.00800901S" {
		t.Errorf("negated = %q, want -P1Y2M3W4DT5H6M7.00800901S", got)
	}
	if neg.Sign() != -1 {
		t.Errorf("negated sign = %v, want -1", neg.Sign())
	}
	back := neg.Abs()
	if got := back.ToString().ToGoString(); got != "P1Y2M3W4DT5H6M7.00800901S" {
		t.Errorf("abs of negated = %q, want the positive form", got)
	}
	if d.Sign() != 1 {
		t.Error("negated or abs mutated the receiver's sign")
	}
}

// TestDurationFromCopies proves from returns a distinct object equal to its source.
func TestDurationFromCopies(t *testing.T) {
	a := mustDuration(t, 1, 2, 3)
	b := DurationFrom(a)
	if a == b {
		t.Error("DurationFrom returned the same pointer, want a copy")
	}
	if a.ToString().ToGoString() != b.ToString().ToGoString() {
		t.Error("from copy does not equal its source")
	}
}

// TestDurationWithAndFrom checks Temporal.Duration.prototype.with overlays the present fields
// onto the receiver and Temporal.Duration.from over a bag defaults the absent ones to zero,
// each rejecting an empty bag with a TypeError and a fractional or mixed-sign field with a
// RangeError, every value checked against @js-temporal/polyfill.
func TestDurationWithAndFrom(t *testing.T) {
	some := Some[float64]
	none := None[float64]()
	base := mustDuration(t, 1, 2, 0, 3, 4) // P1Y2M3DT4H
	if got := base.With(none, some(5), none, none, none, none, none, none, none, none).ToString().ToGoString(); got != "P1Y5M3DT4H" {
		t.Errorf("with months = %q, want P1Y5M3DT4H", got)
	}
	if got := base.With(none, none, none, some(10), some(0), none, none, none, none, none).ToString().ToGoString(); got != "P1Y2M10D" {
		t.Errorf("with days and zero hours = %q, want P1Y2M10D", got)
	}
	if got := base.With(some(-1), some(-2), none, some(-3), some(-4), none, none, none, none, none).ToString().ToGoString(); got != "-P1Y2M3DT4H" {
		t.Errorf("with negated = %q, want -P1Y2M3DT4H", got)
	}
	if base.ToString().ToGoString() != "P1Y2M3DT4H" {
		t.Error("with mutated the receiver")
	}
	if got := DurationFromFields(none, none, none, none, some(1), some(30), none, none, none, none).ToString().ToGoString(); got != "PT1H30M" {
		t.Errorf("from bag = %q, want PT1H30M", got)
	}
	if got := DurationFromFields(none, none, none, some(-2), some(-3), none, none, none, none, none).ToString().ToGoString(); got != "-P2DT3H" {
		t.Errorf("from bag negative = %q, want -P2DT3H", got)
	}
	if got := DurationFromFields(none, none, none, none, none, some(90), none, none, none, none).ToString().ToGoString(); got != "PT90M" {
		t.Errorf("from bag unbalanced = %q, want PT90M", got)
	}
	assertTypeError := func(name string, fn func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not throw", name)
				return
			}
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
		}()
		fn()
	}
	assertTypeError("with empty", func() { base.With(none, none, none, none, none, none, none, none, none, none) })
	assertTypeError("with fractional", func() { base.With(none, some(1.5), none, none, none, none, none, none, none, none) })
	assertTypeError("with mixed sign", func() { base.With(some(1), some(-1), none, none, none, none, none, none, none, none) })
	assertTypeError("from empty", func() {
		DurationFromFields(none, none, none, none, none, none, none, none, none, none)
	})
	assertTypeError("from fractional", func() {
		DurationFromFields(none, some(1.5), none, none, none, none, none, none, none, none)
	})
}

// TestDurationAddSubtract checks add and subtract against the polyfill: the reduced profile
// takes no relativeTo, so both fold days and time over a fixed 24-hour day, balance to the
// coarser of the two operands' default largest units, and throw a RangeError when either
// operand carries years, months, or weeks.
func TestDurationAddSubtract(t *testing.T) {
	d := func(a ...float64) *Duration { return mustDuration(t, a...) }
	// P2D + PT50H = P4DT2H (98 hours balanced back over a 24-hour day).
	if got := d(0, 0, 0, 2).Add(d(0, 0, 0, 0, 50)).ToString().ToGoString(); got != "P4DT2H" {
		t.Errorf("days + hours = %q, want P4DT2H", got)
	}
	// PT1H30M + PT30M = PT2H, both operands coarsest at the hour so no day appears.
	if got := d(0, 0, 0, 0, 1, 30).Add(d(0, 0, 0, 0, 0, 30)).ToString().ToGoString(); got != "PT2H" {
		t.Errorf("hours + minutes = %q, want PT2H", got)
	}
	// P2DT3H - PT5H = P1DT22H.
	if got := d(0, 0, 0, 2, 3).Subtract(d(0, 0, 0, 0, 5)).ToString().ToGoString(); got != "P1DT22H" {
		t.Errorf("subtract = %q, want P1DT22H", got)
	}
	base := d(0, 0, 0, 2)
	base.Add(d(0, 0, 0, 0, 1))
	if base.ToString().ToGoString() != "P2D" {
		t.Error("add mutated the receiver")
	}
	assertRangeError := func(name string, fn func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not throw", name)
				return
			}
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
		}()
		fn()
	}
	assertRangeError("add months operand", func() { d(0, 0, 0, 1).Add(d(0, 1)) })
	assertRangeError("add weeks receiver", func() { d(0, 0, 1).Add(d(0, 0, 0, 1)) })
	assertRangeError("subtract years", func() { d(1).Subtract(d(0, 0, 0, 1)) })
}

// TestDurationTotal checks total against the polyfill. Without a relativeTo reference the
// duration must be day-or-finer with no calendar units and unit day or smaller, days counting
// as a fixed 24 hours; with a PlainDate reference the fixed units divide the span and the
// irregular month and year interpolate between their boundaries.
func TestDurationTotal(t *testing.T) {
	d := func(a ...float64) *Duration { return mustDuration(t, a...) }
	rel := NewPlainDate(2024, 1, 1)
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"hour no rel", d(0, 0, 0, 1, 1).Total("hour", nil), 25},
		{"day no rel", d(0, 0, 0, 1, 12).Total("day", nil), 1.5},
		{"month rel years", d(1, 2).Total("month", rel), 14},
		{"day rel years", d(1).Total("day", rel), 366},
		{"week rel", d(0, 0, 0, 20).Total("week", rel), 2.857142857142857},
		{"year rel months", d(0, 18).Total("year", rel), 1.4958904109589042},
		{"month rel days+time", d(0, 1, 0, 0, 12).Total("month", rel), 1.0172413793103448},
		{"year neg", d(0, -1, 0, -15).Total("month", rel), -1.5},
		{"day rel with time", d(0, 0, 0, 1, 13).Total("day", rel), 1.5416666666666667},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	assertRangeError := func(name string, fn func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not throw", name)
				return
			}
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
		}()
		fn()
	}
	assertRangeError("week unit no rel", func() { d(0, 0, 0, 20).Total("week", nil) })
	assertRangeError("weeks present no rel", func() { d(0, 0, 2).Total("day", nil) })
	assertRangeError("months present no rel", func() { d(0, 1).Total("hour", nil) })
	assertRangeError("month unit no rel", func() { d(0, 2).Total("month", nil) })
}

// TestDurationCompare checks compare against the polyfill: without a reference the day-and-time
// durations order on a fixed 24-hour day and a calendar unit throws, and with a PlainDate
// reference the calendar resolves both endpoints before ordering.
func TestDurationCompare(t *testing.T) {
	d := func(a ...float64) *Duration { return mustDuration(t, a...) }
	rel := NewPlainDate(2024, 1, 1)
	cases := []struct {
		name string
		got  float64
		want float64
	}{
		{"time no rel", DurationCompare(d(0, 0, 0, 0, 2), d(0, 0, 0, 0, 0, 90), nil), 1},
		{"equal no rel", DurationCompare(d(0, 0, 0, 1), d(0, 0, 0, 0, 24), nil), 0},
		{"days no rel", DurationCompare(d(0, 0, 0, 1), d(0, 0, 0, 2), nil), -1},
		{"cal rel", DurationCompare(d(0, 1), d(0, 0, 0, 20), rel), 1},
		{"cal rel equal", DurationCompare(d(0, 1), d(0, 0, 0, 31), rel), 0},
		{"cal rel neg", DurationCompare(d(0, -1), d(0, 0, 0, -20), rel), -1},
		{"years rel", DurationCompare(d(1), d(0, 0, 0, 365), rel), 1},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
	assertRangeError := func(name string, fn func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not throw", name)
				return
			}
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
		}()
		fn()
	}
	assertRangeError("cal no rel", func() { DurationCompare(d(0, 1), d(0, 0, 0, 1), nil) })
	assertRangeError("weeks no rel", func() { DurationCompare(d(0, 0, 1), d(0, 0, 0, 1), nil) })
}

// TestDurationRound checks round against the polyfill: without a reference the day-and-time
// duration rounds over a fixed 24-hour day and balances to largestUnit, with a calendar unit or
// week largestUnit throwing; with a PlainDate reference the endpoint rounds at smallestUnit,
// irregular units bracketing between two boundaries and negative durations rounding in
// wall-clock terms, then rebalances to largestUnit.
func TestDurationRound(t *testing.T) {
	d := func(a ...float64) *Duration { return mustDuration(t, a...) }
	rel := NewPlainDate(2024, 1, 1)
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"day no rel", d(0, 0, 0, 1, 12).Round("day", "", 1, "halfExpand", nil).ToString().ToGoString(), "P2D"},
		{"day down no rel", d(0, 0, 0, 1, 11).Round("day", "", 1, "halfExpand", nil).ToString().ToGoString(), "P1D"},
		{"hour no rel", d(0, 0, 0, 0, 1, 30).Round("hour", "", 1, "halfExpand", nil).ToString().ToGoString(), "PT2H"},
		{"largest hour no rel", d(0, 0, 0, 1, 2).Round("", "hour", 1, "halfExpand", nil).ToString().ToGoString(), "PT26H"},
		{"largest day from hours", d(0, 0, 0, 0, 50).Round("", "day", 1, "halfExpand", nil).ToString().ToGoString(), "P2DT2H"},
		{"minute inc 15", d(0, 0, 0, 0, 0, 37).Round("minute", "", 15, "halfExpand", nil).ToString().ToGoString(), "PT30M"},
		{"hour floor no rel", d(0, 0, 0, 0, 1, 59).Round("hour", "", 1, "floor", nil).ToString().ToGoString(), "PT1H"},
		{"month rel keeps year", d(1, 2).Round("month", "", 1, "halfExpand", rel).ToString().ToGoString(), "P1Y2M"},
		{"year rel", d(1, 2).Round("year", "", 1, "halfExpand", rel).ToString().ToGoString(), "P1Y"},
		{"year rel down to zero", d(0, 5).Round("year", "", 1, "halfExpand", rel).ToString().ToGoString(), "PT0S"},
		{"largest year rel", d(0, 25).Round("", "year", 1, "halfExpand", rel).ToString().ToGoString(), "P2Y1M"},
		{"largest month from days", d(0, 0, 0, 70).Round("", "month", 1, "halfExpand", rel).ToString().ToGoString(), "P2M10D"},
		{"week rel", d(0, 0, 0, 20).Round("week", "", 1, "halfExpand", rel).ToString().ToGoString(), "P3W"},
		{"day rel mixed", d(0, 1, 0, 15, 12).Round("day", "", 1, "halfExpand", rel).ToString().ToGoString(), "P1M16D"},
		{"month rel negative", d(0, 0, 0, -40).Round("month", "", 1, "halfExpand", rel).ToString().ToGoString(), "-P1M"},
		{"month rel inc 2", d(0, 5).Round("month", "", 2, "halfExpand", rel).ToString().ToGoString(), "P6M"},
		{"largest year sm day", d(0, 25, 0, 5).Round("day", "year", 1, "halfExpand", rel).ToString().ToGoString(), "P2Y1M5D"},
		{"hour from calendar", d(0, 0, 0, 1, 1, 40).Round("hour", "", 1, "halfExpand", rel).ToString().ToGoString(), "P1DT2H"},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	assertRangeError := func(name string, fn func()) {
		defer func() {
			r := recover()
			if r == nil {
				t.Errorf("%s did not throw", name)
				return
			}
			if _, ok := r.(Thrown); !ok {
				panic(r)
			}
		}()
		fn()
	}
	assertRangeError("largest week no rel", func() { d(0, 0, 0, 20).Round("", "week", 1, "halfExpand", nil) })
	assertRangeError("month unit no rel", func() { d(0, 0, 0, 20).Round("month", "", 1, "halfExpand", nil) })
	assertRangeError("weeks present no rel", func() { d(0, 0, 2).Round("day", "", 1, "halfExpand", nil) })
}

// TestDurationRejects checks the RangeError cases against the polyfill: a non-integral
// component (Duration rejects rather than truncates), a NaN or non-finite component, a
// mixed-sign set, a years field at 2^32, and a seconds field at 2^53.
func TestDurationRejects(t *testing.T) {
	throwing := [][10]float64{
		{1.5},                       // non-integral
		{nan()},                     // NaN
		{inf(1)},                    // non-finite
		{1, -2},                     // mixed sign
		{1 << 32},                   // years at the 2^32 bound
		{0, 0, 0, 0, 0, 0, 1 << 53}, // seconds at the 2^53 bound
	}
	for _, c := range throwing {
		if !durationThrows(c) {
			t.Errorf("NewDuration%v did not throw", c)
		}
	}
	valid := [][10]float64{
		{(1 << 32) - 1},                     // just under the years bound
		{0, 0, 0, 0, 0, 0, (1 << 53) - 1},   // just under the seconds bound
		{0, 0, 0, 0, 0, 0, 0, 1 << 53},      // milliseconds far above 2^53 still normalizes in range
		{0, 0, 0, math.Copysign(0, -1), -4}, // a negative-zero field does not clash with a negative field
	}
	for _, c := range valid {
		if durationThrows(c) {
			t.Errorf("NewDuration%v threw at a valid value", c)
		}
	}
}

// yearMonthThrows reports whether NewPlainYearMonth throws a RangeError for the args.
func yearMonthThrows(y, m float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainYearMonth(y, m)
	return false
}

// TestPlainYearMonthFields checks the clean getters against the leap year-month 2020-02,
// the values taken from @js-temporal/polyfill.
func TestPlainYearMonthFields(t *testing.T) {
	ym := NewPlainYearMonth(2020, 2)
	if ym.Year() != 2020 || ym.Month() != 2 {
		t.Errorf("year/month = %v/%v, want 2020/2", ym.Year(), ym.Month())
	}
	if got := ym.MonthCode().ToGoString(); got != "M02" {
		t.Errorf("monthCode = %q, want M02", got)
	}
	if got := ym.CalendarId().ToGoString(); got != "iso8601" {
		t.Errorf("calendarId = %q, want iso8601", got)
	}
	if ym.DaysInMonth() != 29 || ym.DaysInYear() != 366 || ym.MonthsInYear() != 12 || !ym.InLeapYear() {
		t.Errorf("derived fields = %v/%v/%v/%v, want 29/366/12/true", ym.DaysInMonth(), ym.DaysInYear(), ym.MonthsInYear(), ym.InLeapYear())
	}
	nonLeap := NewPlainYearMonth(2021, 2)
	if nonLeap.DaysInMonth() != 28 || nonLeap.DaysInYear() != 365 || nonLeap.InLeapYear() {
		t.Errorf("non-leap 2021-02 = %v/%v/%v, want 28/365/false", nonLeap.DaysInMonth(), nonLeap.DaysInYear(), nonLeap.InLeapYear())
	}
}

// TestPlainYearMonthToString checks the ISO 8601 rendering, including the expanded-year form
// beyond 0..9999, against @js-temporal/polyfill.
func TestPlainYearMonthToString(t *testing.T) {
	cases := []struct {
		y, m float64
		want string
	}{
		{2020, 3, "2020-03"},
		{-1, 5, "-000001-05"},
		{10000, 5, "+010000-05"},
	}
	for _, c := range cases {
		ym := NewPlainYearMonth(c.y, c.m)
		if got := ym.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainYearMonth(%v,%v).ToString() = %q, want %q", c.y, c.m, got, c.want)
		}
		if got := ym.ToJSON().ToGoString(); got != c.want {
			t.Errorf("PlainYearMonth(%v,%v).ToJSON() = %q, want %q", c.y, c.m, got, c.want)
		}
	}
}

// TestPlainYearMonthCompareAndEquals checks the static comparator and equals.
func TestPlainYearMonthCompareAndEquals(t *testing.T) {
	a := NewPlainYearMonth(2020, 3)
	if got := PlainYearMonthCompare(a, NewPlainYearMonth(2020, 4)); got != -1 {
		t.Errorf("compare(2020-03, 2020-04) = %v, want -1", got)
	}
	if got := PlainYearMonthCompare(a, NewPlainYearMonth(2019, 12)); got != 1 {
		t.Errorf("compare(2020-03, 2019-12) = %v, want 1", got)
	}
	if got := PlainYearMonthCompare(a, NewPlainYearMonth(2020, 3)); got != 0 {
		t.Errorf("compare(2020-03, 2020-03) = %v, want 0", got)
	}
	if !a.Equals(NewPlainYearMonth(2020, 3)) || a.Equals(NewPlainYearMonth(2020, 4)) {
		t.Error("equals mismatch")
	}
}

// TestPlainYearMonthFromCopies checks that from returns a distinct equal object.
func TestPlainYearMonthFromCopies(t *testing.T) {
	a := NewPlainYearMonth(2020, 3)
	b := PlainYearMonthFrom(a)
	if a == b {
		t.Error("PlainYearMonthFrom returned the same pointer")
	}
	if !a.Equals(b) {
		t.Error("copy does not equal its source")
	}
}

// TestPlainYearMonthTruncatesAndRejects checks that a fractional argument truncates and that
// the out-of-range and out-of-limit cases throw, the bounds taken from @js-temporal/polyfill.
func TestPlainYearMonthTruncatesAndRejects(t *testing.T) {
	if got := NewPlainYearMonth(2020, 1.5).Month(); got != 1 {
		t.Errorf("month 1.5 truncated to %v, want 1", got)
	}
	throwing := [][2]float64{
		{2020, 0},    // month below 1
		{2020, 13},   // month above 12
		{nan(), 1},   // NaN year
		{-271821, 3}, // before the low limit
		{275760, 10}, // after the high limit
	}
	for _, c := range throwing {
		if !yearMonthThrows(c[0], c[1]) {
			t.Errorf("NewPlainYearMonth%v did not throw", c)
		}
	}
	valid := [][2]float64{
		{-271821, 4}, // at the low limit
		{275760, 9},  // at the high limit
	}
	for _, c := range valid {
		if yearMonthThrows(c[0], c[1]) {
			t.Errorf("NewPlainYearMonth%v threw at a valid value", c)
		}
	}
}

// TestPlainYearMonthArithmetic checks add, subtract, until, and since against
// @js-temporal/polyfill, including the reference-day rule that lets a backward step clamp a
// long month end and the years-and-months difference.
func TestPlainYearMonthArithmetic(t *testing.T) {
	d := func(a ...float64) *Duration { return mustDuration(t, a...) }
	ym := func(y, m float64) *PlainYearMonth { return NewPlainYearMonth(y, m) }
	cases := []struct {
		got  *PlainYearMonth
		want string
	}{
		{ym(2024, 1).AddDuration(d(0, 1), "constrain"), "2024-02"},
		{ym(2024, 1).AddDuration(d(1), "constrain"), "2025-01"},
		{ym(2024, 1).AddDuration(d(0, 13), "constrain"), "2025-02"},
		{ym(2024, 1).AddDuration(d(1, 2), "constrain"), "2025-03"},
		{ym(2024, 1).AddDuration(d(0, 0, 0, 31), "constrain"), "2024-02"},
		{ym(2024, 1).AddDuration(d(0, 0, 0, 0, 48), "constrain"), "2024-01"},
		{ym(2024, 12).AddDuration(d(0, 1), "constrain"), "2025-01"},
		{ym(2024, 1).SubtractDuration(d(0, 1), "constrain"), "2023-12"},
		{ym(2024, 1).SubtractDuration(d(2, 3), "constrain"), "2021-10"},
		{ym(2024, 3).SubtractDuration(d(0, 1), "constrain"), "2024-02"},
		{ym(2024, 1).SubtractDuration(d(0, 0, 0, 1), "constrain"), "2024-01"},
	}
	for i, c := range cases {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("case %d = %q, want %q", i, got, c.want)
		}
	}

	diffs := []struct {
		got  *Duration
		want string
	}{
		{ym(2024, 1).Until(ym(2025, 6), "year"), "P1Y5M"},
		{ym(2024, 1).Until(ym(2025, 6), "month"), "P17M"},
		{ym(2024, 1).Since(ym(2025, 6), "year"), "-P1Y5M"},
		{ym(2025, 6).Until(ym(2024, 1), "year"), "-P1Y5M"},
		{ym(2024, 1).Until(ym(2024, 1), "year"), "PT0S"},
		{ym(2020, 1).Until(ym(2023, 7), "year"), "P3Y6M"},
	}
	for i, c := range diffs {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("diff %d = %q, want %q", i, got, c.want)
		}
	}

	// A subtract that lands on a clamped month end throws under reject.
	throws := func(fn func()) (thrown bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(Thrown); ok {
					thrown = true
				}
			}
		}()
		fn()
		return false
	}
	if !throws(func() { ym(2024, 3).SubtractDuration(d(0, 1), "reject") }) {
		t.Error("2024-03 subtract P1M reject did not throw")
	}
}

// TestPlainYearMonthWithAndToPlainDate covers with overlaying present year and month fields with
// the overflow option, and toPlainDate combining the year-month with a constrained day.
func TestPlainYearMonthWithAndToPlainDate(t *testing.T) {
	ym := func(y, m float64) *PlainYearMonth { return NewPlainYearMonth(y, m) }
	some := func(v float64) Opt[float64] { return Some[float64](v) }
	none := None[float64]()
	withs := []struct {
		got  *PlainYearMonth
		want string
	}{
		{ym(2020, 3).WithFields(none, some(11), "constrain"), "2020-11"},
		{ym(2020, 3).WithFields(some(1999), none, "constrain"), "1999-03"},
		{ym(2020, 3).WithFields(some(2021), some(2), "constrain"), "2021-02"},
		{ym(2020, 3).WithFields(none, some(13), "constrain"), "2020-12"},
		{ym(2020, 3).WithFields(none, some(7), "constrain"), "2020-07"},
	}
	for i, c := range withs {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("with %d = %q, want %q", i, got, c.want)
		}
	}

	dates := []struct {
		got  *PlainDate
		want string
	}{
		{ym(2020, 3).ToPlainDate(15), "2020-03-15"},
		{ym(2020, 3).ToPlainDate(31), "2020-03-31"},
		{ym(2020, 2).ToPlainDate(31), "2020-02-29"},
		{ym(2021, 2).ToPlainDate(31), "2021-02-28"},
	}
	for i, c := range dates {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("toPlainDate %d = %q, want %q", i, got, c.want)
		}
	}

	throws := func(fn func()) (thrown bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(Thrown); ok {
					thrown = true
				}
			}
		}()
		fn()
		return false
	}
	if !throws(func() { ym(2020, 3).WithFields(none, some(13), "reject") }) {
		t.Error("2020-03 with month 13 reject did not throw")
	}
}

// TestPlainYearMonthEra covers era and eraYear resolving at the first of the month, undefined under
// ISO and named under the hosted non-ISO calendars.
func TestPlainYearMonthEra(t *testing.T) {
	withCal := func(y, m float64, cal string) *PlainYearMonth {
		return PlainDateWithCalendar(NewPlainDate(y, m, 15), cal).ToPlainYearMonth()
	}
	if era := NewPlainYearMonth(2020, 3).Era(); !era.IsUndefined() {
		t.Errorf("iso era = %v, want undefined", era)
	}
	if eraYear := NewPlainYearMonth(2020, 3).EraYear(); !eraYear.IsUndefined() {
		t.Errorf("iso eraYear = %v, want undefined", eraYear)
	}
	eras := []struct {
		ym      *PlainYearMonth
		era     string
		eraYear float64
	}{
		{withCal(2020, 3, "gregory"), "gregory", 2020},
		{withCal(0, 3, "gregory"), "gregory-inverse", 1},
		{withCal(2020, 3, "roc"), "roc", 109},
		{withCal(2020, 3, "japanese"), "reiwa", 2},
	}
	for i, c := range eras {
		if era := c.ym.Era(); era.IsUndefined() || era.Get().ToGoString() != c.era {
			t.Errorf("era %d = %v, want %q", i, era, c.era)
		}
		if ey := c.ym.EraYear(); ey.IsUndefined() || ey.Get() != c.eraYear {
			t.Errorf("eraYear %d = %v, want %v", i, ey, c.eraYear)
		}
	}
}

// monthDayThrows reports whether NewPlainMonthDay throws a RangeError for the args.
func monthDayThrows(m, d float64) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainMonthDay(m, d)
	return false
}

// TestPlainMonthDayFields checks the getters and rendering, including the leap-reference
// February 29, against @js-temporal/polyfill.
func TestPlainMonthDayFields(t *testing.T) {
	md := NewPlainMonthDay(3, 15)
	if got := md.MonthCode().ToGoString(); got != "M03" {
		t.Errorf("monthCode = %q, want M03", got)
	}
	if md.Day() != 15 {
		t.Errorf("day = %v, want 15", md.Day())
	}
	if got := md.CalendarId().ToGoString(); got != "iso8601" {
		t.Errorf("calendarId = %q, want iso8601", got)
	}
	if got := md.ToString().ToGoString(); got != "03-15" {
		t.Errorf("toString = %q, want 03-15", got)
	}
	if got := md.ToJSON().ToGoString(); got != "03-15" {
		t.Errorf("toJSON = %q, want 03-15", got)
	}
	feb29 := NewPlainMonthDay(2, 29)
	if got := feb29.ToString().ToGoString(); got != "02-29" {
		t.Errorf("Feb 29 toString = %q, want 02-29", got)
	}
}

// TestPlainMonthDayEqualsAndFrom checks equals and the copy from makes.
func TestPlainMonthDayEqualsAndFrom(t *testing.T) {
	a := NewPlainMonthDay(3, 15)
	if !a.Equals(NewPlainMonthDay(3, 15)) || a.Equals(NewPlainMonthDay(3, 16)) {
		t.Error("equals mismatch")
	}
	b := PlainMonthDayFrom(a)
	if a == b {
		t.Error("PlainMonthDayFrom returned the same pointer")
	}
	if !a.Equals(b) {
		t.Error("copy does not equal its source")
	}
}

// TestPlainMonthDayTruncatesAndRejects checks that a fractional argument truncates and that
// the out-of-range cases throw, including February 30, the bounds from @js-temporal/polyfill.
func TestPlainMonthDayTruncatesAndRejects(t *testing.T) {
	if got := NewPlainMonthDay(1.5, 1).MonthCode().ToGoString(); got != "M01" {
		t.Errorf("month 1.5 truncated to %q, want M01", got)
	}
	throwing := [][2]float64{
		{13, 1},    // month above 12
		{1, 32},    // day above the month length
		{nan(), 1}, // NaN month
		{2, 30},    // February 30 does not exist in the leap reference year
	}
	for _, c := range throwing {
		if !monthDayThrows(c[0], c[1]) {
			t.Errorf("NewPlainMonthDay%v did not throw", c)
		}
	}
	if monthDayThrows(2, 29) {
		t.Error("NewPlainMonthDay(2, 29) threw, but the leap reference year admits it")
	}
}

// TestPlainMonthDayWithAndToPlainDate covers WithFields overlaying the month and day, constraining
// or rejecting on overflow, and ToPlainDate supplying the year, with the leap-day cases pinned to
// @js-temporal/polyfill.
func TestPlainMonthDayWithAndToPlainDate(t *testing.T) {
	md := func(m, d float64) *PlainMonthDay { return NewPlainMonthDay(m, d) }
	some := func(v float64) Opt[float64] { return Some[float64](v) }
	none := None[float64]()
	withs := []struct {
		got  *PlainMonthDay
		want string
	}{
		{md(3, 15).WithFields(none, some(20), "constrain"), "03-20"},
		{md(3, 15).WithFields(some(12), none, "constrain"), "12-15"},
		{md(3, 15).WithFields(some(7), some(4), "constrain"), "07-04"},
		{md(3, 15).WithFields(some(2), some(30), "constrain"), "02-29"},
	}
	for i, c := range withs {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("with %d = %q, want %q", i, got, c.want)
		}
	}

	dates := []struct {
		got  *PlainDate
		want string
	}{
		{md(3, 15).ToPlainDate(2020), "2020-03-15"},
		{md(2, 29).ToPlainDate(2020), "2020-02-29"},
		{md(2, 29).ToPlainDate(2021), "2021-02-28"},
	}
	for i, c := range dates {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("toPlainDate %d = %q, want %q", i, got, c.want)
		}
	}

	throws := func(fn func()) (thrown bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(Thrown); ok {
					thrown = true
				}
			}
		}()
		fn()
		return false
	}
	if !throws(func() { md(3, 15).WithFields(some(4), some(31), "reject") }) {
		t.Error("03-15 with April 31 reject did not throw")
	}
}

// TestPlainYearMonthAndMonthDayFromFields covers the from-bag constructors: PlainYearMonthFromFields
// building from a year and month with constrain and reject, and PlainMonthDayFromFields building
// from a month and day with an optional year that sets the leap check, pinned to
// @js-temporal/polyfill.
func TestPlainYearMonthAndMonthDayFromFields(t *testing.T) {
	some := func(v float64) Opt[float64] { return Some[float64](v) }
	none := None[float64]()
	if got := PlainYearMonthFromFields(2020, 3, "constrain").ToString().ToGoString(); got != "2020-03" {
		t.Errorf("year-month from 2020, 3 = %q, want 2020-03", got)
	}
	if got := PlainYearMonthFromFields(2020, 13, "constrain").ToString().ToGoString(); got != "2020-12" {
		t.Errorf("year-month from 2020, 13 constrain = %q, want 2020-12", got)
	}

	mds := []struct {
		got  *PlainMonthDay
		want string
	}{
		{PlainMonthDayFromFields(3, 15, none, "constrain"), "03-15"},
		{PlainMonthDayFromFields(2, 29, none, "constrain"), "02-29"},
		{PlainMonthDayFromFields(2, 29, some(2021), "constrain"), "02-28"},
		{PlainMonthDayFromFields(2, 29, some(2020), "constrain"), "02-29"},
		{PlainMonthDayFromFields(4, 31, none, "constrain"), "04-30"},
	}
	for i, c := range mds {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("month-day from %d = %q, want %q", i, got, c.want)
		}
	}

	throws := func(fn func()) (thrown bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(Thrown); ok {
					thrown = true
				}
			}
		}()
		fn()
		return false
	}
	if !throws(func() { PlainYearMonthFromFields(2020, 13, "reject") }) {
		t.Error("year-month from month 13 reject did not throw")
	}
	if !throws(func() { PlainMonthDayFromFields(2, 29, some(2021), "reject") }) {
		t.Error("month-day from Feb 29 with common year reject did not throw")
	}
}

// TestPlainDateEra pins that the ISO calendar has no era: era and eraYear read as
// undefined optionals, the value every ISO date gives for the era-based fields.
func TestPlainDateEra(t *testing.T) {
	pd := NewPlainDate(2020, 3, 15)
	if !pd.Era().IsUndefined() {
		t.Errorf("Era() = %v, want undefined", pd.Era())
	}
	if !pd.EraYear().IsUndefined() {
		t.Errorf("EraYear() = %v, want undefined", pd.EraYear())
	}
}

// TestPlainDateWeekOfYear pins the ISO 8601 week date across the boundaries where a
// week belongs to the neighbouring year, checked against @js-temporal/polyfill.
func TestPlainDateWeekOfYear(t *testing.T) {
	cases := []struct {
		y, m, d        int
		week, weekYear float64
	}{
		{2020, 1, 1, 1, 2020},    // first week of its own year
		{2020, 6, 15, 25, 2020},  // mid-year
		{2020, 3, 15, 11, 2020},  // last day of a week
		{2020, 12, 31, 53, 2020}, // a 53-week year's last week
		{2021, 1, 1, 53, 2020},   // early January in the previous year's week 53
		{2019, 12, 30, 1, 2020},  // late December in the next year's week 1
		{2018, 12, 31, 1, 2019},  // late December, week 1 of the next year
		{2023, 1, 1, 52, 2022},   // early January in the previous year's week 52
	}
	for _, c := range cases {
		pd := NewPlainDate(float64(c.y), float64(c.m), float64(c.d))
		w := pd.WeekOfYear()
		if w.IsUndefined() || w.Get() != c.week {
			t.Errorf("%d-%02d-%02d WeekOfYear() = %v, want %v", c.y, c.m, c.d, w, c.week)
		}
		wy := pd.YearOfWeek()
		if wy.IsUndefined() || wy.Get() != c.weekYear {
			t.Errorf("%d-%02d-%02d YearOfWeek() = %v, want %v", c.y, c.m, c.d, wy, c.weekYear)
		}
	}
}

// TestPlainDateTimeCalendarFields pins that a date-time answers the calendar-dependent
// getters off its date half, so it reads the same as the PlainDate it carries.
func TestPlainDateTimeCalendarFields(t *testing.T) {
	dt := NewPlainDateTime(2020, 3, 15, 10, 30, 0, 0, 0, 0)
	if !dt.Era().IsUndefined() || !dt.EraYear().IsUndefined() {
		t.Errorf("date-time era/eraYear = %v/%v, want undefined", dt.Era(), dt.EraYear())
	}
	if w := dt.WeekOfYear(); w.IsUndefined() || w.Get() != 11 {
		t.Errorf("date-time WeekOfYear() = %v, want 11", w)
	}
	if wy := dt.YearOfWeek(); wy.IsUndefined() || wy.Get() != 2020 {
		t.Errorf("date-time YearOfWeek() = %v, want 2020", wy)
	}
}

// bigInt parses a decimal string into a big.Int for the Instant tests, failing the test
// on a malformed literal.
func bigInt(t *testing.T, s string) *big.Int {
	t.Helper()
	b, ok := new(big.Int).SetString(s, 10)
	if !ok {
		t.Fatalf("bad big.Int literal %q", s)
	}
	return b
}

// instantThrows reports whether NewInstant throws a RangeError for the count.
func instantThrows(ns *big.Int) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewInstant(ns)
	return false
}

// TestInstantEpochGetters checks epochNanoseconds and epochMilliseconds, including the
// floor toward minus infinity a negative instant takes, against @js-temporal/polyfill.
func TestInstantEpochGetters(t *testing.T) {
	cases := []struct {
		ns string
		ms float64
	}{
		{"0", 0},
		{"123456789", 123},
		{"1000000000", 1000},
		{"-1", -1},
		{"1500000000000000000", 1500000000000},
		{"8640000000000000000000", 8640000000000000},
		{"-8640000000000000000000", -8640000000000000},
	}
	for _, c := range cases {
		i := NewInstant(bigInt(t, c.ns))
		if got := i.EpochNanoseconds(); got.Cmp(bigInt(t, c.ns)) != 0 {
			t.Errorf("EpochNanoseconds(%s) = %s, want %s", c.ns, got, c.ns)
		}
		if got := i.EpochMilliseconds(); got != c.ms {
			t.Errorf("EpochMilliseconds(%s) = %v, want %v", c.ns, got, c.ms)
		}
	}
}

// TestInstantToString checks the default UTC ISO rendering, including a fractional
// second, a negative instant borrowing into the previous day, and the expanded-year
// form at each range bound, against @js-temporal/polyfill.
func TestInstantToString(t *testing.T) {
	cases := []struct {
		ns   string
		want string
	}{
		{"0", "1970-01-01T00:00:00Z"},
		{"123456789", "1970-01-01T00:00:00.123456789Z"},
		{"1000000000", "1970-01-01T00:00:01Z"},
		{"-1", "1969-12-31T23:59:59.999999999Z"},
		{"1500000000000000000", "2017-07-14T02:40:00Z"},
		{"-62135596800000000000", "0001-01-01T00:00:00Z"},
		{"8640000000000000000000", "+275760-09-13T00:00:00Z"},
		{"-8640000000000000000000", "-271821-04-20T00:00:00Z"},
	}
	for _, c := range cases {
		i := NewInstant(bigInt(t, c.ns))
		if got := i.ToString().ToGoString(); got != c.want {
			t.Errorf("Instant(%s).ToString() = %q, want %q", c.ns, got, c.want)
		}
		if got := i.ToJSON().ToGoString(); got != c.want {
			t.Errorf("Instant(%s).ToJSON() = %q, want %q", c.ns, got, c.want)
		}
	}
}

// TestInstantCompareEquals checks the ordering static and the equals method.
func TestInstantCompareEquals(t *testing.T) {
	a := NewInstant(bigInt(t, "1"))
	b := NewInstant(bigInt(t, "2"))
	c := NewInstant(bigInt(t, "1"))
	if got := InstantCompare(a, b); got != -1 {
		t.Errorf("compare(1, 2) = %v, want -1", got)
	}
	if got := InstantCompare(b, a); got != 1 {
		t.Errorf("compare(2, 1) = %v, want 1", got)
	}
	if got := InstantCompare(a, c); got != 0 {
		t.Errorf("compare(1, 1) = %v, want 0", got)
	}
	if !a.Equals(c) {
		t.Errorf("Instant(1).equals(Instant(1)) = false, want true")
	}
	if a.Equals(b) {
		t.Errorf("Instant(1).equals(Instant(2)) = true, want false")
	}
}

// TestInstantAddSubtract checks Temporal.Instant.prototype.add and subtract fold a duration's
// time part into the epoch count, negating for subtract, and reject the calendar units. Every
// value was checked against @js-temporal/polyfill.
func TestInstantAddSubtract(t *testing.T) {
	base := NewInstant(bigInt(t, "1000000000000000000")) // 2001-09-09T01:46:40Z
	dur := func(h, mi, s, ms, us, ns float64) *Duration {
		return NewDuration(0, 0, 0, 0, h, mi, s, ms, us, ns)
	}
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"add an hour", base.AddDuration(dur(1, 0, 0, 0, 0, 0)).ToString().ToGoString(), "2001-09-09T02:46:40Z"},
		{"add hms", base.AddDuration(dur(1, 30, 15, 0, 0, 0)).ToString().ToGoString(), "2001-09-09T03:16:55Z"},
		{"add subsecond", base.AddDuration(dur(0, 0, 0, 0, 0, 500)).ToString().ToGoString(), "2001-09-09T01:46:40.0000005Z"},
		{"subtract two hours", base.AddDuration(dur(-2, 0, 0, 0, 0, 0)).ToString().ToGoString(), "2001-09-08T23:46:40Z"},
		{"add a negative hour", base.AddDuration(dur(-1, 0, 0, 0, 0, 0)).ToString().ToGoString(), "2001-09-09T00:46:40Z"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// The calendar units are meaningless for an exact time, so each throws a RangeError.
	calendar := []struct {
		name string
		dur  *Duration
	}{
		{"days", NewDuration(0, 0, 0, 1, 0, 0, 0, 0, 0, 0)},
		{"weeks", NewDuration(0, 0, 1, 0, 0, 0, 0, 0, 0, 0)},
		{"months", NewDuration(0, 1, 0, 0, 0, 0, 0, 0, 0, 0)},
		{"years", NewDuration(1, 0, 0, 0, 0, 0, 0, 0, 0, 0)},
	}
	for _, tc := range calendar {
		if !bagThrows(func() { base.AddDuration(tc.dur) }) {
			t.Errorf("Instant add over a %s duration did not throw", tc.name)
		}
	}
}

// TestInstantUntilSince checks Temporal.Instant.prototype.until and since report the exact-time
// difference, balanced from largestUnit down and rounded at smallestUnit, since negating the
// result. Every value was checked against @js-temporal/polyfill.
func TestInstantUntilSince(t *testing.T) {
	a := NewInstant(bigInt(t, "0"))
	b := NewInstant(bigInt(t, "8130250500000")) // + 8130.2505 seconds = 2h15m30.2505s
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"until default second", a.Until(b, "second", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT8130.2505S"},
		{"since default second", a.Since(b, "second", "nanosecond", 1, "trunc").ToString().ToGoString(), "-PT8130.2505S"},
		{"until largest hour", a.Until(b, "hour", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT2H15M30.2505S"},
		{"until largest minute", a.Until(b, "minute", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT135M30.2505S"},
		{"until smallest second trunc", a.Until(b, "second", "second", 1, "trunc").ToString().ToGoString(), "PT8130S"},
		{"until smallest second ceil", a.Until(b, "second", "second", 1, "ceil").ToString().ToGoString(), "PT8131S"},
		{"until smallest minute inc15 floor", a.Until(b, "minute", "minute", 15, "floor").ToString().ToGoString(), "PT135M"},
		{"reversed", b.Until(a, "second", "nanosecond", 1, "trunc").ToString().ToGoString(), "-PT8130.2505S"},
		{"equal", a.Until(a, "second", "nanosecond", 1, "trunc").ToString().ToGoString(), "PT0S"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// A largestUnit smaller than the smallestUnit and an out-of-range increment each throw.
	if !bagThrows(func() { a.Until(b, "nanosecond", "second", 1, "trunc") }) {
		t.Errorf("Until with largestUnit smaller than smallestUnit did not throw")
	}
	if !bagThrows(func() { a.Until(b, "second", "minute", 60, "trunc") }) {
		t.Errorf("Until with an out-of-range increment did not throw")
	}
}

func TestInstantRound(t *testing.T) {
	b := NewInstant(bigInt(t, "8130250500000")) // 2h15m30.2505s after the epoch
	cases := []struct {
		name string
		got  string
		want string
	}{
		{"hour halfExpand", b.Round("hour", 1, "halfExpand").ToString().ToGoString(), "1970-01-01T02:00:00Z"},
		{"minute halfExpand", b.Round("minute", 1, "halfExpand").ToString().ToGoString(), "1970-01-01T02:16:00Z"},
		{"second halfExpand", b.Round("second", 1, "halfExpand").ToString().ToGoString(), "1970-01-01T02:15:30Z"},
		{"second ceil", b.Round("second", 1, "ceil").ToString().ToGoString(), "1970-01-01T02:15:31Z"},
		{"minute inc15", b.Round("minute", 15, "halfExpand").ToString().ToGoString(), "1970-01-01T02:15:00Z"},
		{"hour inc6", b.Round("hour", 6, "halfExpand").ToString().ToGoString(), "1970-01-01T00:00:00Z"},
		{"hour inc24", b.Round("hour", 24, "halfExpand").ToString().ToGoString(), "1970-01-01T00:00:00Z"},
		{"nanosecond dayspan", b.Round("nanosecond", 86400000000000, "halfExpand").ToString().ToGoString(), "1970-01-01T00:00:00Z"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, tc.got, tc.want)
		}
	}

	// An increment that does not divide the day length, one past the day length, and an invalid
	// unit each throw a RangeError.
	if !bagThrows(func() { b.Round("hour", 5, "halfExpand") }) {
		t.Errorf("Round with an increment not dividing the day did not throw")
	}
	if !bagThrows(func() { b.Round("hour", 48, "halfExpand") }) {
		t.Errorf("Round with an increment past the day length did not throw")
	}
	if !bagThrows(func() { b.Round("day", 1, "halfExpand") }) {
		t.Errorf("Round with a calendar unit did not throw")
	}
}

// TestInstantFactories checks fromEpochMilliseconds, fromEpochNanoseconds, and from over
// an Instant, plus that from returns a distinct copy.
func TestInstantFactories(t *testing.T) {
	if got := InstantFromEpochMilliseconds(1000).ToString().ToGoString(); got != "1970-01-01T00:00:01Z" {
		t.Errorf("fromEpochMilliseconds(1000) = %q, want 1970-01-01T00:00:01Z", got)
	}
	if got := InstantFromEpochNanoseconds(bigInt(t, "1000000000")).ToString().ToGoString(); got != "1970-01-01T00:00:01Z" {
		t.Errorf("fromEpochNanoseconds(1e9) = %q, want 1970-01-01T00:00:01Z", got)
	}
	src := NewInstant(bigInt(t, "42"))
	cp := InstantFrom(src)
	if cp == src {
		t.Errorf("InstantFrom returned the same pointer, want a copy")
	}
	if !cp.Equals(src) {
		t.Errorf("InstantFrom copy does not equal its source")
	}
}

// TestInstantRangeThrows checks that a count past either range bound throws a RangeError
// while the bound itself is accepted, and that a fractional millisecond throws.
func TestInstantRangeThrows(t *testing.T) {
	if instantThrows(bigInt(t, "8640000000000000000000")) {
		t.Errorf("NewInstant at the upper bound threw, want accepted")
	}
	if !instantThrows(bigInt(t, "8640000000000000000001")) {
		t.Errorf("NewInstant past the upper bound did not throw")
	}
	if !instantThrows(bigInt(t, "-8640000000000000000001")) {
		t.Errorf("NewInstant past the lower bound did not throw")
	}
	fracThrew := func() (thrown bool) {
		defer func() {
			if r := recover(); r != nil {
				if _, ok := r.(Thrown); ok {
					thrown = true
				}
			}
		}()
		InstantFromEpochMilliseconds(1.5)
		return false
	}()
	if !fracThrew {
		t.Errorf("fromEpochMilliseconds(1.5) did not throw")
	}
}

// zdtThrows reports whether NewZonedDateTime throws for the count and zone.
func zdtThrows(ns *big.Int, tz string) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewZonedDateTime(ns, FromGoString(tz))
	return false
}

// TestZonedDateTimeExactTime checks the exact-time getters, which read the instant with the
// zone dropped, against @js-temporal/polyfill.
func TestZonedDateTimeExactTime(t *testing.T) {
	z := NewZonedDateTime(bigInt(t, "1000000000"), FromGoString("UTC"))
	if got := z.EpochNanoseconds(); got.Cmp(bigInt(t, "1000000000")) != 0 {
		t.Errorf("EpochNanoseconds = %s, want 1000000000", got)
	}
	if got := z.EpochMilliseconds(); got != 1000 {
		t.Errorf("EpochMilliseconds = %v, want 1000", got)
	}
	if got := z.TimeZoneId().ToGoString(); got != "UTC" {
		t.Errorf("TimeZoneId = %q, want UTC", got)
	}
	if got := z.CalendarId().ToGoString(); got != "iso8601" {
		t.Errorf("CalendarId = %q, want iso8601", got)
	}
	if got := z.ToInstant().ToString().ToGoString(); got != "1970-01-01T00:00:01Z" {
		t.Errorf("ToInstant = %q, want 1970-01-01T00:00:01Z", got)
	}
}

// TestZonedDateTimeLocalFields checks the wall-clock getters and the offset, in UTC, in a
// fixed offset zone, and in a named zone across a daylight-saving boundary, against
// @js-temporal/polyfill.
func TestZonedDateTimeLocalFields(t *testing.T) {
	// New York on the epoch is winter, five hours behind UTC.
	ny := NewZonedDateTime(bigInt(t, "0"), FromGoString("America/New_York"))
	if got := ny.Year(); got != 1969 {
		t.Errorf("NY0 Year = %v, want 1969", got)
	}
	if got := ny.Month(); got != 12 {
		t.Errorf("NY0 Month = %v, want 12", got)
	}
	if got := ny.Day(); got != 31 {
		t.Errorf("NY0 Day = %v, want 31", got)
	}
	if got := ny.Hour(); got != 19 {
		t.Errorf("NY0 Hour = %v, want 19", got)
	}
	if got := ny.OffsetNanoseconds(); got != -18000000000000 {
		t.Errorf("NY0 OffsetNanoseconds = %v, want -18000000000000", got)
	}
	if got := ny.Offset().ToGoString(); got != "-05:00" {
		t.Errorf("NY0 Offset = %q, want -05:00", got)
	}

	// The same zone in July is summer, four hours behind: the offset follows the transition.
	summer := NewZonedDateTime(bigInt(t, "1719792000000000000"), FromGoString("America/New_York"))
	if got := summer.Year(); got != 2024 {
		t.Errorf("summer Year = %v, want 2024", got)
	}
	if got := summer.Hour(); got != 20 {
		t.Errorf("summer Hour = %v, want 20", got)
	}
	if got := summer.DayOfWeek(); got != 7 {
		t.Errorf("summer DayOfWeek = %v, want 7", got)
	}
	if got := summer.DayOfYear(); got != 182 {
		t.Errorf("summer DayOfYear = %v, want 182", got)
	}
	if w := summer.WeekOfYear(); w.IsUndefined() || w.Get() != 26 {
		t.Errorf("summer WeekOfYear = %v (undefined %v), want 26", w.Get(), w.IsUndefined())
	}
	if got := summer.InLeapYear(); !got {
		t.Errorf("summer InLeapYear = %v, want true", got)
	}
	if got := summer.MonthCode().ToGoString(); got != "M06" {
		t.Errorf("summer MonthCode = %q, want M06", got)
	}
	if got := summer.Offset().ToGoString(); got != "-04:00" {
		t.Errorf("summer Offset = %q, want -04:00", got)
	}

	// A fixed numeric offset shifts the wall clock by a constant.
	off := NewZonedDateTime(bigInt(t, "0"), FromGoString("+05:30"))
	if got := off.Hour(); got != 5 {
		t.Errorf("off Hour = %v, want 5", got)
	}
	if got := off.Minute(); got != 30 {
		t.Errorf("off Minute = %v, want 30", got)
	}
	if got := off.OffsetNanoseconds(); got != 19800000000000 {
		t.Errorf("off OffsetNanoseconds = %v, want 19800000000000", got)
	}
	if got := off.TimeZoneId().ToGoString(); got != "+05:30" {
		t.Errorf("off TimeZoneId = %q, want +05:30", got)
	}
}

// TestZonedDateTimeToString checks the round-trippable rendering, the local ISO date-time
// with the offset and the bracketed zone, against @js-temporal/polyfill.
func TestZonedDateTimeToString(t *testing.T) {
	cases := []struct {
		ns   string
		tz   string
		want string
	}{
		{"0", "UTC", "1970-01-01T00:00:00+00:00[UTC]"},
		{"1000000000", "UTC", "1970-01-01T00:00:01+00:00[UTC]"},
		{"-1", "UTC", "1969-12-31T23:59:59.999999999+00:00[UTC]"},
		{"0", "America/New_York", "1969-12-31T19:00:00-05:00[America/New_York]"},
		{"1719792000000000000", "America/New_York", "2024-06-30T20:00:00-04:00[America/New_York]"},
		{"0", "+05:30", "1970-01-01T05:30:00+05:30[+05:30]"},
	}
	for _, c := range cases {
		z := NewZonedDateTime(bigInt(t, c.ns), FromGoString(c.tz))
		if got := z.ToString().ToGoString(); got != c.want {
			t.Errorf("ZonedDateTime(%s, %s).ToString() = %q, want %q", c.ns, c.tz, got, c.want)
		}
		if got := z.ToJSON().ToGoString(); got != c.want {
			t.Errorf("ZonedDateTime(%s, %s).ToJSON() = %q, want %q", c.ns, c.tz, got, c.want)
		}
	}
}

// TestZonedDateTimeCompareEquals checks the ordering static and the equals method, which
// also weighs the zone identifier.
func TestZonedDateTimeCompareEquals(t *testing.T) {
	a := NewZonedDateTime(bigInt(t, "0"), FromGoString("UTC"))
	b := NewZonedDateTime(bigInt(t, "1000000000"), FromGoString("UTC"))
	c := NewZonedDateTime(bigInt(t, "0"), FromGoString("America/New_York"))
	if got := ZonedDateTimeCompare(a, b); got != -1 {
		t.Errorf("compare(a, b) = %v, want -1", got)
	}
	if got := ZonedDateTimeCompare(b, a); got != 1 {
		t.Errorf("compare(b, a) = %v, want 1", got)
	}
	if !a.Equals(NewZonedDateTime(bigInt(t, "0"), FromGoString("UTC"))) {
		t.Errorf("a.equals(same) = false, want true")
	}
	if a.Equals(b) {
		t.Errorf("a.equals(b) = true, want false")
	}
	if a.Equals(c) {
		t.Errorf("a.equals(c) = true, want false: same instant, different zone")
	}
}

// TestZonedDateTimeConversions checks toPlainDate, toPlainTime, and toPlainDateTime carry
// the wall-clock reading.
func TestZonedDateTimeConversions(t *testing.T) {
	z := NewZonedDateTime(bigInt(t, "1719792000000000000"), FromGoString("America/New_York"))
	if got := z.ToPlainDateTime().ToString().ToGoString(); got != "2024-06-30T20:00:00" {
		t.Errorf("ToPlainDateTime = %q, want 2024-06-30T20:00:00", got)
	}
	if got := z.ToPlainDate().ToString().ToGoString(); got != "2024-06-30" {
		t.Errorf("ToPlainDate = %q, want 2024-06-30", got)
	}
	if got := z.ToPlainTime().ToString().ToGoString(); got != "20:00:00" {
		t.Errorf("ToPlainTime = %q, want 20:00:00", got)
	}
}

// TestZonedDateTimeFromCopies checks from over a ZonedDateTime returns an independent copy.
func TestZonedDateTimeFromCopies(t *testing.T) {
	a := NewZonedDateTime(bigInt(t, "0"), FromGoString("UTC"))
	b := ZonedDateTimeFrom(a)
	if !a.Equals(b) {
		t.Errorf("from copy not equal to original")
	}
	if a == b {
		t.Errorf("from returned the same pointer, want a copy")
	}
}

// TestZonedDateTimeAddDuration checks add and subtract split the calendar part from the exact
// time. Across the New York spring-forward boundary a whole day added keeps the wall clock at
// noon and re-resolves the offset to -04:00, while an exact twenty-four hours skips the lost hour
// and reads 13:00; a calendar-unit duration moves the date and re-resolves the offset; a day
// added into the fall-back overlap keeps the wall clock and takes the earlier offset; and a Tokyo
// zone with no transition adds an exact ninety minutes. Every expected value was checked against
// @js-temporal/polyfill.
func TestZonedDateTimeAddDuration(t *testing.T) {
	spring := ZonedDateTimeFromString("2024-03-09T12:00:00-05:00[America/New_York]")
	fallback := ZonedDateTimeFromString("2024-11-02T01:30:00-04:00[America/New_York]")
	tokyo := ZonedDateTimeFromString("2024-01-01T00:00:00+09:00[Asia/Tokyo]")
	cases := []struct {
		name string
		base *ZonedDateTime
		dur  *Duration
		want string
	}{
		{"day across spring forward", spring, NewDuration(0, 0, 0, 1, 0, 0, 0, 0, 0, 0), "2024-03-10T12:00:00-04:00[America/New_York]"},
		{"exact 24 hours across spring forward", spring, NewDuration(0, 0, 0, 0, 24, 0, 0, 0, 0, 0), "2024-03-10T13:00:00-04:00[America/New_York]"},
		{"one month", spring, NewDuration(0, 1, 0, 0, 0, 0, 0, 0, 0, 0), "2024-04-09T12:00:00-04:00[America/New_York]"},
		{"mixed calendar units", spring, NewDuration(1, 2, 0, 3, 0, 0, 0, 0, 0, 0), "2025-05-12T12:00:00-04:00[America/New_York]"},
		{"subtract a day", spring, NewDuration(0, 0, 0, 1, 0, 0, 0, 0, 0, 0).Negated(), "2024-03-08T12:00:00-05:00[America/New_York]"},
		{"day into fall-back overlap", fallback, NewDuration(0, 0, 0, 1, 0, 0, 0, 0, 0, 0), "2024-11-03T01:30:00-04:00[America/New_York]"},
		{"exact time in a stable zone", tokyo, NewDuration(0, 0, 0, 0, 0, 90, 0, 0, 0, 0), "2024-01-01T01:30:00+09:00[Asia/Tokyo]"},
	}
	for _, c := range cases {
		got := c.base.AddDuration(c.dur, "constrain").ToString().ToGoString()
		if got != c.want {
			t.Errorf("%s: AddDuration = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestZonedDateTimeDifference(t *testing.T) {
	a := ZonedDateTimeFromString("2024-03-08T12:00:00[America/New_York]")
	b := ZonedDateTimeFromString("2024-03-11T12:00:00[America/New_York]")
	early := ZonedDateTimeFromString("2024-03-11T10:00:00[America/New_York]")
	t1 := ZonedDateTimeFromString("2024-01-01T00:00:00[Asia/Tokyo]")
	t2 := ZonedDateTimeFromString("2024-04-06T06:30:00[Asia/Tokyo]")
	cases := []struct {
		name string
		got  *Duration
		want string
	}{
		{"hours across spring forward", a.Until(b, "hour"), "PT71H"},
		{"days across spring forward", a.Until(b, "day"), "P3D"},
		{"weeks fold to days", a.Until(b, "week"), "P3D"},
		{"tokyo months", t1.Until(t2, "month"), "P3M5DT6H30M"},
		{"tokyo years", t1.Until(t2, "year"), "P3M5DT6H30M"},
		{"backward days", b.Until(a, "day"), "-P3D"},
		{"borrow into days", a.Until(early, "day"), "P2DT22H"},
		{"borrow into hours", a.Until(early, "hour"), "PT69H"},
		{"since negates", a.Since(b, "day"), "-P3D"},
	}
	for _, c := range cases {
		if got := c.got.ToString().ToGoString(); got != c.want {
			t.Errorf("%s: difference = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestZonedDateTimeRound(t *testing.T) {
	cases := []struct {
		name      string
		base      string
		unit      string
		increment float64
		mode      string
		want      string
	}{
		{"hour down", "2024-06-15T12:29:00[America/New_York]", "hour", 1, "halfExpand", "2024-06-15T12:00:00-04:00[America/New_York]"},
		{"hour up", "2024-06-15T12:30:00[America/New_York]", "hour", 1, "halfExpand", "2024-06-15T13:00:00-04:00[America/New_York]"},
		{"fifteen minutes", "2024-06-15T12:31:40[America/New_York]", "minute", 15, "halfExpand", "2024-06-15T12:30:00-04:00[America/New_York]"},
		{"day down", "2024-06-15T11:00:00[America/New_York]", "day", 1, "halfExpand", "2024-06-15T00:00:00-04:00[America/New_York]"},
		{"day up", "2024-06-15T13:00:00[America/New_York]", "day", 1, "halfExpand", "2024-06-16T00:00:00-04:00[America/New_York]"},
		{"day on the twenty-three-hour spring day", "2024-03-10T12:00:00[America/New_York]", "day", 1, "halfExpand", "2024-03-10T00:00:00-05:00[America/New_York]"},
		{"day on the twenty-five-hour fall day", "2024-11-03T12:00:00[America/New_York]", "day", 1, "halfExpand", "2024-11-04T00:00:00-05:00[America/New_York]"},
		{"day ceil", "2024-06-15T00:00:01[America/New_York]", "day", 1, "ceil", "2024-06-16T00:00:00-04:00[America/New_York]"},
		{"hour into overlap keeps later offset", "2024-11-03T01:40:00-05:00[America/New_York]", "hour", 1, "floor", "2024-11-03T01:00:00-05:00[America/New_York]"},
		{"hour up out of overlap", "2024-11-03T01:40:00-05:00[America/New_York]", "hour", 1, "halfExpand", "2024-11-03T02:00:00-05:00[America/New_York]"},
	}
	for _, c := range cases {
		got := ZonedDateTimeFromString(c.base).Round(c.unit, c.increment, c.mode).ToString().ToGoString()
		if got != c.want {
			t.Errorf("%s: Round = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestZonedDateTimeWithFamily(t *testing.T) {
	some := Some[float64]
	none := None[float64]()
	base := ZonedDateTimeFromString("2024-06-15T12:30:45-04:00[America/New_York]")
	// with: overlay date and time fields, offset preferred.
	if got := base.WithFields(none, none, none, none, some(0), some(0), none, none, none, "constrain").ToString().ToGoString(); got != "2024-06-15T12:00:00-04:00[America/New_York]" {
		t.Errorf("with minute/second = %q", got)
	}
	gap := ZonedDateTimeFromString("2024-03-10T12:00:00-04:00[America/New_York]")
	if got := gap.WithFields(none, none, none, some(2), some(30), none, none, none, none, "constrain").ToString().ToGoString(); got != "2024-03-10T03:30:00-04:00[America/New_York]" {
		t.Errorf("with into the gap = %q", got)
	}
	overlap := ZonedDateTimeFromString("2024-11-03T01:30:00-05:00[America/New_York]")
	if got := overlap.WithFields(none, none, none, none, some(15), none, none, none, none, "constrain").ToString().ToGoString(); got != "2024-11-03T01:15:00-05:00[America/New_York]" {
		t.Errorf("with inside the overlap = %q", got)
	}
	jan31 := ZonedDateTimeFromString("2024-01-31T12:00:00-05:00[America/New_York]")
	if got := jan31.WithFields(none, some(2), none, none, none, none, none, none, none, "constrain").ToString().ToGoString(); got != "2024-02-29T12:00:00-05:00[America/New_York]" {
		t.Errorf("with month constrain = %q", got)
	}
	if !zdtCall(func() { jan31.WithFields(none, some(2), none, none, none, none, none, none, none, "reject") }) {
		t.Errorf("with month reject did not throw")
	}
	// withPlainTime: replace the time, compatible disambiguation.
	if got := base.WithPlainTime(PlainTimeFromString("08:00")).ToString().ToGoString(); got != "2024-06-15T08:00:00-04:00[America/New_York]" {
		t.Errorf("withPlainTime = %q", got)
	}
	if got := base.WithPlainTime(nil).ToString().ToGoString(); got != "2024-06-15T00:00:00-04:00[America/New_York]" {
		t.Errorf("withPlainTime midnight = %q", got)
	}
	// withTimeZone: keep the instant, re-home the zone.
	if got := base.WithTimeZone("Asia/Tokyo").ToString().ToGoString(); got != "2024-06-16T01:30:45+09:00[Asia/Tokyo]" {
		t.Errorf("withTimeZone = %q", got)
	}
	// withCalendar: identity for the ISO calendar.
	if got := base.WithCalendar("iso8601").ToString().ToGoString(); got != "2024-06-15T12:30:45-04:00[America/New_York]" {
		t.Errorf("withCalendar = %q", got)
	}
	// withCalendar: a non-ISO calendar re-reads the wall-clock year and era.
	roc := base.WithCalendar("roc")
	if got := roc.CalendarId().ToGoString(); got != "roc" {
		t.Errorf("withCalendar id = %q, want roc", got)
	}
	if got := roc.Year(); got != 113 {
		t.Errorf("withCalendar roc year = %v, want 113", got)
	}
}

// TestZonedDateTimeDayQueries pins startOfDay to the first instant of the local day and hoursInDay
// to the day length, twenty-four on an ordinary day, twenty-three across spring forward, and
// twenty-five across fall back.
func TestZonedDateTimeDayQueries(t *testing.T) {
	normal := ZonedDateTimeFromString("2024-06-15T12:30:45-04:00[America/New_York]")
	if got := normal.StartOfDay().ToString().ToGoString(); got != "2024-06-15T00:00:00-04:00[America/New_York]" {
		t.Errorf("startOfDay normal = %q", got)
	}
	if got := normal.HoursInDay(); got != 24 {
		t.Errorf("hoursInDay normal = %v", got)
	}
	spring := ZonedDateTimeFromString("2024-03-10T15:00:00-04:00[America/New_York]")
	if got := spring.StartOfDay().ToString().ToGoString(); got != "2024-03-10T00:00:00-05:00[America/New_York]" {
		t.Errorf("startOfDay spring = %q", got)
	}
	if got := spring.HoursInDay(); got != 23 {
		t.Errorf("hoursInDay spring = %v", got)
	}
	fall := ZonedDateTimeFromString("2024-11-03T15:00:00-05:00[America/New_York]")
	if got := fall.StartOfDay().ToString().ToGoString(); got != "2024-11-03T00:00:00-04:00[America/New_York]" {
		t.Errorf("startOfDay fall = %q", got)
	}
	if got := fall.HoursInDay(); got != 25 {
		t.Errorf("hoursInDay fall = %v", got)
	}
}

// TestZonedDateTimeFromFields pins Temporal.ZonedDateTime.from over a property bag: an ordinary
// reading, the two disambiguation branches across a spring-forward gap and a fall-back overlap, and
// every offset-option branch weighing a supplied offset against the zone. Each value was checked
// against @js-temporal/polyfill.
func TestZonedDateTimeFromFields(t *testing.T) {
	some := Some[float64]
	none := None[float64]()
	noStr := None[string]()
	someStr := Some[string]
	from := func(y, mo, d float64, h, mi Opt[float64], off Opt[string], overflow, disamb, offOpt string) string {
		return ZonedDateTimeFromFields(y, mo, d, h, mi, none, none, none, none, "America/New_York", off, overflow, disamb, offOpt).ToString().ToGoString()
	}
	// Ordinary reading.
	if got := from(2024, 6, 15, some(12), some(30), noStr, "constrain", "compatible", "reject"); got != "2024-06-15T12:30:00-04:00[America/New_York]" {
		t.Errorf("basic = %q", got)
	}
	// Spring-forward gap: compatible shifts forward, earlier shifts back.
	if got := from(2024, 3, 10, some(2), some(30), noStr, "constrain", "compatible", "reject"); got != "2024-03-10T03:30:00-04:00[America/New_York]" {
		t.Errorf("gap compatible = %q", got)
	}
	if got := from(2024, 3, 10, some(2), some(30), noStr, "constrain", "earlier", "reject"); got != "2024-03-10T01:30:00-05:00[America/New_York]" {
		t.Errorf("gap earlier = %q", got)
	}
	// Fall-back overlap: compatible takes the earlier branch, later the second.
	if got := from(2024, 11, 3, some(1), some(30), noStr, "constrain", "compatible", "reject"); got != "2024-11-03T01:30:00-04:00[America/New_York]" {
		t.Errorf("overlap compatible = %q", got)
	}
	if got := from(2024, 11, 3, some(1), some(30), noStr, "constrain", "later", "reject"); got != "2024-11-03T01:30:00-05:00[America/New_York]" {
		t.Errorf("overlap later = %q", got)
	}
	// Offset option reject: the supplied offset selects the matching overlap branch.
	if got := from(2024, 11, 3, some(1), some(30), someStr("-05:00"), "constrain", "compatible", "reject"); got != "2024-11-03T01:30:00-05:00[America/New_York]" {
		t.Errorf("overlap offset -05:00 reject = %q", got)
	}
	if got := from(2024, 11, 3, some(1), some(30), someStr("-04:00"), "constrain", "compatible", "reject"); got != "2024-11-03T01:30:00-04:00[America/New_York]" {
		t.Errorf("overlap offset -04:00 reject = %q", got)
	}
	// use takes the offset at face value; ignore drops it; prefer keeps a match and otherwise falls back.
	if got := from(2024, 11, 3, some(1), some(30), someStr("-05:00"), "constrain", "compatible", "use"); got != "2024-11-03T01:30:00-05:00[America/New_York]" {
		t.Errorf("overlap offset use = %q", got)
	}
	if got := from(2024, 11, 3, some(1), some(30), someStr("+05:00"), "constrain", "compatible", "ignore"); got != "2024-11-03T01:30:00-04:00[America/New_York]" {
		t.Errorf("overlap offset ignore = %q", got)
	}
	if got := from(2024, 11, 3, some(1), some(30), someStr("-05:00"), "constrain", "compatible", "prefer"); got != "2024-11-03T01:30:00-05:00[America/New_York]" {
		t.Errorf("overlap offset prefer match = %q", got)
	}
	if got := from(2024, 11, 3, some(1), some(30), someStr("+05:00"), "constrain", "compatible", "prefer"); got != "2024-11-03T01:30:00-04:00[America/New_York]" {
		t.Errorf("overlap offset prefer fallback = %q", got)
	}
	// overflow constrain clamps an out-of-range month.
	if got := from(2024, 13, 15, some(12), none, noStr, "constrain", "compatible", "reject"); got != "2024-12-15T12:00:00-05:00[America/New_York]" {
		t.Errorf("overflow constrain = %q", got)
	}
	// disambiguation reject throws inside a gap; a non-matching offset under reject throws.
	if !zdtCall(func() {
		ZonedDateTimeFromFields(2024, 3, 10, some(2), some(30), none, none, none, none, "America/New_York", noStr, "constrain", "reject", "reject")
	}) {
		t.Errorf("gap reject did not throw")
	}
	if !zdtCall(func() {
		ZonedDateTimeFromFields(2024, 11, 3, some(1), some(30), none, none, none, none, "America/New_York", someStr("+05:00"), "constrain", "compatible", "reject")
	}) {
		t.Errorf("offset reject did not throw")
	}
}

// zdtCall runs f and reports whether it threw a Temporal error.
func zdtCall(f func()) (threw bool) {
	defer func() {
		if recover() != nil {
			threw = true
		}
	}()
	f()
	return false
}

// TestZonedDateTimeRejects checks the range guard and the unknown-zone guard both throw a
// RangeError.
func TestZonedDateTimeRejects(t *testing.T) {
	if !zdtThrows(bigInt(t, "8640000000000000000001"), "UTC") {
		t.Errorf("out-of-range count did not throw")
	}
	if !zdtThrows(bigInt(t, "0"), "Mars/Olympus_Mons") {
		t.Errorf("unknown zone did not throw")
	}
	if !zdtThrows(bigInt(t, "0"), "+99:00") {
		t.Errorf("out-of-range offset did not throw")
	}
}

// TestNowFixedClock pins the clock with BENTO_NOW_NS and the default zone with TZ, the way the
// differential harness does, and checks every Temporal.Now function reads that fixed instant.
// 1700000000123456789 ns is 2023-11-14T22:13:20.123456789Z.
func TestNowFixedClock(t *testing.T) {
	t.Setenv("BENTO_NOW_NS", "1700000000123456789")
	t.Setenv("TZ", "UTC")
	if got := NowInstant().ToString().ToGoString(); got != "2023-11-14T22:13:20.123456789Z" {
		t.Errorf("NowInstant = %q", got)
	}
	if got := NowTimeZoneId().ToGoString(); got != "UTC" {
		t.Errorf("NowTimeZoneId = %q, want UTC", got)
	}
	if got := NowZonedDateTimeISO().ToString().ToGoString(); got != "2023-11-14T22:13:20.123456789+00:00[UTC]" {
		t.Errorf("NowZonedDateTimeISO = %q", got)
	}
	if got := NowPlainDateTimeISO().ToString().ToGoString(); got != "2023-11-14T22:13:20.123456789" {
		t.Errorf("NowPlainDateTimeISO = %q", got)
	}
	if got := NowPlainDateISO().ToString().ToGoString(); got != "2023-11-14" {
		t.Errorf("NowPlainDateISO = %q", got)
	}
	if got := NowPlainTimeISO().ToString().ToGoString(); got != "22:13:20.123456789" {
		t.Errorf("NowPlainTimeISO = %q", got)
	}
}

// TestNowInZone checks the ISO functions that take an explicit zone read the fixed instant in
// that zone: on 2023-11-14 New York is five hours behind UTC, so the wall clock reads 17:13.
func TestNowInZone(t *testing.T) {
	t.Setenv("BENTO_NOW_NS", "1700000000123456789")
	t.Setenv("TZ", "UTC")
	ny := FromGoString("America/New_York")
	if got := NowZonedDateTimeISOIn(ny).ToString().ToGoString(); got != "2023-11-14T17:13:20.123456789-05:00[America/New_York]" {
		t.Errorf("NowZonedDateTimeISOIn(NY) = %q", got)
	}
	if got := NowPlainDateTimeISOIn(ny).ToString().ToGoString(); got != "2023-11-14T17:13:20.123456789" {
		t.Errorf("NowPlainDateTimeISOIn(NY) = %q", got)
	}
	if got := NowPlainDateISOIn(ny).ToString().ToGoString(); got != "2023-11-14" {
		t.Errorf("NowPlainDateISOIn(NY) = %q", got)
	}
	if got := NowPlainTimeISOIn(ny).ToString().ToGoString(); got != "17:13:20.123456789" {
		t.Errorf("NowPlainTimeISOIn(NY) = %q", got)
	}
}

// TestNowDefaultZone checks that with TZ unset the default zone is UTC, so timeZoneId is a
// deterministic identifier rather than the host-specific local zone.
func TestNowDefaultZone(t *testing.T) {
	t.Setenv("BENTO_NOW_NS", "0")
	t.Setenv("TZ", "")
	if got := NowTimeZoneId().ToGoString(); got != "UTC" {
		t.Errorf("NowTimeZoneId with TZ unset = %q, want UTC", got)
	}
	if got := NowInstant().ToString().ToGoString(); got != "1970-01-01T00:00:00Z" {
		t.Errorf("NowInstant at epoch = %q", got)
	}
}

// calendarThrows reports whether NewPlainDateCal throws a RangeError for the calendar id.
func calendarThrows(id string) (thrown bool) {
	defer func() {
		if r := recover(); r != nil {
			if _, ok := r.(Thrown); ok {
				thrown = true
			}
		}
	}()
	NewPlainDateCal(2024, 6, 30, id)
	return false
}

// TestGregoryCalendar checks the gregory calendar against @js-temporal/polyfill: it shares
// the ISO year, month, and day but adds an era, and it annotates its toString with the
// calendar id. The era splits at ISO year 1, so year 2024 is the "gregory" era, year 0 is
// "gregory-inverse" eraYear 1, and year -5 is "gregory-inverse" eraYear 6.
func TestGregoryCalendar(t *testing.T) {
	g := NewPlainDateCal(2024, 6, 30, "gregory")
	if got := g.CalendarId().ToGoString(); got != "gregory" {
		t.Errorf("CalendarId = %q, want gregory", got)
	}
	if g.Year() != 2024 || g.Month() != 6 || g.Day() != 30 {
		t.Errorf("fields = %v-%v-%v, want 2024-6-30", g.Year(), g.Month(), g.Day())
	}
	if g.Era().IsUndefined() || g.Era().Get().ToGoString() != "gregory" {
		t.Errorf("Era = %v, want gregory", g.Era())
	}
	if g.EraYear().IsUndefined() || g.EraYear().Get() != 2024 {
		t.Errorf("EraYear = %v, want 2024", g.EraYear())
	}
	if got := g.ToString().ToGoString(); got != "2024-06-30[u-ca=gregory]" {
		t.Errorf("ToString = %q", got)
	}
	if got := g.ToJSON().ToGoString(); got != "2024-06-30[u-ca=gregory]" {
		t.Errorf("ToJSON = %q", got)
	}

	// The era boundary: ISO year 0 and year -5 fall in the inverse era.
	zero := NewPlainDateCal(0, 3, 15, "gregory")
	if zero.Era().Get().ToGoString() != "gregory-inverse" {
		t.Errorf("year 0 era = %v, want gregory-inverse", zero.Era())
	}
	if zero.EraYear().Get() != 1 {
		t.Errorf("year 0 eraYear = %v, want 1", zero.EraYear())
	}
	neg := NewPlainDateCal(-5, 12, 31, "gregory")
	if neg.EraYear().Get() != 6 {
		t.Errorf("year -5 eraYear = %v, want 6", neg.EraYear())
	}
}

// TestGregoryEqualsAndCompare checks that equals weighs the calendar while compare ignores
// it: the same ISO day under iso8601 and under gregory is not equal but orders the same.
func TestGregoryEqualsAndCompare(t *testing.T) {
	iso := NewPlainDate(2024, 6, 30)
	g := NewPlainDateCal(2024, 6, 30, "gregory")
	if iso.Equals(g) {
		t.Error("iso8601 date equals gregory date, want not equal")
	}
	if c := PlainDateCompare(iso, g); c != 0 {
		t.Errorf("compare iso vs gregory = %v, want 0", c)
	}
	if !g.Equals(NewPlainDateCal(2024, 6, 30, "gregory")) {
		t.Error("two gregory dates on the same day are not equal")
	}
}

// TestPlainDateWithCalendar checks withCalendar reinterprets the ISO date under another
// calendar, and that an unhosted or invalid id throws a RangeError.
func TestPlainDateWithCalendar(t *testing.T) {
	iso := NewPlainDate(2024, 6, 30)
	g := PlainDateWithCalendar(iso, "gregory")
	if got := g.CalendarId().ToGoString(); got != "gregory" {
		t.Errorf("withCalendar id = %q, want gregory", got)
	}
	if g.Era().Get().ToGoString() != "gregory" {
		t.Errorf("withCalendar era = %v, want gregory", g.Era())
	}
	if !calendarThrows("nonsense") {
		t.Error("invalid calendar id did not throw")
	}
	if !calendarThrows("gregorian") {
		t.Error("gregorian (not a hosted alias) did not throw")
	}
	if calendarThrows("GREGORY") {
		t.Error("case-insensitive GREGORY threw, want accepted")
	}
}

// TestGregoryPlainDateTime checks the gregory calendar on PlainDateTime: it delegates era
// and eraYear to its date half and trails the annotation after the time.
func TestGregoryPlainDateTime(t *testing.T) {
	dt := NewPlainDateTimeCal(2024, 6, 30, 12, 34, 56, 0, 0, 0, "gregory")
	if got := dt.CalendarId().ToGoString(); got != "gregory" {
		t.Errorf("CalendarId = %q, want gregory", got)
	}
	if dt.Era().IsUndefined() || dt.Era().Get().ToGoString() != "gregory" {
		t.Errorf("Era = %v, want gregory", dt.Era())
	}
	if dt.EraYear().Get() != 2024 {
		t.Errorf("EraYear = %v, want 2024", dt.EraYear())
	}
	if got := dt.ToString().ToGoString(); got != "2024-06-30T12:34:56[u-ca=gregory]" {
		t.Errorf("ToString = %q", got)
	}
	inv := NewPlainDateTimeCal(0, 3, 15, 1, 2, 3, 0, 0, 0, "gregory")
	if inv.Era().Get().ToGoString() != "gregory-inverse" {
		t.Errorf("year 0 era = %v, want gregory-inverse", inv.Era())
	}
	wc := PlainDateTimeWithCalendar(NewPlainDateTime(2024, 6, 30, 12, 0, 0, 0, 0, 0), "gregory")
	if got := wc.ToString().ToGoString(); got != "2024-06-30T12:00:00[u-ca=gregory]" {
		t.Errorf("withCalendar ToString = %q", got)
	}
}

// TestRocCalendar checks the roc (Minguo) calendar against @js-temporal/polyfill. It counts
// from 1912, so its year is the ISO year minus 1911 while the month and day stay ISO, and its
// toString prints the unchanged ISO year with the calendar annotation. The era splits at ISO
// year 1912: 2024 is roc year 113 in the "roc" era, 1912 is roc year 1, 1911 is roc year 0 in
// the "roc-inverse" era eraYear 1, and ISO year -5 is "roc-inverse" eraYear 1917.
func TestRocCalendar(t *testing.T) {
	r := NewPlainDateCal(2024, 6, 30, "roc")
	if got := r.CalendarId().ToGoString(); got != "roc" {
		t.Errorf("CalendarId = %q, want roc", got)
	}
	if r.Year() != 113 || r.Month() != 6 || r.Day() != 30 {
		t.Errorf("fields = %v-%v-%v, want 113-6-30", r.Year(), r.Month(), r.Day())
	}
	if r.Era().IsUndefined() || r.Era().Get().ToGoString() != "roc" {
		t.Errorf("Era = %v, want roc", r.Era())
	}
	if r.EraYear().IsUndefined() || r.EraYear().Get() != 113 {
		t.Errorf("EraYear = %v, want 113", r.EraYear())
	}
	if got := r.ToString().ToGoString(); got != "2024-06-30[u-ca=roc]" {
		t.Errorf("ToString = %q, want 2024-06-30[u-ca=roc]", got)
	}

	one := NewPlainDateCal(1912, 1, 1, "roc")
	if one.Year() != 1 || one.Era().Get().ToGoString() != "roc" || one.EraYear().Get() != 1 {
		t.Errorf("1912 = year %v era %v eraYear %v, want 1 roc 1", one.Year(), one.Era(), one.EraYear())
	}
	zero := NewPlainDateCal(1911, 12, 31, "roc")
	if zero.Year() != 0 || zero.Era().Get().ToGoString() != "roc-inverse" || zero.EraYear().Get() != 1 {
		t.Errorf("1911 = year %v era %v eraYear %v, want 0 roc-inverse 1", zero.Year(), zero.Era(), zero.EraYear())
	}
	neg := NewPlainDateCal(-5, 1, 1, "roc")
	if neg.Year() != -1916 || neg.EraYear().Get() != 1917 {
		t.Errorf("-5 = year %v eraYear %v, want -1916 1917", neg.Year(), neg.EraYear())
	}
}

// TestRocEqualsAndCompare mirrors the gregory check: equals weighs the calendar, compare
// ignores it, so an iso8601 day and a roc day on the same ISO date are unequal but order the
// same, and roc and gregory on the same ISO date are unequal.
func TestRocEqualsAndCompare(t *testing.T) {
	iso := NewPlainDate(2024, 6, 30)
	r := NewPlainDateCal(2024, 6, 30, "roc")
	if iso.Equals(r) {
		t.Error("iso8601 date equals roc date, want not equal")
	}
	if c := PlainDateCompare(iso, r); c != 0 {
		t.Errorf("compare iso vs roc = %v, want 0", c)
	}
	if r.Equals(NewPlainDateCal(2024, 6, 30, "gregory")) {
		t.Error("roc date equals gregory date, want not equal")
	}
}

// TestRocPlainDateTime checks roc on PlainDateTime: it delegates the year offset and era to
// its date half and trails the annotation after the time, over both the ten-argument
// constructor and withCalendar.
func TestRocPlainDateTime(t *testing.T) {
	dt := NewPlainDateTimeCal(2024, 6, 30, 12, 34, 56, 0, 0, 0, "roc")
	if dt.Year() != 113 || dt.EraYear().Get() != 113 {
		t.Errorf("year %v eraYear %v, want 113 113", dt.Year(), dt.EraYear())
	}
	if got := dt.ToString().ToGoString(); got != "2024-06-30T12:34:56[u-ca=roc]" {
		t.Errorf("ToString = %q, want 2024-06-30T12:34:56[u-ca=roc]", got)
	}
	wc := PlainDateTimeWithCalendar(NewPlainDateTime(2024, 6, 30, 12, 0, 0, 0, 0, 0), "roc")
	if got := wc.CalendarId().ToGoString(); got != "roc" {
		t.Errorf("withCalendar id = %q, want roc", got)
	}
}

// TestJapaneseCalendar checks the japanese calendar against @js-temporal/polyfill. Its year,
// month, and day match ISO, but its era is a nengo resolved from the whole date. The modern
// eras each begin on a fixed day, so the era turns at that day: 1989-01-07 is showa 64 and
// 1989-01-08 heisei 1. Before Meiji begins on 1868-09-08 the era falls back to "japanese"
// mirroring the ISO year, and below year 1 to "japanese-inverse".
func TestJapaneseCalendar(t *testing.T) {
	j := NewPlainDateCal(2024, 6, 30, "japanese")
	if got := j.CalendarId().ToGoString(); got != "japanese" {
		t.Errorf("CalendarId = %q, want japanese", got)
	}
	if j.Year() != 2024 || j.Month() != 6 || j.Day() != 30 {
		t.Errorf("fields = %v-%v-%v, want 2024-6-30", j.Year(), j.Month(), j.Day())
	}
	if j.Era().Get().ToGoString() != "reiwa" || j.EraYear().Get() != 6 {
		t.Errorf("2024 = %v %v, want reiwa 6", j.Era(), j.EraYear())
	}
	if got := j.ToString().ToGoString(); got != "2024-06-30[u-ca=japanese]" {
		t.Errorf("ToString = %q", got)
	}

	// Each nengo boundary: the start day and the day before.
	cases := []struct {
		y, m, d int
		era     string
		eraYear float64
	}{
		{2019, 5, 1, "reiwa", 1},
		{2019, 4, 30, "heisei", 31},
		{1989, 1, 8, "heisei", 1},
		{1989, 1, 7, "showa", 64},
		{1926, 12, 25, "showa", 1},
		{1926, 12, 24, "taisho", 15},
		{1912, 7, 30, "taisho", 1},
		{1912, 7, 29, "meiji", 45},
		{1868, 9, 8, "meiji", 1},
		{1868, 9, 7, "japanese", 1868},
		{1000, 1, 1, "japanese", 1000},
		{1, 1, 1, "japanese", 1},
		{0, 1, 1, "japanese-inverse", 1},
		{-5, 6, 15, "japanese-inverse", 6},
	}
	for _, c := range cases {
		d := NewPlainDateCal(float64(c.y), float64(c.m), float64(c.d), "japanese")
		if d.Era().Get().ToGoString() != c.era || d.EraYear().Get() != c.eraYear {
			t.Errorf("%d-%d-%d = %v %v, want %s %v", c.y, c.m, c.d, d.Era(), d.EraYear(), c.era, c.eraYear)
		}
	}
}

// TestJapanesePlainDateTime checks japanese on PlainDateTime: it delegates the nengo era to
// its date half, so a date-time on a boundary reads the same era its date would, and it
// trails the annotation after the time.
func TestJapanesePlainDateTime(t *testing.T) {
	dt := NewPlainDateTimeCal(1989, 1, 8, 0, 0, 0, 0, 0, 0, "japanese")
	if dt.Era().Get().ToGoString() != "heisei" || dt.EraYear().Get() != 1 {
		t.Errorf("1989-01-08 = %v %v, want heisei 1", dt.Era(), dt.EraYear())
	}
	if got := dt.ToString().ToGoString(); got != "1989-01-08T00:00:00[u-ca=japanese]" {
		t.Errorf("ToString = %q", got)
	}
	wc := PlainDateTimeWithCalendar(NewPlainDateTime(2024, 6, 30, 12, 0, 0, 0, 0, 0), "japanese")
	if wc.Era().Get().ToGoString() != "reiwa" {
		t.Errorf("withCalendar era = %v, want reiwa", wc.Era())
	}
}
