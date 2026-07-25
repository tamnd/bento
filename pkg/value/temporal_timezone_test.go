package value

import "testing"

// resolveTimeZone mirrors Temporal's ToTemporalTimeZoneIdentifier: the "UTC" name is
// matched case insensitively and canonicalizes to upper case, and a full ISO date-time
// string used as a time-zone identifier resolves through its bracketed annotation rather
// than its own offset.
func TestResolveTimeZoneCaseInsensitiveAndBracket(t *testing.T) {
	cases := []struct {
		id   string
		want string
	}{
		{"UTC", "UTC"},
		{"uTc", "UTC"},
		{"UtC", "UTC"},
		{"America/New_York", "America/New_York"},
		{"+05:30", "+05:30"},
		// The bracket annotation wins over the string's own -12:12 offset.
		{"2021-08-19T17:30:45.123456789-12:12[+01:46]", "+01:46"},
	}
	for _, c := range cases {
		z := InstantFromEpochMilliseconds(0).ToZonedDateTimeISO(c.id)
		if got := z.TimeZoneId().ToGoString(); got != c.want {
			t.Errorf("ToZonedDateTimeISO(%q).timeZoneId = %q, want %q", c.id, got, c.want)
		}
	}
}
