package value

import (
	"math"
	"testing"
	"time"
)

// TestTimeClipNormalizesNegativeZero pins that a time value of zero is the positive zero,
// never the negative one. TimeClip closes with ToIntegerOrInfinity, which maps -0 to +0,
// so a date built from -0 or from a tiny negative fraction reads back +0 through getTime
// and valueOf, and a SameValue(«-0», «0») test cannot see a stray negative zero.
func TestTimeClipNormalizesNegativeZero(t *testing.T) {
	for _, in := range []float64{math.Copysign(0, -1), -0.4, -0.999} {
		d := NewDateFromMillis(in)
		if got := d.GetTime(); got != 0 || math.Signbit(got) {
			t.Errorf("NewDateFromMillis(%v).GetTime() = %v (signbit %v), want +0", in, got, math.Signbit(got))
		}
		if got := d.ValueOf(); got != 0 || math.Signbit(got) {
			t.Errorf("NewDateFromMillis(%v).ValueOf() = %v (signbit %v), want +0", in, got, math.Signbit(got))
		}
	}
}

// TestNewDateReadsTheClock pins that the bare constructor lands on the current moment.
// The window is generous on purpose: the assertion is that a clock was read at all, not
// that it agrees with a second reading to the millisecond.
func TestNewDateReadsTheClock(t *testing.T) {
	before := float64(time.Now().UnixMilli())
	d := NewDate()
	after := float64(time.Now().UnixMilli())
	if d.GetTime() < before || d.GetTime() > after {
		t.Errorf("new Date() = %v, want between %v and %v", d.GetTime(), before, after)
	}
}

// TestTimeValueRoundTrips pins that a date built from a time value gives that value
// back through both of the reads that report it.
func TestTimeValueRoundTrips(t *testing.T) {
	d := NewDateFromMillis(1_700_000_000_000)
	if got := d.GetTime(); got != 1_700_000_000_000 {
		t.Errorf("getTime() = %v, want 1700000000000", got)
	}
	if got := d.ValueOf(); got != 1_700_000_000_000 {
		t.Errorf("valueOf() = %v, want 1700000000000", got)
	}
}

// TestTimeValueTruncates pins that a fractional time value loses its fraction rather
// than rounding, and that it truncates toward zero on both sides of the epoch.
func TestTimeValueTruncates(t *testing.T) {
	if got := NewDateFromMillis(1.9).GetTime(); got != 1 {
		t.Errorf("new Date(1.9) = %v, want 1", got)
	}
	if got := NewDateFromMillis(-1.9).GetTime(); got != -1 {
		t.Errorf("new Date(-1.9) = %v, want -1", got)
	}
}

// TestOutOfRangeIsTheInvalidDate pins TimeClip: a value past the representable range, or
// one that is not a number at all, becomes the Invalid Date rather than throwing at
// construction or silently saturating.
func TestOutOfRangeIsTheInvalidDate(t *testing.T) {
	for _, ms := range []float64{maxTimeValue + 1, -maxTimeValue - 1, math.NaN(), math.Inf(1), math.Inf(-1)} {
		if got := NewDateFromMillis(ms).GetTime(); !math.IsNaN(got) {
			t.Errorf("new Date(%v) = %v, want NaN", ms, got)
		}
	}
	// The boundary itself is representable, so it must survive.
	if got := NewDateFromMillis(maxTimeValue).GetTime(); got != maxTimeValue {
		t.Errorf("new Date(%v) = %v, want the boundary itself", maxTimeValue, got)
	}
}

// TestISOStringFormats pins the serialization across the epoch, a leap day, and the
// millisecond padding that must always print three digits.
func TestISOStringFormats(t *testing.T) {
	for _, c := range []struct {
		ms   float64
		want string
	}{
		{0, "1970-01-01T00:00:00.000Z"},
		{1, "1970-01-01T00:00:00.001Z"},
		{1_700_000_000_000, "2023-11-14T22:13:20.000Z"},
		{951_782_400_000, "2000-02-29T00:00:00.000Z"},
		{1_234_567_890_123, "2009-02-13T23:31:30.123Z"},
	} {
		if got := NewDateFromMillis(c.ms).ToISOString().ToGoString(); got != c.want {
			t.Errorf("toISOString(%v) = %q, want %q", c.ms, got, c.want)
		}
	}
}

// TestISOStringBeforeTheEpoch pins the negative time value, the case Go's own truncating
// division gets wrong: without the Euclidean split a moment inside 1969 would print as
// the day after it with a negative time of day.
func TestISOStringBeforeTheEpoch(t *testing.T) {
	for _, c := range []struct {
		ms   float64
		want string
	}{
		{-1, "1969-12-31T23:59:59.999Z"},
		{-86_400_000, "1969-12-31T00:00:00.000Z"},
		{-2_208_988_800_000, "1900-01-01T00:00:00.000Z"},
	} {
		if got := NewDateFromMillis(c.ms).ToISOString().ToGoString(); got != c.want {
			t.Errorf("toISOString(%v) = %q, want %q", c.ms, got, c.want)
		}
	}
}

// TestISOStringExpandsAWideYear pins the six-digit expanded form, the spelling a year
// outside 0 through 9999 takes.
func TestISOStringExpandsAWideYear(t *testing.T) {
	// 275760-09-13 is the last representable day, the far end of the time value range.
	if got := NewDateFromMillis(maxTimeValue).ToISOString().ToGoString(); got != "+275760-09-13T00:00:00.000Z" {
		t.Errorf("toISOString at the upper bound = %q", got)
	}
	if got := NewDateFromMillis(-maxTimeValue).ToISOString().ToGoString(); got != "-271821-04-20T00:00:00.000Z" {
		t.Errorf("toISOString at the lower bound = %q", got)
	}
}

// TestInvalidDateHasNoISOString pins that serializing the Invalid Date throws a
// RangeError rather than printing a NaN-shaped string, which is what keeps an invalid
// date from leaking into output as though it were a real instant.
func TestInvalidDateHasNoISOString(t *testing.T) {
	defer func() {
		e := recover()
		if e == nil {
			t.Fatal("toISOString of the Invalid Date did not throw")
		}
	}()
	NewDateFromMillis(math.NaN()).ToISOString()
}

// TestDateNowReadsTheSameClock pins the static: it gives a time value directly, without
// building a Date, and it agrees with the constructor.
func TestDateNowReadsTheSameClock(t *testing.T) {
	before := DateNow()
	d := NewDate()
	after := DateNow()
	if d.GetTime() < before || d.GetTime() > after {
		t.Errorf("Date.now() = %v..%v does not bracket new Date() = %v", before, after, d.GetTime())
	}
}

// withZone runs a function with the process's local zone fixed at an offset, so the
// local getters have an answer that does not depend on the machine the test runs on.
func withZone(t *testing.T, offsetSeconds int, f func()) {
	t.Helper()
	saved := time.Local
	time.Local = time.FixedZone("Test", offsetSeconds)
	defer func() { time.Local = saved }()
	f()
}

// TestUTCGettersReadTheComponents pins each UTC getter against a known instant,
// 2023-11-14T22:13:20.000Z, a Tuesday. The month is zero-based and the weekday counts
// from Sunday, both of which are JavaScript's numbering and not the ISO calendar's.
func TestUTCGettersReadTheComponents(t *testing.T) {
	d := NewDateFromMillis(1_700_000_000_123)
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"getUTCFullYear", d.GetUTCFullYear(), 2023},
		{"getUTCMonth", d.GetUTCMonth(), 10},
		{"getUTCDate", d.GetUTCDate(), 14},
		{"getUTCDay", d.GetUTCDay(), 2},
		{"getUTCHours", d.GetUTCHours(), 22},
		{"getUTCMinutes", d.GetUTCMinutes(), 13},
		{"getUTCSeconds", d.GetUTCSeconds(), 20},
		{"getUTCMilliseconds", d.GetUTCMilliseconds(), 123},
	} {
		if c.got != c.want {
			t.Errorf("%s() = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestUTCGettersBeforeTheEpoch pins the components on the other side of 1970, where the
// day split has to step back rather than truncate toward zero.
func TestUTCGettersBeforeTheEpoch(t *testing.T) {
	// 1969-12-31T23:59:59.999Z, a Wednesday.
	d := NewDateFromMillis(-1)
	for _, c := range []struct {
		name string
		got  float64
		want float64
	}{
		{"getUTCFullYear", d.GetUTCFullYear(), 1969},
		{"getUTCMonth", d.GetUTCMonth(), 11},
		{"getUTCDate", d.GetUTCDate(), 31},
		{"getUTCDay", d.GetUTCDay(), 3},
		{"getUTCHours", d.GetUTCHours(), 23},
		{"getUTCMilliseconds", d.GetUTCMilliseconds(), 999},
	} {
		if c.got != c.want {
			t.Errorf("%s() = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestLocalGettersShiftByTheZone pins that the local getters read the local wall clock,
// with a half-hour zone chosen so a wrong shift shows up in the minutes as well as the
// hours, and past midnight so the date and the weekday move too.
func TestLocalGettersShiftByTheZone(t *testing.T) {
	withZone(t, 5*3600+1800, func() {
		// 2023-11-14T22:13:20Z is 2023-11-15T03:43:20 at UTC+05:30, a Wednesday.
		d := NewDateFromMillis(1_700_000_000_000)
		for _, c := range []struct {
			name string
			got  float64
			want float64
		}{
			{"getFullYear", d.GetFullYear(), 2023},
			{"getMonth", d.GetMonth(), 10},
			{"getDate", d.GetDate(), 15},
			{"getDay", d.GetDay(), 3},
			{"getHours", d.GetHours(), 3},
			{"getMinutes", d.GetMinutes(), 43},
			{"getSeconds", d.GetSeconds(), 20},
			{"getTimezoneOffset", d.GetTimezoneOffset(), -330},
		} {
			if c.got != c.want {
				t.Errorf("%s() at UTC+05:30 = %v, want %v", c.name, c.got, c.want)
			}
		}
	})
}

// TestTimezoneOffsetSignsWithTheSpecification pins the sign, which is the opposite of
// what a zone name suggests: the offset is what you add to local time to reach UTC, so a
// zone ahead of UTC reports a negative number and one behind reports a positive one.
func TestTimezoneOffsetSignsWithTheSpecification(t *testing.T) {
	withZone(t, -8*3600, func() {
		if got := NewDateFromMillis(0).GetTimezoneOffset(); got != 480 {
			t.Errorf("getTimezoneOffset() at UTC-08:00 = %v, want 480", got)
		}
	})
	withZone(t, 0, func() {
		if got := NewDateFromMillis(0).GetTimezoneOffset(); got != 0 {
			t.Errorf("getTimezoneOffset() at UTC = %v, want 0", got)
		}
	})
}

// TestInvalidDateGettersAreNaN pins that every read of the Invalid Date propagates
// rather than quietly reporting the epoch, which is the whole point of a guard that
// lives in one place instead of in each getter.
func TestInvalidDateGettersAreNaN(t *testing.T) {
	d := NewDateFromMillis(math.NaN())
	for name, got := range map[string]float64{
		"getFullYear":        d.GetFullYear(),
		"getMonth":           d.GetMonth(),
		"getDate":            d.GetDate(),
		"getDay":             d.GetDay(),
		"getHours":           d.GetHours(),
		"getMinutes":         d.GetMinutes(),
		"getSeconds":         d.GetSeconds(),
		"getMilliseconds":    d.GetMilliseconds(),
		"getUTCFullYear":     d.GetUTCFullYear(),
		"getUTCMonth":        d.GetUTCMonth(),
		"getUTCDate":         d.GetUTCDate(),
		"getUTCDay":          d.GetUTCDay(),
		"getUTCHours":        d.GetUTCHours(),
		"getUTCMinutes":      d.GetUTCMinutes(),
		"getUTCSeconds":      d.GetUTCSeconds(),
		"getUTCMilliseconds": d.GetUTCMilliseconds(),
		"getTimezoneOffset":  d.GetTimezoneOffset(),
	} {
		if !math.IsNaN(got) {
			t.Errorf("%s() of the Invalid Date = %v, want NaN", name, got)
		}
	}
}

// TestLocalGettersReconstructTheTimeValue pins the local split against the offset it was
// built from: the components, read back as UTC and shifted by the offset, must land on
// the instant they came from. This is the invariant that catches a shift applied in the
// wrong direction, which a single-zone check with a symmetric offset could miss.
func TestLocalGettersReconstructTheTimeValue(t *testing.T) {
	withZone(t, 9*3600, func() {
		const ms = 1_234_567_890_123
		d := NewDateFromMillis(ms)
		shifted := NewDateFromMillis(ms - d.GetTimezoneOffset()*60_000)
		if d.GetHours() != shifted.GetUTCHours() || d.GetDate() != shifted.GetUTCDate() {
			t.Errorf("local read %v-%v does not match the shifted UTC read %v-%v",
				d.GetDate(), d.GetHours(), shifted.GetUTCDate(), shifted.GetUTCHours())
		}
	})
}
