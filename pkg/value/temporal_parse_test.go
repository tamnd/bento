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
		{"2024-06-30T12:30:45+05:00:00.123456789", "2024-06-30T12:30:45", "iso8601"},
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
		"2024-06-30T12:30:45+05:30.5", "2024-06-30T12:30:45+05.5",
		"2024-06-30T12:30:45+05:00:00.0000000001", "2024-06-30T12:30:45+05:00:00.",
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

// TestInstantFromString pins the string parser feeding Temporal.Instant.from: the wall-clock
// reading is taken as UTC and the offset is subtracted to reach the epoch count, a Z designator
// is a zero offset, a sub-minute offset shifts the seconds, and a calendar annotation is ignored,
// all against @js-temporal/polyfill.
func TestInstantFromString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2020-01-01T00:00:00Z", "2020-01-01T00:00:00Z"},
		{"2020-01-01T12:30:45-05:00", "2020-01-01T17:30:45Z"},
		{"2020-01-01T12:30:45.123456789+01:00", "2020-01-01T11:30:45.123456789Z"},
		{"2020-01-01T00:00:00-05:00[America/New_York]", "2020-01-01T05:00:00Z"},
		{"2020-01-01T00:00:00+05:00:30", "2019-12-31T18:59:30Z"},
		{"19700101T000000Z", "1970-01-01T00:00:00Z"},
		{"2020-01-01T00Z", "2020-01-01T00:00:00Z"},
		{"-000001-01-01T00:00:00Z", "-000001-01-01T00:00:00Z"},
		{"2020-01-01T00:00:00Z[u-ca=hebrew]", "2020-01-01T00:00:00Z"},
		{"2020-01-01 00:00:00+00:00", "2020-01-01T00:00:00Z"},
	}
	for _, c := range cases {
		if got := InstantFromString(c.in).ToString().ToGoString(); got != c.want {
			t.Errorf("InstantFromString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestInstantFromStringThrows pins the strings Instant.from rejects: a date-only or offset-less
// string, a grammar violation, a count past the Instant range, and a critical non-calendar
// annotation each throw a RangeError, all against @js-temporal/polyfill.
func TestInstantFromStringThrows(t *testing.T) {
	bad := []string{
		"2020-01-01", "2020-01-01T00:00:00", "2020-01-01T24:00:00Z",
		"not-a-date", "+275760-09-14T00:00:00Z", "2020-01-01T00:00:00Z[!foo=bar]",
	}
	for _, s := range bad {
		name := catchThrow(func() { InstantFromString(s) })
		if name == "" {
			t.Errorf("InstantFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("InstantFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}

// TestZonedDateTimeFromString pins the string parser feeding Temporal.ZonedDateTime.from: a
// bare wall-clock reading resolved through the zone, a matching numeric offset, a Z designator
// giving the exact instant, the UTC and fixed-offset zones, a fall-back overlap taking the
// earlier reading, and a spring-forward gap shifting past the transition, all against
// @js-temporal/polyfill.
func TestZonedDateTimeFromString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"2020-06-15T12:30:00[America/New_York]", "2020-06-15T12:30:00-04:00[America/New_York]"},
		{"2020-06-15T12:30:00-04:00[America/New_York]", "2020-06-15T12:30:00-04:00[America/New_York]"},
		{"2020-06-15T12:30:00Z[America/New_York]", "2020-06-15T08:30:00-04:00[America/New_York]"},
		{"2020-06-15T12:30:00[UTC]", "2020-06-15T12:30:00+00:00[UTC]"},
		{"2020-06-15T12:30:00-05:00[-05:00]", "2020-06-15T12:30:00-05:00[-05:00]"},
		{"2020-06-15[America/New_York]", "2020-06-15T00:00:00-04:00[America/New_York]"},
		{"2020-11-01T01:30:00[America/New_York]", "2020-11-01T01:30:00-04:00[America/New_York]"},
		{"2020-11-01T01:30:00-05:00[America/New_York]", "2020-11-01T01:30:00-05:00[America/New_York]"},
		{"2020-03-08T02:30:00[America/New_York]", "2020-03-08T03:30:00-04:00[America/New_York]"},
	}
	for _, c := range cases {
		if got := ZonedDateTimeFromString(c.in).ToString().ToGoString(); got != c.want {
			t.Errorf("ZonedDateTimeFromString(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestZonedDateTimeFromStringThrows pins the strings ZonedDateTime.from rejects: a string with
// no time-zone annotation, an offset the zone never applies for that wall clock, an unknown
// zone, a non-ISO calendar, and an out-of-range date each throw a RangeError, all against
// @js-temporal/polyfill.
func TestZonedDateTimeFromStringThrows(t *testing.T) {
	bad := []string{
		"2020-06-15T12:30:00",
		"2020-06-15T12:30:00Z",
		"2020-06-15T12:30:00-04:00",
		"2020-06-15T12:30:00-05:00[America/New_York]",
		"2020-06-15T12:30:00-04:00[-05:00]",
		"2020-06-15T12:30:00[Not/AZone]",
		"2020-06-15T12:30:00[America/New_York][u-ca=gregory]",
		"2020-13-01T00:00:00[UTC]",
	}
	for _, s := range bad {
		name := catchThrow(func() { ZonedDateTimeFromString(s) })
		if name == "" {
			t.Errorf("ZonedDateTimeFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("ZonedDateTimeFromString(%q) threw %s, want RangeError", s, name)
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

// TestTemporalAnnotationGrammarAccepts pins the RFC 9557 annotations the shared parser accepts
// through the PlainDate path: a time zone with or without a critical marker, a known or unknown
// lowercase key, a key with digits, underscores, or hyphens, and the first of two calendar
// annotations winning. All checked against @js-temporal/polyfill.
func TestTemporalAnnotationGrammarAccepts(t *testing.T) {
	cases := []struct{ in, wantCal string }{
		{"2024-06-15[America/New_York]", "iso8601"},
		{"2024-06-15[!America/New_York]", "iso8601"},
		{"2024-06-15[u-ca=iso8601]", "iso8601"},
		{"2024-06-15[!u-ca=iso8601]", "iso8601"},
		{"2024-06-15[foo=bar]", "iso8601"},
		{"2024-06-15[foo1=bar]", "iso8601"},
		{"2024-06-15[foo_bar=baz]", "iso8601"},
		{"2024-06-15[u-ca=iso8601][foo=bar]", "iso8601"},
		{"2024-06-15[u-ca=gregory][u-ca=iso8601]", "gregory"},
	}
	for _, c := range cases {
		d := PlainDateFromString(c.in)
		if got := d.CalendarId().ToGoString(); got != c.wantCal {
			t.Errorf("PlainDateFromString(%q) calendar = %q, want %q", c.in, got, c.wantCal)
		}
	}
}

// TestTemporalAnnotationGrammarRejects pins the RFC 9557 annotations the shared parser rejects
// through the PlainDate path: an uppercase or mixed-case key, a leading-digit key, an empty
// bracket, and a critical annotation whose key is not "u-ca". Each throws a RangeError, matching
// @js-temporal/polyfill.
func TestTemporalAnnotationGrammarRejects(t *testing.T) {
	bad := []string{
		"2024-06-15[FOO=bar]",
		"2024-06-15[u-CA=iso8601]",
		"2024-06-15[U-CA=iso8601]",
		"2024-06-15[9foo=bar]",
		"2024-06-15[]",
		"2024-06-15[!foo=bar]",
		"2024-06-15[!x-y=z]",
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

// TestPlainMonthDayFromString pins the strings Temporal.PlainMonthDay.from accepts: the bare
// extended and basic month-day forms with an optional "--" prefix, a full date or date-time
// string whose month and day it keeps, an expanded-year full date, an explicit iso8601
// annotation, and the yearless forms that admit a day out of range for the month. All checked
// against @js-temporal/polyfill.
func TestPlainMonthDayFromString(t *testing.T) {
	cases := []struct{ in, want string }{
		{"10-01", "10-01"},
		{"1001", "10-01"},
		{"--10-01", "10-01"},
		{"--1001", "10-01"},
		{"1976-10-01", "10-01"},
		{"19761001", "10-01"},
		{"1976-10-01T15:23:30.1+00:00", "10-01"},
		{"2024-06-30T12:30:45", "06-30"},
		{"-999999-10-01", "10-01"},
		{"+999999-10-01", "10-01"},
		{"10-01[u-ca=iso8601]", "10-01"},
		{"10-01[!u-ca=iso8601]", "10-01"},
		{"2024-02-29", "02-29"},
		{"02-29", "02-29"},
		{"02-30", "02-30"},
		{"06-31", "06-31"},
	}
	for _, c := range cases {
		md := PlainMonthDayFromString(c.in)
		if got := md.ToString().ToGoString(); got != c.want {
			t.Errorf("PlainMonthDayFromString(%q) toString = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestPlainMonthDayFromStringThrows pins the strings PlainMonthDay.from rejects: a bad grammar, a
// single leading dash, a bare month-day with a time, an out-of-range month or day, a day out of
// range on a full date, a Z designator, and a non-ISO calendar on a bare month-day each throw a
// RangeError, all against @js-temporal/polyfill.
func TestPlainMonthDayFromStringThrows(t *testing.T) {
	bad := []string{
		"10", "100", "10-1", "1-01", "-10-01", "---10-01", "202406",
		"13-01", "00-15", "06-00", "06-32", "02-32",
		"10-01T00:00", "10-01 12:00",
		"2024-06-31", "2024-02-30", "2024-13-01",
		"2024-06-15Z", "10-01Z",
		"06-15[u-ca=gregory]", "06-15[u-ca=hebrew]",
	}
	for _, s := range bad {
		name := catchThrow(func() { PlainMonthDayFromString(s) })
		if name == "" {
			t.Errorf("PlainMonthDayFromString(%q) did not throw, want RangeError", s)
			continue
		}
		if name != "RangeError" {
			t.Errorf("PlainMonthDayFromString(%q) threw %s, want RangeError", s, name)
		}
	}
}
