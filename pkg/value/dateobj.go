package value

// This file gives the AOT runtime Date, the built-in every JavaScript program that
// touches a clock reaches for, and the one built-in Node code uses more than any other
// after the collections. Until now every spelling of it handed the unit back, including
// Date.now(), so a compiled .js entry could not read the time at all.
//
// A Date is a single Number: the time value, milliseconds since the epoch. That is the
// whole of its state, which is why the type is one float64 wide and why an out-of-range
// or unparsable input becomes the Invalid Date, a NaN time value rather than an error at
// construction. The formatting and the component reads derive from that number, and this
// first slice covers the number itself; the calendar getters are their own slice.
//
// The date arithmetic leans on the helpers the Temporal implementation already owns
// (epochDaysToISO and the digit formatters), so the two built-ins agree on the civil
// calendar rather than each carrying their own.

import (
	"math"
	"time"
)

// maxTimeValue is the largest magnitude a time value may have, 100,000,000 days either
// side of the epoch in milliseconds. A number outside that range is not a representable
// instant, so it becomes the Invalid Date.
const maxTimeValue = 8.64e15

// Date is bento's runtime representation of a JavaScript Date. It holds the time value,
// the milliseconds since the epoch, and nothing else: every read a Date answers is
// derived from that one number. A NaN time value is the Invalid Date, the state a
// construction from an out-of-range or unparsable input lands in.
type Date struct {
	ms float64
}

// NewDate builds a Date for the current moment, the lowering of new Date(). It reads the
// same wall clock the rest of the runtime does and truncates to whole milliseconds, the
// resolution a time value has.
func NewDate() *Date {
	return &Date{ms: float64(time.Now().UnixMilli())}
}

// NewDateFromMillis builds a Date for a time value, the lowering of new Date(ms). The
// value is clipped the way the specification's TimeClip does: a non-finite or
// out-of-range number, and a fraction, each land where they must, which is the Invalid
// Date for the first two and the truncated integer for the third.
func NewDateFromMillis(ms float64) *Date {
	return &Date{ms: timeClip(ms)}
}

// DateNow is the current time value as a Number, the lowering of Date.now(). It is the
// same clock new Date() reads, handed back as the number rather than wrapped, since
// that is what the static gives.
func DateNow() float64 {
	return float64(time.Now().UnixMilli())
}

// timeClip is the specification's TimeClip: a non-finite value or one beyond the
// representable range is not an instant, so it becomes NaN, and a fractional value
// truncates toward zero, since a time value is a whole number of milliseconds.
func timeClip(ms float64) float64 {
	if math.IsNaN(ms) || math.IsInf(ms, 0) || math.Abs(ms) > maxTimeValue {
		return math.NaN()
	}
	return math.Trunc(ms)
}

// GetTime is the time value, the lowering of date.getTime(). It is NaN for the Invalid
// Date, which is what makes an invalid date compare false against everything including
// itself.
func (d *Date) GetTime() float64 { return d.ms }

// ValueOf is the time value, the lowering of date.valueOf() and the number a Date
// coerces to in arithmetic. It is the same number getTime gives.
func (d *Date) ValueOf() float64 { return d.ms }

// ToISOString is the date as the ISO 8601 string in UTC, the lowering of
// date.toISOString(). The format is fixed: a four-digit year, or the expanded six-digit
// form with a sign for a year outside 0 through 9999, then the date, the time to
// milliseconds with all three fraction digits always present, and the Z designator. The
// Invalid Date has no ISO spelling, so it throws a RangeError, which is what the
// specification requires and what makes an invalid date impossible to serialize by
// accident.
func (d *Date) ToISOString() BStr {
	if math.IsNaN(d.ms) {
		Throw(NewRangeError(FromGoString("Invalid time value")))
	}
	p := splitTimeValue(d.ms)
	s := formatISOYear(p.year) + "-" + twoDigit(p.month) + "-" + twoDigit(p.day) +
		"T" + twoDigit(p.hour) + ":" + twoDigit(p.minute) + ":" + twoDigit(p.second) +
		"." + zeroPad(p.milli, 3) + "Z"
	return FromGoString(s)
}

// euclidDivMod divides with a non-negative remainder, the split a time value needs to
// land on the correct day. Go's own division truncates toward zero, which for a time
// value before the epoch would give the day after the right one and a negative time of
// day, so the quotient steps back by one whenever the remainder would go negative.
func euclidDivMod(n, d int64) (q, r int64) {
	q, r = n/d, n%d
	if r < 0 {
		q--
		r += d
	}
	return q, r
}

// timeParts is a time value taken apart into the calendar components every getter and
// every format reads. The month is one-based here, the way the ISO calendar and the
// Temporal helpers count it; getMonth subtracts one for the zero-based number
// JavaScript reports.
type timeParts struct {
	year    int
	month   int
	day     int
	weekday int
	hour    int
	minute  int
	second  int
	milli   int
}

// splitTimeValue takes a time value apart. It is the one place the calendar arithmetic
// lives: the getters, the ISO format, and the other formats all read the same split, so
// they cannot drift from each other. The caller must have ruled out the Invalid Date,
// since NaN has no components and no integer to convert to.
func splitTimeValue(ms float64) timeParts {
	days, rem := euclidDivMod(int64(ms), 86_400_000)
	year, month, day := epochDaysToISO(int(days))
	// The epoch day itself was a Thursday, so the week counts from there. The extra
	// term keeps the modulus non-negative for a day before the epoch.
	weekday := int(((days%7)+7+4)%7) % 7
	hour := rem / 3_600_000
	rem %= 3_600_000
	minute := rem / 60_000
	rem %= 60_000
	return timeParts{
		year:    year,
		month:   month,
		day:     day,
		weekday: weekday,
		hour:    int(hour),
		minute:  int(minute),
		second:  int(rem / 1_000),
		milli:   int(rem % 1_000),
	}
}

// localOffsetMillis is how far local time runs ahead of UTC at this instant, in
// milliseconds. It is read per instant rather than once, because a zone with daylight
// saving has two different offsets across a year and a date in July must not be read
// with January's offset.
func (d *Date) localOffsetMillis() int64 {
	_, offsetSec := time.UnixMilli(int64(d.ms)).Local().Zone()
	return int64(offsetSec) * 1000
}

// localParts is the date as the local zone sees it. Shifting the time value by the
// offset and then splitting it as though it were UTC gives the local wall clock, which
// is what the local getters report.
func (d *Date) localParts() timeParts {
	return splitTimeValue(d.ms + float64(d.localOffsetMillis()))
}

// GetTimezoneOffset is the difference between local time and UTC in minutes, the
// lowering of date.getTimezoneOffset(). The sign is the specification's, not the one a
// zone name suggests: a zone ahead of UTC reports a negative number, because the value
// is what you add to local time to get UTC.
func (d *Date) GetTimezoneOffset() float64 {
	if math.IsNaN(d.ms) {
		return d.ms
	}
	return float64(-d.localOffsetMillis() / 60_000)
}

// The local calendar getters. Each reports the component the local zone sees, and each
// gives NaN for the Invalid Date, which is what makes every read of an invalid date
// propagate rather than quietly reporting 1970.
func (d *Date) GetFullYear() float64     { return d.local(func(p timeParts) int { return p.year }) }
func (d *Date) GetMonth() float64        { return d.local(func(p timeParts) int { return p.month - 1 }) }
func (d *Date) GetDate() float64         { return d.local(func(p timeParts) int { return p.day }) }
func (d *Date) GetDay() float64          { return d.local(func(p timeParts) int { return p.weekday }) }
func (d *Date) GetHours() float64        { return d.local(func(p timeParts) int { return p.hour }) }
func (d *Date) GetMinutes() float64      { return d.local(func(p timeParts) int { return p.minute }) }
func (d *Date) GetSeconds() float64      { return d.local(func(p timeParts) int { return p.second }) }
func (d *Date) GetMilliseconds() float64 { return d.local(func(p timeParts) int { return p.milli }) }

// The UTC calendar getters. They read the same components without the zone shift, so a
// program that wants a stable answer across machines reaches for these.
func (d *Date) GetUTCFullYear() float64     { return d.utc(func(p timeParts) int { return p.year }) }
func (d *Date) GetUTCMonth() float64        { return d.utc(func(p timeParts) int { return p.month - 1 }) }
func (d *Date) GetUTCDate() float64         { return d.utc(func(p timeParts) int { return p.day }) }
func (d *Date) GetUTCDay() float64          { return d.utc(func(p timeParts) int { return p.weekday }) }
func (d *Date) GetUTCHours() float64        { return d.utc(func(p timeParts) int { return p.hour }) }
func (d *Date) GetUTCMinutes() float64      { return d.utc(func(p timeParts) int { return p.minute }) }
func (d *Date) GetUTCSeconds() float64      { return d.utc(func(p timeParts) int { return p.second }) }
func (d *Date) GetUTCMilliseconds() float64 { return d.utc(func(p timeParts) int { return p.milli }) }

// local reads one component off the local split, giving NaN for the Invalid Date. The
// guard lives here rather than in each of the sixteen getters so that no getter can
// forget it.
func (d *Date) local(pick func(timeParts) int) float64 {
	if math.IsNaN(d.ms) {
		return d.ms
	}
	return float64(pick(d.localParts()))
}

// utc reads one component off the UTC split, with the same Invalid Date guard local has.
func (d *Date) utc(pick func(timeParts) int) float64 {
	if math.IsNaN(d.ms) {
		return d.ms
	}
	return float64(pick(splitTimeValue(d.ms)))
}
