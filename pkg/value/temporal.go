package value

import (
	"math"
	"strconv"
)

// PlainDate is bento's runtime representation of a Temporal.PlainDate (Temporal
// §3): a calendar date with no time and no zone, an ISO year, month, and day. This
// slice hosts only the ISO 8601 calendar, the one every Temporal.PlainDate carries
// unless a caller names another; a non-ISO calendar hands back at lowering, so a
// PlainDate that reached the runtime is always iso8601 and its calendarId reports
// that string.
//
// The three fields are the proleptic Gregorian year, the month in 1..12, and the
// day in 1..(days in that month). They are stored as the integers RejectISODate
// validated, so every derived accessor (the weekday, the day of the year, the leap
// flag) recomputes from them over the ISO calendar rather than caching a second
// copy. The calendar-dependent getters the checker types as number | undefined
// (era, eraYear, weekOfYear, yearOfWeek) hand back at lowering rather than lower to
// a getter here, so this type carries only the fields every ISO date defines.
type PlainDate struct {
	year  int // proleptic Gregorian year, may be negative or above 9999
	month int // 1..12
	day   int // 1..isoDaysInMonth(year, month)
}

// NewPlainDate builds a PlainDate from the constructor's three number arguments,
// running ToIntegerWithTruncation on each and then RejectISODate, so a fractional
// argument truncates toward zero, a non-finite one throws a RangeError, and an
// out-of-range or out-of-limits date throws a RangeError, the same order
// new Temporal.PlainDate(y, m, d) follows in the specification. A fourth calendar
// argument is not accepted here; a non-ISO calendar hands back at lowering, so this
// constructor is only ever reached for the ISO calendar.
func NewPlainDate(isoYear, isoMonth, isoDay float64) *PlainDate {
	y := toIntegerWithTruncation(isoYear)
	m := toIntegerWithTruncation(isoMonth)
	d := toIntegerWithTruncation(isoDay)
	rejectISODate(y, m, d)
	return &PlainDate{year: int(y), month: int(m), day: int(d)}
}

// PlainDateFrom implements Temporal.PlainDate.from for a PlainDate argument: it
// returns a fresh PlainDate with the same fields, the copy the specification makes
// so the result is a distinct object that compares equal to its source. from over a
// string or a property bag hands back at lowering, so this is only reached with a
// PlainDate in hand.
func PlainDateFrom(pd *PlainDate) *PlainDate {
	return &PlainDate{year: pd.year, month: pd.month, day: pd.day}
}

// toIntegerWithTruncation implements the abstract operation of the same name
// (Temporal): NaN becomes zero, a non-finite value throws a RangeError, and any
// other value truncates toward zero to a mathematical integer. It returns a float64
// so the range checks in rejectISODate run before the value is narrowed to an int,
// which keeps a wildly out-of-range year (1e300) from wrapping on the conversion.
func toIntegerWithTruncation(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	if math.IsInf(x, 0) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate requires finite integer components")))
	}
	return math.Trunc(x)
}

// rejectISODate throws a RangeError unless (year, month, day) is a real ISO date
// within Temporal's representable range: the month in 1..12, the day in 1..(days in
// that month), and the whole date between -271821-04-19 and +275760-09-13, the
// bounds ISODateWithinLimits fixes. The arguments are the truncated float64s so the
// year bound is checked before the value is narrowed to an int.
func rejectISODate(year, month, day float64) {
	if month < 1 || month > 12 {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate month must be between 1 and 12")))
	}
	if year < -271821 || year > 275760 {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	y, m, d := int(year), int(month), int(day)
	if d < 1 || d > isoDaysInMonth(y, m) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate day is out of range for the month")))
	}
	if !isoDateWithinLimits(y, m, d) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
}

// isoDateWithinLimits reports whether a date falls in Temporal's representable
// range, -271821-04-19 through +275760-09-13 inclusive. It compares the year, then
// the month, then the day at each end rather than converting to an epoch-day count,
// which is both exact and self-evidently the stated bound.
func isoDateWithinLimits(year, month, day int) bool {
	if year < -271821 || year > 275760 {
		return false
	}
	if year == -271821 {
		if month < 4 || (month == 4 && day < 19) {
			return false
		}
	}
	if year == 275760 {
		if month > 9 || (month == 9 && day > 13) {
			return false
		}
	}
	return true
}

// isLeapISO reports whether year is a leap year in the proleptic Gregorian
// calendar: divisible by four, except centuries that are not divisible by four
// hundred.
func isLeapISO(year int) bool {
	return year%4 == 0 && (year%100 != 0 || year%400 == 0)
}

// isoDaysInMonth returns the number of days in month (1..12) of year, honoring the
// leap-year length of February.
func isoDaysInMonth(year, month int) int {
	switch month {
	case 1, 3, 5, 7, 8, 10, 12:
		return 31
	case 4, 6, 9, 11:
		return 30
	default: // February
		if isLeapISO(year) {
			return 29
		}
		return 28
	}
}

// isoToEpochDays returns the number of days from the Unix epoch (1970-01-01) to the
// given ISO date, negative before the epoch. It is Howard Hinnant's days_from_civil
// algorithm, exact for the whole proleptic Gregorian range.
func isoToEpochDays(year, month, day int) int {
	y := year
	if month <= 2 {
		y--
	}
	era := y
	if y < 0 {
		era = y - 399
	}
	era /= 400
	yoe := y - era*400
	mp := (month + 9) % 12
	doy := (153*mp+2)/5 + day - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe - 719468
}

// Year returns the ISO year.
func (pd *PlainDate) Year() float64 { return float64(pd.year) }

// Month returns the ISO month, 1..12.
func (pd *PlainDate) Month() float64 { return float64(pd.month) }

// Day returns the ISO day of the month.
func (pd *PlainDate) Day() float64 { return float64(pd.day) }

// CalendarId returns the calendar identifier, always "iso8601" for this slice.
func (pd *PlainDate) CalendarId() BStr { return FromGoString("iso8601") }

// MonthCode returns the ISO month code, "M" followed by the two-digit month. The
// ISO calendar has no leap months, so the code never carries the trailing "L".
func (pd *PlainDate) MonthCode() BStr {
	code := "M"
	if pd.month < 10 {
		code += "0"
	}
	return FromGoString(code + strconv.Itoa(pd.month))
}

// DayOfWeek returns the ISO day of the week, Monday=1 through Sunday=7. The epoch
// day 1970-01-01 is a Thursday (ISO 4), which fixes the offset.
func (pd *PlainDate) DayOfWeek() float64 {
	e := isoToEpochDays(pd.year, pd.month, pd.day)
	return float64((((e+3)%7)+7)%7 + 1)
}

// DayOfYear returns the 1-based ordinal day within the year.
func (pd *PlainDate) DayOfYear() float64 {
	return float64(isoToEpochDays(pd.year, pd.month, pd.day) - isoToEpochDays(pd.year, 1, 1) + 1)
}

// DaysInWeek is always 7 in the ISO calendar.
func (pd *PlainDate) DaysInWeek() float64 { return 7 }

// DaysInMonth returns the number of days in this date's month.
func (pd *PlainDate) DaysInMonth() float64 { return float64(isoDaysInMonth(pd.year, pd.month)) }

// DaysInYear returns 366 in a leap year and 365 otherwise.
func (pd *PlainDate) DaysInYear() float64 {
	if isLeapISO(pd.year) {
		return 366
	}
	return 365
}

// MonthsInYear is always 12 in the ISO calendar.
func (pd *PlainDate) MonthsInYear() float64 { return 12 }

// InLeapYear reports whether this date's year is an ISO leap year.
func (pd *PlainDate) InLeapYear() bool { return isLeapISO(pd.year) }

// Equals implements Temporal.PlainDate.prototype.equals: two dates are equal when
// their year, month, and day match under the same (ISO) calendar.
func (pd *PlainDate) Equals(other *PlainDate) bool {
	return pd.year == other.year && pd.month == other.month && pd.day == other.day
}

// PlainDateCompare implements Temporal.PlainDate.compare, the static comparator:
// -1 if a precedes b, 1 if a follows b, 0 if they fall on the same day.
func PlainDateCompare(a, b *PlainDate) float64 {
	switch {
	case a.year != b.year:
		if a.year < b.year {
			return -1
		}
		return 1
	case a.month != b.month:
		if a.month < b.month {
			return -1
		}
		return 1
	case a.day != b.day:
		if a.day < b.day {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// ToString implements Temporal.PlainDate.prototype.toString for the default
// options: the ISO 8601 date, YYYY-MM-DD, with the year expanded to a signed
// six-digit form outside 0..9999.
func (pd *PlainDate) ToString() BStr {
	return FromGoString(formatISOYear(pd.year) + "-" + twoDigit(pd.month) + "-" + twoDigit(pd.day))
}

// ToJSON implements Temporal.PlainDate.prototype.toJSON, the same ISO string
// toString produces under default options.
func (pd *PlainDate) ToJSON() BStr { return pd.ToString() }

// formatISOYear renders the ISO year: four digits for 0..9999, otherwise a leading
// sign and six digits, the expanded-year form ISO 8601 uses beyond the plain range.
func formatISOYear(year int) string {
	if year >= 0 && year <= 9999 {
		return zeroPad(year, 4)
	}
	sign := "+"
	if year < 0 {
		sign = "-"
		year = -year
	}
	return sign + zeroPad(year, 6)
}

// twoDigit renders a month or day as exactly two digits.
func twoDigit(n int) string { return zeroPad(n, 2) }

// zeroPad renders n as a decimal left-padded with zeros to at least width digits.
func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}
