package value

import "testing"

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
// out-of-range month or day, a non-finite component, a NaN month (truncates to zero
// then fails the month range), and the two representable-range boundaries.
func TestPlainDateRejects(t *testing.T) {
	throwing := [][3]float64{
		{2020, 0, 1},
		{2020, 13, 1},
		{2020, 1, 0},
		{2020, 2, 30},
		{2020, nan(), 1},
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
