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
	days, rem := euclidDivMod(int64(d.ms), 86_400_000)
	year, month, day := epochDaysToISO(int(days))
	hour := rem / 3_600_000
	rem %= 3_600_000
	minute := rem / 60_000
	rem %= 60_000
	second := rem / 1_000
	milli := rem % 1_000
	s := formatISOYear(year) + "-" + twoDigit(month) + "-" + twoDigit(day) +
		"T" + twoDigit(int(hour)) + ":" + twoDigit(int(minute)) + ":" + twoDigit(int(second)) +
		"." + zeroPad(int(milli), 3) + "Z"
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
