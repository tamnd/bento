package value

import (
	"math"
	"testing"
	"time"
)

// parseMs is the time value a string names, for a test that does not want to spell out
// the BStr conversion each time.
func parseMs(s string) float64 { return ParseDate(FromGoString(s)) }

// TestParseISOFormats pins the grammar the specification fixes, one row per shape it
// admits: the three date-only widths, the time with and without seconds and a fraction,
// and both spellings of an offset.
func TestParseISOFormats(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"2023", 1_672_531_200_000},                      // 2023-01-01T00:00:00Z
		{"2023-11", 1_698_796_800_000},                   // 2023-11-01T00:00:00Z
		{"2023-11-14", 1_699_920_000_000},                // midnight UTC
		{"2023-11-14T22:13:20Z", 1_700_000_000_000},      //
		{"2023-11-14T22:13Z", 1_699_999_980_000},         // seconds default to zero
		{"2023-11-14T22:13:20.123Z", 1_700_000_000_123},  //
		{"2023-11-14T22:13:20.1Z", 1_700_000_000_100},    // a short fraction pads
		{"2023-11-14T22:13:20.1239Z", 1_700_000_000_123}, // a long one truncates
		{"2023-11-15T03:43:20+05:30", 1_700_000_000_000}, // an offset ahead of UTC
		{"2023-11-14T14:13:20-08:00", 1_700_000_000_000}, // and one behind
		{"2023-11-15T03:43:20+0530", 1_700_000_000_000},  // the colon is optional
		{"+002023-11-14T22:13:20Z", 1_700_000_000_000},   // the expanded year
		{"1969-12-31T23:59:59.999Z", -1},                 // before the epoch
		{"2023-11-15T00:00:00Z", 1_700_006_400_000},      //
		{"2023-11-14T24:00:00Z", 1_700_006_400_000},      // hour 24 is the next midnight
		{"-000001-01-01T00:00:00Z", -62_198_755_200_000}, // a year before the era
		{"  2023-11-14T22:13:20Z  ", 1_700_000_000_000},  // surrounding space is trimmed
	} {
		if got := parseMs(c.in); got != c.want {
			t.Errorf("parse(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseRejectsWhatIsNotADate pins that a string outside every format this runtime
// reads gives the Invalid Date rather than a guess. Most of these are near misses: an
// unpadded field, an out-of-range component, a separator in the wrong place, or trailing
// text, each of which a lenient parser would happily accept as something the string did
// not say.
func TestParseRejectsWhatIsNotADate(t *testing.T) {
	for _, in := range []string{
		"",
		"not a date",
		"2023-1-14",                  // the month is not padded
		"2023-13-01",                 // there is no month 13
		"2023-11-31",                 // November has 30 days
		"2023-02-29",                 // 2023 is not a leap year
		"2023-11-14T25:00:00Z",       // there is no hour 25
		"2023-11-14T22:60:00Z",       // nor minute 60
		"2023-11-14T22:13:20.Z",      // a fraction point with no digits
		"2023-11-14T22:13:20Z extra", // trailing text
		"2023-11-14Z",                // a date-only string takes no offset
		"2023-11-14T22:13:20+24:00",  // there is no offset of a full day
		"20231114",                   // the basic format is outside the profile
		"-000000-01-01T00:00:00Z",    // year zero is not written with a sign
	} {
		if got := parseMs(in); !math.IsNaN(got) {
			t.Errorf("parse(%q) = %v, want the Invalid Date", in, got)
		}
	}
	// A leap day in a leap year is the same shape and must still parse, so the February
	// rejection above is about the year and not about the parser refusing the 29th.
	if got := parseMs("2024-02-29"); math.IsNaN(got) {
		t.Error("parse of a real leap day gave the Invalid Date")
	}
}

// TestDateOnlyIsUTCAndDateTimeIsLocal pins the rule that is easy to miss and impossible
// to guess at: a date-only string is UTC, and the same date with a time but no offset is
// local. They are different instants everywhere but Greenwich, and that is specified
// rather than accidental.
func TestDateOnlyIsUTCAndDateTimeIsLocal(t *testing.T) {
	withZone(t, 5*3600+1800, func() {
		utc := parseMs("2023-11-14")
		local := parseMs("2023-11-14T00:00:00")
		if utc != 1_699_920_000_000 {
			t.Errorf("the date-only string parsed as %v, want midnight UTC", utc)
		}
		if want := utc - float64(5*3600+1800)*1000; local != want {
			t.Errorf("the date-time string parsed as %v, want %v, local midnight", local, want)
		}
	})
}

// TestParseAcrossADaylightSavingJump pins that local parsing goes through the zone
// database rather than a fixed offset. In New York the same wall clock reading is a
// different instant in January and in July, and the two must differ by exactly the hour
// the clocks moved.
func TestParseAcrossADaylightSavingJump(t *testing.T) {
	ny, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip("no zone database on this machine")
	}
	saved := time.Local
	time.Local = ny
	defer func() { time.Local = saved }()

	winter := parseMs("2023-01-15T12:00:00")
	summer := parseMs("2023-07-15T12:00:00")
	if want := float64(time.Date(2023, 1, 15, 12, 0, 0, 0, ny).UnixMilli()); winter != want {
		t.Errorf("the winter reading parsed as %v, want %v", winter, want)
	}
	if want := float64(time.Date(2023, 7, 15, 12, 0, 0, 0, ny).UnixMilli()); summer != want {
		t.Errorf("the summer reading parsed as %v, want %v", summer, want)
	}
}

// TestParseTheLegacyFormats pins the two non-ISO spellings this runtime reads: the mail
// and HTTP date a server sends, and the toString spelling a date arrives as when it went
// through a string and back.
func TestParseTheLegacyFormats(t *testing.T) {
	for _, c := range []struct {
		in   string
		want float64
	}{
		{"Tue, 14 Nov 2023 22:13:20 GMT", 1_700_000_000_000},
		{"Tue, 14 Nov 2023 17:13:20 -0500", 1_700_000_000_000},
		{"Tue Nov 14 2023 22:13:20 GMT+0000 (Coordinated Universal Time)", 1_700_000_000_000},
		{"Tue Nov 14 2023 14:13:20 GMT-0800 (Pacific Standard Time)", 1_700_000_000_000},
	} {
		if got := parseMs(c.in); got != c.want {
			t.Errorf("parse(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestParseClipsAnUnrepresentableInstant pins that a well-formed string naming a moment
// outside the representable range is the Invalid Date rather than a wrapped number.
func TestParseClipsAnUnrepresentableInstant(t *testing.T) {
	if got := parseMs("+275761-01-01T00:00:00Z"); !math.IsNaN(got) {
		t.Errorf("parse past the upper bound = %v, want the Invalid Date", got)
	}
	if got := parseMs("+275760-09-13T00:00:00Z"); got != maxTimeValue {
		t.Errorf("parse at the upper bound = %v, want %v", got, maxTimeValue)
	}
}

// TestNewDateFromStringRoundTrips pins the constructor half against the serialization: a
// date printed as ISO and read back must be the same instant.
func TestNewDateFromStringRoundTrips(t *testing.T) {
	for _, ms := range []float64{0, 1_700_000_000_123, -2_208_988_800_000, maxTimeValue} {
		d := NewDateFromMillis(ms)
		back := NewDateFromString(d.ToISOString())
		if back.GetTime() != ms {
			t.Errorf("round trip of %v through %q gave %v", ms, d.ToISOString().ToGoString(), back.GetTime())
		}
	}
}

// TestNewDateFromStringIsInvalidNotAThrow pins that an unparsable string constructs. A
// bad date is something a program checks for with isNaN, not something it catches.
func TestNewDateFromStringIsInvalidNotAThrow(t *testing.T) {
	d := NewDateFromString(FromGoString("nonsense"))
	if !math.IsNaN(d.GetTime()) {
		t.Errorf("new Date(\"nonsense\") = %v, want the Invalid Date", d.GetTime())
	}
}
