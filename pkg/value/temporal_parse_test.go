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
