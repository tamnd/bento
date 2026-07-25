package value

import (
	"math"
	"testing"
	"time"
)

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
