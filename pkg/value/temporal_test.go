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
