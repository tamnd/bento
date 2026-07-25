package value

import (
	"math"
	"testing"
)

// TestDateUTCBuildsFromComponents pins the UTC construction, including the defaults a
// short argument list falls back on: the first of the month at midnight.
func TestDateUTCBuildsFromComponents(t *testing.T) {
	for _, c := range []struct {
		args []float64
		want float64
	}{
		{[]float64{2023, 10, 14, 22, 13, 20, 123}, 1_700_000_000_123},
		{[]float64{2023, 10, 14}, 1_699_920_000_000},   // midnight
		{[]float64{2023, 10}, 1_698_796_800_000},       // the first of November
		{[]float64{1970, 0, 1}, 0},                     // the epoch itself
		{[]float64{1969, 11, 31, 23, 59, 59, 999}, -1}, // one millisecond before it
	} {
		if got := DateUTC(c.args...); got != c.want {
			t.Errorf("Date.UTC(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

// TestComponentsOverflowIntoTheFieldAbove pins the rule that makes this arithmetic rather
// than validation: an out-of-range component carries. That is what lets a program add
// forty-five days to a date without counting month lengths itself.
func TestComponentsOverflowIntoTheFieldAbove(t *testing.T) {
	for _, c := range []struct {
		name string
		args []float64
		want []float64 // year, month, day, hour
	}{
		{"month 12 is next January", []float64{2023, 12, 1}, []float64{2024, 0, 1, 0}},
		{"month -1 is last December", []float64{2023, -1, 1}, []float64{2022, 11, 1, 0}},
		{"day 0 is the last of the month before", []float64{2023, 10, 0}, []float64{2023, 9, 31, 0}},
		{"day 32 rolls the month", []float64{2023, 0, 32}, []float64{2023, 1, 1, 0}},
		{"hour 25 rolls the day", []float64{2023, 0, 1, 25}, []float64{2023, 0, 2, 1}},
		{"a day past a leap day", []float64{2024, 1, 30}, []float64{2024, 2, 1, 0}},
	} {
		d := NewDateFromMillis(DateUTC(c.args...))
		got := []float64{d.GetUTCFullYear(), d.GetUTCMonth(), d.GetUTCDate(), d.GetUTCHours()}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: Date.UTC(%v) read back as %v, want %v", c.name, c.args, got, c.want)
				break
			}
		}
	}
}

// TestTwoDigitYearsAreTheNineteenHundreds pins the legacy rule, which applies to the
// constructor and to Date.UTC and to nothing else.
func TestTwoDigitYearsAreTheNineteenHundreds(t *testing.T) {
	if got := NewDateFromMillis(DateUTC(99, 0, 1)).GetUTCFullYear(); got != 1999 {
		t.Errorf("Date.UTC(99, 0, 1) is year %v, want 1999", got)
	}
	if got := NewDateFromMillis(DateUTC(0, 0, 1)).GetUTCFullYear(); got != 1900 {
		t.Errorf("Date.UTC(0, 0, 1) is year %v, want 1900", got)
	}
	// A hundred is a real year, not a two-digit one.
	if got := NewDateFromMillis(DateUTC(100, 0, 1)).GetUTCFullYear(); got != 100 {
		t.Errorf("Date.UTC(100, 0, 1) is year %v, want 100", got)
	}
	// setFullYear is outside the rule, so it can name the year five.
	d := NewDateFromMillis(0)
	d.SetUTCFullYear(5)
	if got := d.GetUTCFullYear(); got != 5 {
		t.Errorf("setUTCFullYear(5) is year %v, want 5", got)
	}
}

// TestANonFiniteComponentIsTheInvalidDate pins that a NaN propagates rather than
// defaulting to zero, since a date built from a zero the caller did not pass is the wrong
// instant reported as a right one.
func TestANonFiniteComponentIsTheInvalidDate(t *testing.T) {
	for _, args := range [][]float64{
		{math.NaN(), 0, 1},
		{2023, math.NaN(), 1},
		{2023, 0, math.Inf(1)},
	} {
		if got := DateUTC(args...); !math.IsNaN(got) {
			t.Errorf("Date.UTC(%v) = %v, want the Invalid Date", args, got)
		}
	}
}

// TestComponentConstructorIsLocal pins the thing about this constructor that surprises
// people: the reading is local time, not UTC, so the same arguments name a different
// instant in a different zone.
func TestComponentConstructorIsLocal(t *testing.T) {
	withZone(t, 9*3600, func() {
		d := NewDateFromComponents(2023, 10, 14, 0, 0, 0, 0)
		if got := d.GetHours(); got != 0 {
			t.Errorf("the local hour is %v, want midnight", got)
		}
		// Midnight at UTC+09:00 is three in the afternoon of the day before in UTC.
		if got := d.GetUTCHours(); got != 15 {
			t.Errorf("the UTC hour is %v, want 15", got)
		}
		if got := d.GetUTCDate(); got != 13 {
			t.Errorf("the UTC date is %v, want the 13th", got)
		}
	})
}

// TestSettersReplaceTheirFieldsOnly pins that a setter moves the field it names and the
// ones it was given, and leaves everything else alone.
func TestSettersReplaceTheirFieldsOnly(t *testing.T) {
	d := NewDateFromMillis(DateUTC(2023, 10, 14, 22, 13, 20, 123))
	d.SetUTCHours(9)
	if got := d.ToISOString().ToGoString(); got != "2023-11-14T09:13:20.123Z" {
		t.Errorf("after setUTCHours(9): %s", got)
	}
	d.SetUTCHours(1, 2)
	if got := d.ToISOString().ToGoString(); got != "2023-11-14T01:02:20.123Z" {
		t.Errorf("after setUTCHours(1, 2): %s", got)
	}
	d.SetUTCHours(3, 4, 5, 6)
	if got := d.ToISOString().ToGoString(); got != "2023-11-14T03:04:05.006Z" {
		t.Errorf("after setUTCHours(3, 4, 5, 6): %s", got)
	}
	d.SetUTCFullYear(2000, 0, 2)
	if got := d.ToISOString().ToGoString(); got != "2000-01-02T03:04:05.006Z" {
		t.Errorf("after setUTCFullYear(2000, 0, 2): %s", got)
	}
}

// TestSettersOverflow pins that a setter carries the same way the constructor does, which
// is the whole reason a program reaches for one: adding to a component and setting it back
// is how date arithmetic is written.
func TestSettersOverflow(t *testing.T) {
	d := NewDateFromMillis(DateUTC(2023, 0, 20))
	d.SetUTCDate(d.GetUTCDate() + 45)
	if got := d.ToISOString().ToGoString(); got != "2023-03-06T00:00:00.000Z" {
		t.Errorf("forty-five days after 2023-01-20 is %s, want 2023-03-06", got)
	}
	d = NewDateFromMillis(DateUTC(2023, 11, 15))
	d.SetUTCMonth(d.GetUTCMonth() + 1)
	if got := d.ToISOString().ToGoString(); got != "2024-01-15T00:00:00.000Z" {
		t.Errorf("a month after 2023-12-15 is %s, want 2024-01-15", got)
	}
}

// TestSetTimeReplacesTheInstant pins the one setter that takes an instant rather than a
// calendar field, including that it clips.
func TestSetTimeReplacesTheInstant(t *testing.T) {
	d := NewDateFromMillis(0)
	if got := d.SetTime(1_700_000_000_000); got != 1_700_000_000_000 || d.GetTime() != got {
		t.Errorf("setTime gave %v and left %v", got, d.GetTime())
	}
	if got := d.SetTime(math.Inf(1)); !math.IsNaN(got) {
		t.Errorf("setTime(Infinity) = %v, want the Invalid Date", got)
	}
}

// TestSettersOnTheInvalidDate pins the exception the specification carves out: a setter on
// an invalid date leaves it invalid, because there are no components to build on, except
// the two year setters, since a year is enough to name a date on its own.
func TestSettersOnTheInvalidDate(t *testing.T) {
	d := NewDateFromMillis(math.NaN())
	if got := d.SetUTCHours(9); !math.IsNaN(got) {
		t.Errorf("setUTCHours on the Invalid Date = %v, want NaN", got)
	}
	if got := d.SetUTCMonth(3); !math.IsNaN(got) {
		t.Errorf("setUTCMonth on the Invalid Date = %v, want NaN", got)
	}
	d.SetUTCFullYear(2023)
	if got := d.ToISOString().ToGoString(); got != "2023-01-01T00:00:00.000Z" {
		t.Errorf("setUTCFullYear on the Invalid Date gave %s, want it built on the epoch", got)
	}
}

// TestSettersGiveBackTheTimeValue pins that a setter is usable as an expression, which is
// what its return value is for.
func TestSettersGiveBackTheTimeValue(t *testing.T) {
	d := NewDateFromMillis(0)
	if got := d.SetUTCDate(2); got != 86_400_000 || got != d.GetTime() {
		t.Errorf("setUTCDate gave %v, want the new time value %v", got, d.GetTime())
	}
}

// TestLocalSettersGoThroughTheZone pins that a local setter rebuilds through the zone
// rather than through a fixed offset: writing the local hour must leave the local hour
// where it was written, whatever the zone.
func TestLocalSettersGoThroughTheZone(t *testing.T) {
	withZone(t, -8*3600, func() {
		d := NewDateFromMillis(1_700_000_000_000)
		d.SetHours(9, 30)
		if h, m := d.GetHours(), d.GetMinutes(); h != 9 || m != 30 {
			t.Errorf("after setHours(9, 30) the local clock reads %v:%v", h, m)
		}
	})
}
