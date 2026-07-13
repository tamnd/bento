package value

import "testing"

// catchThrow runs fn and reports the error name it threw, or "" if it returned normally.
func catchThrow(fn func()) (name string) {
	defer func() {
		if r := recover(); r != nil {
			if t, ok := r.(Thrown); ok {
				name = t.ErrorName()
				return
			}
			panic(r)
		}
	}()
	fn()
	return ""
}

// TestPlainDateFromString pins the ISO 8601 string parser feeding Temporal.PlainDate.from:
// the accepted forms and their date and calendar, checked against @js-temporal/polyfill.
func TestPlainDateFromString(t *testing.T) {
	cases := []struct {
		in      string
		wantStr string
		wantCal string
	}{
		{"2024-06-30", "2024-06-30", "iso8601"},
		{"20240630", "2024-06-30", "iso8601"},
		{"2024-06-30T12:34:56", "2024-06-30", "iso8601"},
		{"2024-06-30T12:34:56+05:30", "2024-06-30", "iso8601"},
		{"2024-06-30T12:34:56-05:30[America/New_York]", "2024-06-30", "iso8601"},
		{"2024-06-30[u-ca=gregory]", "2024-06-30[u-ca=gregory]", "gregory"},
		{"2024-06-30[u-ca=japanese]", "2024-06-30[u-ca=japanese]", "japanese"},
		{"2024-06-30T00:00:00[u-ca=roc]", "2024-06-30[u-ca=roc]", "roc"},
		{"+002024-06-30", "2024-06-30", "iso8601"},
		{"-000005-06-30", "-000005-06-30", "iso8601"},
		{"2024-06-30T12:34:56.123456789", "2024-06-30", "iso8601"},
		{"2024-06-30T12", "2024-06-30", "iso8601"},
		{"2024-06-30 12:34:56", "2024-06-30", "iso8601"},
		{"2024-06-30[America/New_York]", "2024-06-30", "iso8601"},
		{"2024-06-30[u-ca=iso8601]", "2024-06-30", "iso8601"},
		{"2024-06-30[foo=bar]", "2024-06-30", "iso8601"},
		{"2024-06-30[!u-ca=gregory]", "2024-06-30[u-ca=gregory]", "gregory"},
	}
	for _, c := range cases {
		pd := PlainDateFromString(c.in)
		if got := pd.ToString().ToGoString(); got != c.wantStr {
			t.Errorf("PlainDateFromString(%q) toString = %q, want %q", c.in, got, c.wantStr)
		}
		if got := pd.CalendarId().ToGoString(); got != c.wantCal {
			t.Errorf("PlainDateFromString(%q) calendarId = %q, want %q", c.in, got, c.wantCal)
		}
	}
}

// TestPlainDateFromStringThrows pins the strings the parser rejects: a bad grammar, an
// out-of-range field, a Z designator on a Plain type, or an unhosted calendar each throw a
// RangeError, checked against @js-temporal/polyfill.
func TestPlainDateFromStringThrows(t *testing.T) {
	bad := []string{
		"2024-06-30T12:34:56Z",
		"2024-06",
		"2024-W01-1",
		"2024-366",
		"2024-002",
		"--06-30",
		"06-30",
		"2024/06/30",
		"2024-13-01",
		"2024-06-31",
		"2024-02-30",
		"2024-00-01",
		"2024-06-00",
		"2024-6-30",
		"24-06-30",
		"T12:34:56",
		"2024-06-30T25:00:00",
		"2024-06-30Z",
		"2024-06-30+05:00",
		"  2024-06-30  ",
		"2024-06-30T",
		"2024-06-30T12:34:56.1234567890",
		"2024-06-30[u-ca=notacalendar]",
	}
	for _, s := range bad {
		name := catchThrow(func() { PlainDateFromString(s) })
		if name == "" {
			t.Errorf("PlainDateFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("PlainDateFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}

// TestPlainDateFromStringLeapSecond pins that a :60 leap second in the dropped time part is
// accepted and the date survives, matching @js-temporal/polyfill; the earlier parser wrongly
// rejected it.
func TestPlainDateFromStringLeapSecond(t *testing.T) {
	for _, s := range []string{"2024-06-30T23:59:60", "2024-06-30T23:59:60.5", "2024-06-30T12:00:60"} {
		pd := PlainDateFromString(s)
		if got := pd.ToString().ToGoString(); got != "2024-06-30" {
			t.Errorf("PlainDateFromString(%q) toString = %q, want 2024-06-30", s, got)
		}
	}
}

// TestPlainTimeFromString pins the string parser feeding Temporal.PlainTime.from: the extended
// and basic forms, a time designator, the date-time forms whose time is kept, the leap second
// constrained to :59, and a calendar annotation ignored whatever it names, all against
// @js-temporal/polyfill.
func TestPlainTimeFromString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"12:30:00", "12:30:00"},
		{"12:30", "12:30:00"},
		{"12", "12:00:00"},
		{"123045", "12:30:45"},
		{"T12:30:00", "12:30:00"},
		{"T12", "12:00:00"},
		{"t12:30", "12:30:00"},
		{"12:30:00.123456789", "12:30:00.123456789"},
		{"12:30:00,5", "12:30:00.5"},
		{"2024-06-30T12:30:00", "12:30:00"},
		{"2024-06-30T12:30:00.5", "12:30:00.5"},
		{"2024-06-30 12:30", "12:30:00"},
		{"12:30:00+05:30", "12:30:00"},
		{"2024-06-30T12:30:00[America/New_York]", "12:30:00"},
		{"12:30:00[u-ca=gregory]", "12:30:00"},
		{"12:30:00[u-ca=bogus]", "12:30:00"},
		{"2024-06-30T12:30:00[u-ca=japanese]", "12:30:00"},
		{"1330", "13:30:00"},
		{"123045.5", "12:30:45.5"},
		{"120560", "12:05:59"},
		{"23:59:60", "23:59:59"},
		{"1230+00:00", "12:30:00"},
		{"120512.5", "12:05:12.5"},
	}
	for _, c := range cases {
		pt := PlainTimeFromString(c.in)
		if got := pt.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainTimeFromString(%q) toString = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPlainTimeFromStringThrows pins the strings PlainTime.from rejects: a bad grammar, an
// out-of-range field, a Z designator, a date-only string with no time, and the basic bare
// forms that are ambiguous with a year-month or month-day date, all against
// @js-temporal/polyfill.
func TestPlainTimeFromStringThrows(t *testing.T) {
	bad := []string{
		"1230", "0630", "2400", "1260", "12305", "1234567",
		"120512", "130512", "010101", "0229", "0430", "1231",
		"12:60", "24:00:00", "12:30:00Z", "12:30.5", "12.5",
		"2024-06-30", "2024-06-30T12:30:00Z", "2024-13-30T12:30",
		"2024-06-31T12:30", "1230[u-ca=iso8601]", "1230[America/New_York]",
		"1230.5", "+123045", "  12:30  ",
	}
	for _, s := range bad {
		name := catchThrow(func() { PlainTimeFromString(s) })
		if name == "" {
			t.Errorf("PlainTimeFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("PlainTimeFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}

// TestPlainDateTimeFromString pins the string parser feeding Temporal.PlainDateTime.from: a
// date-only string defaults the time to midnight, a date-time string keeps its time, the
// basic and extended forms and the offset are read, the leap second is constrained to :59,
// and a calendar annotation names the calendar, all against @js-temporal/polyfill.
func TestPlainDateTimeFromString(t *testing.T) {
	cases := []struct {
		in      string
		wantStr string
		wantCal string
	}{
		{"2024-06-30", "2024-06-30T00:00:00", "iso8601"},
		{"2024-06-30T12:30:45", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30T12:30", "2024-06-30T12:30:00", "iso8601"},
		{"2024-06-30T12", "2024-06-30T12:00:00", "iso8601"},
		{"20240630", "2024-06-30T00:00:00", "iso8601"},
		{"20240630T123045", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30 12:30:45", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30T12:30:45.123456789", "2024-06-30T12:30:45.123456789", "iso8601"},
		{"2024-06-30T12:30:45+05:30", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30T12:30:45-05:00[America/New_York]", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30T12:30:45[u-ca=gregory]", "2024-06-30T12:30:45[u-ca=gregory]", "gregory"},
		{"2024-06-30[u-ca=japanese]", "2024-06-30T00:00:00[u-ca=japanese]", "japanese"},
		{"2024-06-30T12:30:45[u-ca=iso8601]", "2024-06-30T12:30:45", "iso8601"},
		{"2024-06-30T23:59:60", "2024-06-30T23:59:59", "iso8601"},
		{"-000005-06-30T12:00", "-000005-06-30T12:00:00", "iso8601"},
		{"+002024-06-30T12:00", "2024-06-30T12:00:00", "iso8601"},
	}
	for _, c := range cases {
		pdt := PlainDateTimeFromString(c.in)
		if got := pdt.ToString().ToGoString(); got != c.wantStr {
			t.Errorf("PlainDateTimeFromString(%q) toString = %q, want %q", c.in, got, c.wantStr)
		}
		if got := pdt.CalendarId().ToGoString(); got != c.wantCal {
			t.Errorf("PlainDateTimeFromString(%q) calendarId = %q, want %q", c.in, got, c.wantCal)
		}
	}
}

// TestPlainDateTimeFromStringThrows pins the strings PlainDateTime.from rejects: a bad
// grammar, an out-of-range date, a Z designator, a time-only string with no date, and an
// unhosted calendar each throw a RangeError, all against @js-temporal/polyfill.
func TestPlainDateTimeFromStringThrows(t *testing.T) {
	bad := []string{
		"2024-06-30T12:30:45Z", "2024-06-30Z", "12:30:45", "T12:30:45",
		"2024-13-30T12:30", "2024-06-31T12:30", "2024-06-30T25:00",
		"2024-06-30T12:30:45[u-ca=bogus]", "2024-06", "2024/06/30",
		"  2024-06-30T12:30  ", "2024-06-30T",
	}
	for _, s := range bad {
		name := catchThrow(func() { PlainDateTimeFromString(s) })
		if name == "" {
			t.Errorf("PlainDateTimeFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("PlainDateTimeFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}

// TestPlainYearMonthFromString pins the string parser feeding Temporal.PlainYearMonth.from:
// the bare year-month forms whose day the type does not carry, the full date and date-time
// forms whose year and month it keeps, the expanded year, and the explicit iso8601 annotation,
// all against @js-temporal/polyfill.
func TestPlainYearMonthFromString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2024-06", "2024-06"},
		{"202406", "2024-06"},
		{"2024-06-30", "2024-06"},
		{"20240630", "2024-06"},
		{"2024-06-30T12:30:45", "2024-06"},
		{"2024-06-01", "2024-06"},
		{"2024-06[u-ca=iso8601]", "2024-06"},
		{"2024-06[!u-ca=iso8601]", "2024-06"},
		{"2024-06-30[u-ca=iso8601]", "2024-06"},
		{"+002024-06", "2024-06"},
		{"-000005-06", "-000005-06"},
		{"+002024-06-30T23:59:60", "2024-06"},
	}
	for _, c := range cases {
		ym := PlainYearMonthFromString(c.in)
		if got := ym.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainYearMonthFromString(%q) toString = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPlainYearMonthFromStringThrows pins the strings PlainYearMonth.from rejects: a bad
// grammar, a bare year-month with a time, an out-of-range month or day, a Z designator, and a
// non-ISO calendar on a bare year-month each throw a RangeError, all against
// @js-temporal/polyfill.
func TestPlainYearMonthFromStringThrows(t *testing.T) {
	bad := []string{
		"2024", "06", "12:30:45", "2024-W06", "2024-366",
		"2024-06T12:30", "2024-06 12:30", "202406T1230",
		"2024-13", "2024-00", "2024-6-30", "2024-06-31",
		"2024-02-30", "2024-06-00", "2024-06Z", "2024-06-30T12:30:45Z",
		"2024-06[u-ca=gregory]", "2024-06[u-ca=japanese]", "2024-06[u-ca=bogus]",
	}
	for _, s := range bad {
		name := catchThrow(func() { PlainYearMonthFromString(s) })
		if name == "" {
			t.Errorf("PlainYearMonthFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("PlainYearMonthFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}
