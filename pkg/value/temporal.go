package value

import (
	"math"
	"math/big"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// PlainDate is bento's runtime representation of a Temporal.PlainDate (Temporal
// §3): a calendar date with no time and no zone, held as an ISO year, month, and day
// paired with a calendar that interprets them. Every PlainDate stores its date over
// the proleptic Gregorian (ISO 8601) calendar the way Temporal does internally; the
// cal field names the calendar the getters report under, "" reading as iso8601. This
// slice hosts the ISO 8601 calendar and the proleptic Gregorian calendar, whose date
// arithmetic is the ISO one with an era; a calendar bento does not host yet hands
// back at lowering, so cal is always "" or "gregory".
//
// The three date fields are the proleptic Gregorian year, the month in 1..12, and the
// day in 1..(days in that month). They are stored as the integers RejectISODate
// validated, so every derived accessor (the weekday, the day of the year, the leap
// flag) recomputes from them rather than caching a second copy. The calendar-dependent
// getters the checker types as an optional read as the value the calendar gives them:
// era and eraYear are undefined under ISO but read the gregory era under gregory, and
// weekOfYear and yearOfWeek are the ISO 8601 week date computed from the ordinal day
// and the weekday, which the gregory calendar shares.
type PlainDate struct {
	year  int    // proleptic Gregorian year, may be negative or above 9999
	month int    // 1..12
	day   int    // 1..isoDaysInMonth(year, month)
	cal   string // canonical calendar id, "" reads as iso8601
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

// canonicalCalendar canonicalizes a Temporal calendar identifier the way
// CanonicalizeCalendar does: identifiers are case-insensitive, so it lowercases the
// id and returns the canonical form, with ok=false for an id bento does not host.
// This slice hosts the ISO 8601 calendar and the proleptic Gregorian calendar; the
// lowerer only ever routes one of these two here, but the check is kept so a stray id
// throws the RangeError the specification requires rather than silently mislabelling.
func canonicalCalendar(id string) (string, bool) {
	switch strings.ToLower(id) {
	case "iso8601":
		return "iso8601", true
	case "gregory":
		return "gregory", true
	case "roc":
		return "roc", true
	case "japanese":
		return "japanese", true
	default:
		return "", false
	}
}

// NewPlainDateCal builds a PlainDate under a named calendar, the four-argument
// constructor new Temporal.PlainDate(y, m, d, calendar). It follows the specification
// order: ToIntegerWithTruncation on each component first, so a non-finite one throws a
// RangeError, then CanonicalizeCalendar, so an unhosted or invalid id throws a
// RangeError, then RejectISODate, so an out-of-range date throws. The date fields are
// the ISO date the components spell; the calendar only changes how the getters label
// them.
func NewPlainDateCal(isoYear, isoMonth, isoDay float64, calendar string) *PlainDate {
	y := toIntegerWithTruncation(isoYear)
	m := toIntegerWithTruncation(isoMonth)
	d := toIntegerWithTruncation(isoDay)
	cal, ok := canonicalCalendar(calendar)
	if !ok {
		Throw(NewRangeError(FromGoString("invalid calendar identifier " + calendar)))
	}
	rejectISODate(y, m, d)
	return &PlainDate{year: int(y), month: int(m), day: int(d), cal: cal}
}

// PlainDateWithCalendar implements Temporal.PlainDate.prototype.withCalendar: it
// reinterprets the same ISO date under another calendar, returning a fresh PlainDate
// with the given calendar id. The id is canonicalized and validated, so an unhosted or
// invalid one throws a RangeError.
func PlainDateWithCalendar(pd *PlainDate, calendar string) *PlainDate {
	cal, ok := canonicalCalendar(calendar)
	if !ok {
		Throw(NewRangeError(FromGoString("invalid calendar identifier " + calendar)))
	}
	return &PlainDate{year: pd.year, month: pd.month, day: pd.day, cal: cal}
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
// (Temporal): a NaN or non-finite value throws a RangeError, and any other value
// truncates toward zero to a mathematical integer. It returns a float64 so the range
// checks in rejectISODate run before the value is narrowed to an int, which keeps a
// wildly out-of-range year (1e300) from wrapping on the conversion. NaN throwing here
// matters: new Temporal.PlainDate(NaN, 1, 1) must raise a RangeError, not settle on
// year zero, since 0000-01-01 is itself a valid date.
func toIntegerWithTruncation(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		Throw(NewRangeError(FromGoString("Temporal component must be a finite integer")))
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

// floorDiv returns the floor of a divided by b for a positive divisor b, which Go's
// truncating division does not give for a negative dividend. It is used to carry a month
// count into a year, where the intermediate month can be negative.
func floorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// addISODate applies the specification's AddISODate: it adds years, months, weeks, and days
// to an ISO date under the overflow rule and returns the balanced date. Years and months carry
// into the year through BalanceISOYearMonth, then the original day is regulated against the new
// month, clamped to the month end under constrain or throwing a RangeError under reject when it
// does not fit. Weeks fold into days at seven each, and the days balance through the epoch-day
// count so a day past the month end rolls into the following months. The days argument is a
// big.Int because the caller has already folded the duration's time part into a whole-day carry
// that can exceed an int.
func addISODate(year, month, day, years, months, weeks int, days *big.Int, overflow string) (int, int, int) {
	inMonth := month + months
	y := year + years + floorDiv(inMonth-1, 12)
	m := (inMonth - 1) % 12
	if m < 0 {
		m += 12
	}
	m++
	d := day
	if dim := isoDaysInMonth(y, m); d > dim {
		if overflow == timeOverflowReject {
			Throw(NewRangeError(FromGoString("Temporal.PlainDate day is out of range for the month")))
		}
		d = dim
	}
	e := big.NewInt(int64(isoToEpochDays(y, m, d)))
	e.Add(e, big.NewInt(int64(weeks)*7))
	e.Add(e, days)
	if !e.IsInt64() {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return epochDaysToISO(int(e.Int64()))
}

// AddDate implements Temporal.PlainDate.prototype.add and, over a negated Duration, subtract.
// A PlainDate has no clock, so the duration's time components fold into a whole-day carry
// truncated toward zero and the sub-day remainder is dropped; that carry joins the duration's
// days, and the years, months, weeks, and days add through addISODate under the overflow rule.
// The result keeps the receiver's calendar, whose year and era re-derive from the moved ISO
// date, and an out-of-range result throws a RangeError.
func (pd *PlainDate) AddDate(dur *Duration, overflow string) *PlainDate {
	days := new(big.Int).Quo(durationTimeNanos(dur), nsPerDay)
	days.Add(days, big.NewInt(int64(dur.days)))
	y, m, d := addISODate(pd.year, pd.month, pd.day, int(dur.years), int(dur.months), int(dur.weeks), days, overflow)
	if !isoDateWithinLimits(y, m, d) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return &PlainDate{year: y, month: m, day: d, cal: pd.cal}
}

// isoDateCompare returns -1, 0, or 1 comparing two ISO dates by year, then month, then day.
func isoDateCompare(y1, m1, d1, y2, m2, d2 int) int {
	switch {
	case y1 != y2:
		if y1 < y2 {
			return -1
		}
		return 1
	case m1 != m2:
		if m1 < m2 {
			return -1
		}
		return 1
	case d1 != d2:
		if d1 < d2 {
			return -1
		}
		return 1
	default:
		return 0
	}
}

// balanceISOYearMonth normalizes a one-based month that may fall outside 1..12 into a year
// and month, carrying whole years with floored division so a zero or negative month borrows.
func balanceISOYearMonth(year, month int) (int, int) {
	y := year + floorDiv(month-1, 12)
	m := (month-1)%12 + 1
	if m < 1 {
		m += 12
	}
	return y, m
}

// isoDateSurpasses reports whether the first date has passed the second in the direction of
// sign. The comparison uses the raw day of month, unclamped, so a February 31 counts as past
// February 29; that is what keeps a month step that would only survive by clamping from being
// counted, so January 31 to February 29 settles as days rather than a whole month.
func isoDateSurpasses(sign, y1, m1, d1, y2, m2, d2 int) bool {
	return sign*isoDateCompare(y1, m1, d1, y2, m2, d2) == 1
}

// differenceISODate implements the specification's DifferenceISODate: the calendar distance
// from the first date to the second, balanced from largestUnit down to days. For a day or
// week largestUnit the gap is a whole count of epoch days, split into weeks under "week". For
// a month or year largestUnit it steps whole years, then whole months, each step testing the
// raw day of month against the target so a step that would only fit by clamping is not taken,
// then settles the remainder in days from the constrained intermediate to the target.
func differenceISODate(y1, m1, d1, y2, m2, d2 int, largestUnit string) (years, months, weeks, days int) {
	switch largestUnit {
	case "year", "month":
		sign := -isoDateCompare(y1, m1, d1, y2, m2, d2)
		if sign == 0 {
			return 0, 0, 0, 0
		}
		if largestUnit == "year" {
			for candidate := sign; !isoDateSurpasses(sign, y1+candidate, m1, d1, y2, m2, d2); candidate += sign {
				years = candidate
			}
		}
		iy, im := balanceISOYearMonth(y1+years, m1+sign)
		for candidate := sign; !isoDateSurpasses(sign, iy, im, d1, y2, m2, d2); candidate += sign {
			months = candidate
			iy, im = balanceISOYearMonth(iy, im+sign)
		}
		if largestUnit == "month" {
			months += years * 12
			years = 0
		}
		my, mm, md := addISODate(y1, m1, d1, years, months, 0, big.NewInt(0), "constrain")
		days = isoToEpochDays(y2, m2, d2) - isoToEpochDays(my, mm, md)
		return years, months, 0, days
	default:
		sy, sm, sd, ly, lm, ld, sign := y1, m1, d1, y2, m2, d2, 1
		if isoDateCompare(y1, m1, d1, y2, m2, d2) > 0 {
			sy, sm, sd, ly, lm, ld, sign = y2, m2, d2, y1, m1, d1, -1
		}
		days = isoToEpochDays(ly, lm, ld) - isoToEpochDays(sy, sm, sd)
		if largestUnit == "week" {
			weeks = (days / 7) * sign
			days = days % 7
		}
		return 0, 0, weeks, days * sign
	}
}

// plainDateDifference builds the Duration from the receiver to other, balanced at largestUnit.
// until and since share it: a.until(b) is this to other, a.since(b) is its negation, so Since
// negates the same walk rather than swapping the operands, which keeps the month anchoring on
// the receiver as the specification requires. The two dates must share a calendar.
func plainDateDifference(from, to *PlainDate, largestUnit string) *Duration {
	if from.calendarID() != to.calendarID() {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate difference between two calendars is not allowed")))
	}
	years, months, weeks, days := differenceISODate(from.year, from.month, from.day, to.year, to.month, to.day, largestUnit)
	return NewDuration(float64(years), float64(months), float64(weeks), float64(days), 0, 0, 0, 0, 0, 0)
}

// Until returns the calendar difference from the receiver to other as a Duration, balanced
// from largestUnit down to days.
func (pd *PlainDate) Until(other *PlainDate, largestUnit string) *Duration {
	return plainDateDifference(pd, other, largestUnit)
}

// Since returns the calendar difference from other to the receiver, the negation of Until, so
// the month anchoring stays on the receiver.
func (pd *PlainDate) Since(other *PlainDate, largestUnit string) *Duration {
	return plainDateDifference(pd, other, largestUnit).Negated()
}

// WithFields implements Temporal.PlainDate.prototype.with: it lays the bag's present year,
// month, and day over the receiver's own fields and regulates the result with the overflow
// option, so an omitted field keeps its current value. The year is read in the receiver's
// calendar reckoning, so under roc a bag year maps back to the ISO year the date stores by
// adding 1911; the other hosted calendars count the ISO year directly. Under constrain the
// month clamps to 1..12 and the day to that month's length, so with month 2 over January 31
// lands on the last day of February; under reject an out-of-range field throws a RangeError.
// monthCode and the era fields are not read here, the lowerer hands back a bag that carries
// them. The receiver is unchanged.
func (pd *PlainDate) WithFields(year, month, day Opt[float64], overflow string) *PlainDate {
	calYear := toIntegerWithTruncation(year.Or(float64(pd.displayYear())))
	m := toIntegerWithTruncation(month.Or(float64(pd.month)))
	d := toIntegerWithTruncation(day.Or(float64(pd.day)))
	isoYear := calYear
	if pd.cal == "roc" {
		isoYear = calYear + 1911
	}
	if overflow == timeOverflowReject {
		rejectISODate(isoYear, m, d)
	} else {
		m = clampFloat(m, 1, 12)
		if isoYear < -271821 || isoYear > 275760 {
			Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
		}
		d = clampFloat(d, 1, float64(isoDaysInMonth(int(isoYear), int(m))))
	}
	if !isoDateWithinLimits(int(isoYear), int(m), int(d)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return &PlainDate{year: int(isoYear), month: int(m), day: int(d), cal: pd.cal}
}

// PlainDateFromFields implements Temporal.PlainDate.from over a property bag: it builds a
// PlainDate from the required year, month, and day fields under the given calendar, regulating
// the result with the overflow option. The year is read in the calendar's own reckoning, so a
// roc bag year maps back to the ISO year by adding 1911 while the other hosted calendars count
// the ISO year directly. Under constrain the month clamps to 1..12 and the day to that month's
// length; under reject an out-of-range field throws a RangeError. The calendar is canonicalized
// and validated, so an unhosted id throws, though the lowerer only routes a hosted one here.
func PlainDateFromFields(year, month, day float64, calendar, overflow string) *PlainDate {
	cal, ok := canonicalCalendar(calendar)
	if !ok {
		Throw(NewRangeError(FromGoString("invalid calendar identifier " + calendar)))
	}
	calYear := toIntegerWithTruncation(year)
	m := toIntegerWithTruncation(month)
	d := toIntegerWithTruncation(day)
	isoYear := calYear
	if cal == "roc" {
		isoYear = calYear + 1911
	}
	if overflow == timeOverflowReject {
		rejectISODate(isoYear, m, d)
	} else {
		m = clampFloat(m, 1, 12)
		if isoYear < -271821 || isoYear > 275760 {
			Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
		}
		d = clampFloat(d, 1, float64(isoDaysInMonth(int(isoYear), int(m))))
	}
	if !isoDateWithinLimits(int(isoYear), int(m), int(d)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return &PlainDate{year: int(isoYear), month: int(m), day: int(d), cal: cal}
}

// PlainDateTimeFromFields implements Temporal.PlainDateTime.from over a property bag: it builds a
// PlainDateTime from the required year, month, and day fields plus the optional time fields under
// the given calendar, regulating each half with the overflow option. The date half reuses
// PlainDateFromFields, so the year is read in the calendar's own reckoning and the day clamps to
// the resulting month under constrain; the time half lays the present fields over an all-zero base
// so an omitted time field defaults to the zero midnight carries, then clamps to its ISO maxima.
// Under reject an out-of-range field in either half throws a RangeError. The result keeps the
// calendar, so a roc bag stays under roc.
func PlainDateTimeFromFields(year, month, day float64, hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], calendar, overflow string) *PlainDateTime {
	date := PlainDateFromFields(year, month, day, calendar, overflow)
	time := regulatePlainTime([6]float64{}, [6]Opt[float64]{hour, minute, second, millisecond, microsecond, nanosecond}, overflow)
	return &PlainDateTime{date: *date, time: *time}
}

// ToPlainDateTime implements Temporal.PlainDate.prototype.toPlainDateTime: it pairs the date
// with a wall-clock time to make a PlainDateTime, defaulting to midnight when no time is
// given. The result keeps this date's calendar, so a non-ISO date stays under its calendar.
// The receiver is copied, so the new PlainDateTime shares no state with it.
func (pd *PlainDate) ToPlainDateTime(time *PlainTime) *PlainDateTime {
	t := PlainTime{}
	if time != nil {
		t = *time
	}
	return &PlainDateTime{date: *pd, time: t}
}

// ToPlainYearMonth implements Temporal.PlainDate.prototype.toPlainYearMonth: it narrows the
// date to its year and month, dropping the day, under the date's own calendar. The result
// keeps that calendar, so its year getter and toString read in the calendar's reckoning and
// a non-ISO year-month carries the reference day the first of the month.
func (pd *PlainDate) ToPlainYearMonth() *PlainYearMonth {
	return &PlainYearMonth{year: pd.year, month: pd.month, cal: pd.cal}
}

// ToPlainMonthDay implements Temporal.PlainDate.prototype.toPlainMonthDay: it narrows the date
// to its month and day, dropping the year, under the date's own calendar. The result keeps
// that calendar, so a non-ISO month-day carries the leap reference year 1972 and the annotation
// in its toString. The four hosted calendars share the ISO month structure, so the month and
// day pass through unchanged.
func (pd *PlainDate) ToPlainMonthDay() *PlainMonthDay {
	return &PlainMonthDay{month: pd.month, day: pd.day, cal: pd.cal}
}

// ToZonedDateTime implements Temporal.PlainDate.prototype.toZonedDateTime: it pins the date, at
// a wall-clock time defaulting to midnight, to a time zone, resolving the exact instant under
// the default compatible disambiguation, which takes the earlier reading in a fall-back overlap
// and shifts forward across a spring-forward gap. The result keeps this date's calendar, so a
// non-ISO date stays under its calendar.
func (pd *PlainDate) ToZonedDateTime(timeZone string, plainTime *PlainTime) *ZonedDateTime {
	t := PlainTime{}
	if plainTime != nil {
		t = *plainTime
	}
	loc, canon := resolveTimeZone(timeZone)
	wall := wallNanoseconds(isoParse{
		year: pd.year, month: pd.month, day: pd.day,
		hour: t.hour, minute: t.minute, second: t.second,
		millisecond: t.millisecond, microsecond: t.microsecond, nanosecond: t.nanosecond,
	})
	epoch := disambiguateCompatible(loc, wall)
	validateEpochNanoseconds(epoch)
	return &ZonedDateTime{ns: epoch, loc: loc, tzID: FromGoString(canon), cal: pd.cal}
}

// displayYear returns the year the calendar counts, which the year getter reports and
// the gregory-style era split turns on. It matches the ISO year for iso8601, gregory,
// and japanese; roc, the Minguo calendar, counts from 1912, so its year is the ISO year
// minus 1911.
func (pd *PlainDate) displayYear() int {
	if pd.cal == "roc" {
		return pd.year - 1911
	}
	return pd.year
}

// calendarEraBase returns the era base name for a calendar that has an era, and false
// for one that does not. iso8601 has none. gregory and roc each carry a two-part era
// that splits at their own year 1, the base name below year 1 and the base name with a
// "-inverse" suffix at year 1 and above.
func calendarEraBase(cal string) (string, bool) {
	switch cal {
	case "gregory":
		return "gregory", true
	case "roc":
		return "roc", true
	default:
		return "", false
	}
}

// Year returns the year the calendar counts: the ISO year under iso8601 and gregory,
// and the ISO year minus 1911 under roc.
func (pd *PlainDate) Year() float64 { return float64(pd.displayYear()) }

// Month returns the ISO month, 1..12.
func (pd *PlainDate) Month() float64 { return float64(pd.month) }

// Day returns the ISO day of the month.
func (pd *PlainDate) Day() float64 { return float64(pd.day) }

// calendarID returns the canonical calendar id this date reports under, mapping the
// empty stored value to "iso8601" so a date built without a calendar reads as ISO.
func (pd *PlainDate) calendarID() string {
	if pd.cal == "" {
		return "iso8601"
	}
	return pd.cal
}

// CalendarId returns the calendar identifier, "iso8601", "gregory", "roc", or "japanese".
func (pd *PlainDate) CalendarId() BStr { return FromGoString(pd.calendarID()) }

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

// japaneseEra returns the Japanese era name and the year within that era for an ISO
// date. The modern nengo each begin on a fixed proleptic-Gregorian day, so the era turns
// on the whole date, not just the year: 1989-01-07 is the last day of showa and
// 1989-01-08 the first of heisei. Each era numbers its first Gregorian year as year 1,
// so its era year is the ISO year minus the era's base year plus one. Before Meiji begins
// on 1868-09-08 the calendar has no nengo, so it falls back to a "japanese" era that
// mirrors the ISO year and splits at year 1 into "japanese" and "japanese-inverse" the
// way gregory splits its own timeline.
func japaneseEra(year, month, day int) (string, int) {
	eras := []struct {
		y, m, d int
		name    string
		base    int
	}{
		{2019, 5, 1, "reiwa", 2019},
		{1989, 1, 8, "heisei", 1989},
		{1926, 12, 25, "showa", 1926},
		{1912, 7, 30, "taisho", 1912},
		{1868, 9, 8, "meiji", 1868},
	}
	for _, e := range eras {
		if year > e.y || (year == e.y && (month > e.m || (month == e.m && day >= e.d))) {
			return e.name, year - e.base + 1
		}
	}
	if year >= 1 {
		return "japanese", year
	}
	return "japanese-inverse", 1 - year
}

// Era implements Temporal.PlainDate.prototype.era. The ISO 8601 calendar has no era,
// so the getter the checker types string | undefined is undefined under ISO. gregory and
// roc split their timeline at their own year 1: a display year of 1 or above is the base
// era and 0 or below the "-inverse" era, gregory at ISO year 1 and roc at ISO year 1912.
// japanese resolves its era from the whole date against the nengo table.
func (pd *PlainDate) Era() Opt[BStr] {
	if pd.cal == "japanese" {
		name, _ := japaneseEra(pd.year, pd.month, pd.day)
		return Some(FromGoString(name))
	}
	base, ok := calendarEraBase(pd.cal)
	if !ok {
		return None[BStr]()
	}
	if pd.displayYear() >= 1 {
		return Some(FromGoString(base))
	}
	return Some(FromGoString(base + "-inverse"))
}

// EraYear implements Temporal.PlainDate.prototype.eraYear, the year counted within the
// era. It is undefined under ISO; under gregory or roc it is the display year itself in
// the base era and 1 minus the display year in the "-inverse" era, so under gregory ISO
// year 0 is eraYear 1 and under roc ISO year 1911 (roc year 0) is eraYear 1 in the
// "roc-inverse" era. japanese counts within the nengo the date falls in.
func (pd *PlainDate) EraYear() Opt[float64] {
	if pd.cal == "japanese" {
		_, eraYear := japaneseEra(pd.year, pd.month, pd.day)
		return Some(float64(eraYear))
	}
	if _, ok := calendarEraBase(pd.cal); !ok {
		return None[float64]()
	}
	y := pd.displayYear()
	if y >= 1 {
		return Some(float64(y))
	}
	return Some(float64(1 - y))
}

// WeekOfYear implements Temporal.PlainDate.prototype.weekOfYear, the ISO 8601 week
// number 1..53. The ISO calendar always defines it, so the optional the checker
// types number | undefined is always present; a calendar without weeks would read
// undefined, which is why the field is optional at all.
func (pd *PlainDate) WeekOfYear() Opt[float64] {
	week, _ := isoWeekOfYear(pd.year, int(pd.DayOfYear()), int(pd.DayOfWeek()))
	return Some(float64(week))
}

// YearOfWeek implements Temporal.PlainDate.prototype.yearOfWeek, the ISO 8601
// week-numbering year that pairs with weekOfYear. It differs from the calendar year
// at a January or December boundary, where a week belongs to the neighbouring year.
func (pd *PlainDate) YearOfWeek() Opt[float64] {
	_, weekYear := isoWeekOfYear(pd.year, int(pd.DayOfYear()), int(pd.DayOfWeek()))
	return Some(float64(weekYear))
}

// isoWeekOfYear computes the ISO 8601 week number and its week-numbering year from a
// date's year, ordinal day, and weekday (Monday=1), the ISOWeekOfYear abstract
// operation. The naive week counts Thursdays from the year's start; it is corrected
// at the two boundaries, where an early-January date can fall in the last week of
// the previous year and a late-December date in the first week of the next.
func isoWeekOfYear(year, dayOfYear, dayOfWeek int) (week int, weekYear int) {
	// dayOfYear is at least 1 and dayOfWeek at most 7, so the numerator is always
	// positive and Go's truncating division agrees with the floor the operation wants.
	week = (dayOfYear - dayOfWeek + 10) / 7
	if week < 1 {
		// A day before the year's first Thursday belongs to the last week of the
		// previous year, which is week 53 when the previous year ends on a Thursday
		// (its Jan 1 is a Friday) or on a Friday of a leap year, and week 52 otherwise.
		jan1 := isoDayOfWeek(year, 1, 1)
		if jan1 == 5 || (jan1 == 6 && isLeapISO(year-1)) {
			return 53, year - 1
		}
		return 52, year - 1
	}
	if week == 53 {
		// Week 53 exists only when the year's last Thursday falls on or after this day;
		// otherwise the day is already in week 1 of the next year.
		daysInYear := 365
		if isLeapISO(year) {
			daysInYear = 366
		}
		if daysInYear-dayOfYear < 4-dayOfWeek {
			return 1, year + 1
		}
	}
	return week, year
}

// isoDayOfWeek returns the ISO weekday, Monday=1 through Sunday=7, for a date given
// by its components, the same computation PlainDate.DayOfWeek runs on its own fields
// and the form isoWeekOfYear needs for a year's January 1.
func isoDayOfWeek(year, month, day int) int {
	e := isoToEpochDays(year, month, day)
	return (((e+3)%7)+7)%7 + 1
}

// Equals implements Temporal.PlainDate.prototype.equals: two dates are equal when
// their year, month, and day match and they carry the same calendar, so the same ISO
// day under iso8601 and under gregory does not compare equal.
func (pd *PlainDate) Equals(other *PlainDate) bool {
	return pd.year == other.year && pd.month == other.month && pd.day == other.day &&
		pd.calendarID() == other.calendarID()
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

// dateCore renders the ISO 8601 date, YYYY-MM-DD, with the year expanded to a signed
// six-digit form outside 0..9999, and no calendar annotation. It is the piece
// PlainDateTime joins with the time across a "T" before the annotation trails the whole
// string.
func (pd *PlainDate) dateCore() string {
	return formatISOYear(pd.year) + "-" + twoDigit(pd.month) + "-" + twoDigit(pd.day)
}

// calendarAnnotation returns the RFC 9557 calendar suffix a non-ISO calendar appends to
// a toString, "[u-ca=<id>]", or "" for the ISO calendar, which prints no annotation.
func (pd *PlainDate) calendarAnnotation() string {
	if pd.cal == "" || pd.cal == "iso8601" {
		return ""
	}
	return "[u-ca=" + pd.cal + "]"
}

// isoString renders the ISO 8601 date with its calendar annotation, the string
// PlainDate.toString wraps.
func (pd *PlainDate) isoString() string {
	return pd.dateCore() + pd.calendarAnnotation()
}

// ToString implements Temporal.PlainDate.prototype.toString for the default
// options: the ISO 8601 date, YYYY-MM-DD, with the year expanded to a signed
// six-digit form outside 0..9999.
func (pd *PlainDate) ToString() BStr {
	return FromGoString(pd.isoString())
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

// PlainTime is bento's runtime representation of a Temporal.PlainTime (Temporal §4):
// a wall-clock time with no date and no zone, the hour, the minute, the second, and
// the three sub-second fields. It carries no calendar and no zone, so unlike PlainDate
// it needs no calendar model at all. The six fields are stored as the integers
// RejectTime validated, so every accessor reads a field directly and toString
// recomputes the fractional-second rendering from the sub-second three.
type PlainTime struct {
	hour        int // 0..23
	minute      int // 0..59
	second      int // 0..59
	millisecond int // 0..999
	microsecond int // 0..999
	nanosecond  int // 0..999
}

// NewPlainTime builds a PlainTime from the constructor's up to six number arguments,
// running ToIntegerWithTruncation on each and then RejectTime, so a fractional
// argument truncates toward zero, a NaN or non-finite one throws a RangeError, and an
// out-of-range field throws a RangeError, the order new Temporal.PlainTime(...) follows
// in the specification. Every argument defaults to zero; the lowerer pads the missing
// trailing components before the call, so this constructor always sees six numbers.
func NewPlainTime(hour, minute, second, millisecond, microsecond, nanosecond float64) *PlainTime {
	h := toIntegerWithTruncation(hour)
	m := toIntegerWithTruncation(minute)
	s := toIntegerWithTruncation(second)
	ms := toIntegerWithTruncation(millisecond)
	us := toIntegerWithTruncation(microsecond)
	ns := toIntegerWithTruncation(nanosecond)
	rejectTime(h, m, s, ms, us, ns)
	return &PlainTime{int(h), int(m), int(s), int(ms), int(us), int(ns)}
}

// PlainTimeFrom implements Temporal.PlainTime.from for a PlainTime argument: it returns
// a fresh PlainTime with the same fields, the copy the specification makes so the result
// is a distinct object that compares equal to its source. from over a string or a
// property bag hands back at lowering, so this is only reached with a PlainTime in hand.
func PlainTimeFrom(pt *PlainTime) *PlainTime {
	return &PlainTime{pt.hour, pt.minute, pt.second, pt.millisecond, pt.microsecond, pt.nanosecond}
}

// timeOverflowReject is the overflow option value that makes an out-of-range field
// throw rather than clamp. The other value, "constrain", is the default and the else
// branch, so only the reject spelling needs a name. The lowerer resolves the option
// bag at compile time and passes the string through.
const timeOverflowReject = "reject"

// regulatePlainTime builds a PlainTime from six optional fields laid over six base
// values. A present field truncates toward zero through ToIntegerWithTruncation; an
// absent one keeps its base, which is zero for from over a bag and the receiver's own
// field for with, so the one helper serves both. Running the truncation over a base is
// a no-op, since a base is already an in-range integer. Under constrain each field then
// clamps to its ISO range; under reject an out-of-range field throws a RangeError,
// matching Temporal's RegulateTime.
func regulatePlainTime(base [6]float64, fields [6]Opt[float64], overflow string) *PlainTime {
	maxima := [6]float64{23, 59, 59, 999, 999, 999}
	var v [6]float64
	for i := range fields {
		v[i] = toIntegerWithTruncation(fields[i].Or(base[i]))
	}
	if overflow == timeOverflowReject {
		rejectTime(v[0], v[1], v[2], v[3], v[4], v[5])
	} else {
		for i := range v {
			v[i] = clampFloat(v[i], 0, maxima[i])
		}
	}
	return &PlainTime{int(v[0]), int(v[1]), int(v[2]), int(v[3]), int(v[4]), int(v[5])}
}

// clampFloat returns x confined to the closed range [lo, hi].
func clampFloat(x, lo, hi float64) float64 {
	if x < lo {
		return lo
	}
	if x > hi {
		return hi
	}
	return x
}

// PlainTimeFromFields implements Temporal.PlainTime.from over a property bag: it lays
// the bag's present fields over an all-zero base, so an omitted field defaults to the
// zero midnight carries, and regulates with the overflow option. The lowerer reads the
// bag at compile time, requires at least one time field, and passes each as a present
// or absent optional.
func PlainTimeFromFields(hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], overflow string) *PlainTime {
	return regulatePlainTime([6]float64{}, [6]Opt[float64]{hour, minute, second, millisecond, microsecond, nanosecond}, overflow)
}

// With implements Temporal.PlainTime.prototype.with: it lays the bag's present fields
// over the receiver's current fields, so an omitted field keeps its existing value, and
// regulates with the overflow option. The result is a fresh PlainTime and the receiver
// is unchanged.
func (pt *PlainTime) With(hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], overflow string) *PlainTime {
	base := [6]float64{float64(pt.hour), float64(pt.minute), float64(pt.second), float64(pt.millisecond), float64(pt.microsecond), float64(pt.nanosecond)}
	return regulatePlainTime(base, [6]Opt[float64]{hour, minute, second, millisecond, microsecond, nanosecond}, overflow)
}

// AddDuration implements Temporal.PlainTime.prototype.add: it folds the duration's time
// units into the receiver's wall clock. Only the six time units count, since days, weeks,
// months, and years do not move the time of day, and the total wraps mod 24 hours, so
// adding past midnight lands on the following day's clock and a net-negative offset lands
// on the previous day's. The receiver is unchanged. subtract is add over a negated
// duration, so no separate SubtractDuration is needed. The fold runs in big.Int because an
// hour field can be up to 2^53 and hours-to-nanoseconds overflows int64.
func (pt *PlainTime) AddDuration(dur *Duration) *PlainTime {
	total := pt.dayNanos()
	total.Add(total, durationTimeNanos(dur))
	total.Mod(total, nsPerDay)
	return plainTimeFromDayNanos(total)
}

// durationTimeNanos folds a Duration's six time components, hours through nanoseconds, into a
// single nanosecond count in big.Int. The calendar components (years, months, weeks, days) are
// ignored; a caller that needs the whole-day carry the time part contributes divides this by
// nsPerDay. The wall-clock arithmetic and the date arithmetic both fold the time part this way.
func durationTimeNanos(dur *Duration) *big.Int {
	total := new(big.Int)
	total.Add(total, bigMulInt(dur.hours, 3_600_000_000_000))
	total.Add(total, bigMulInt(dur.minutes, 60_000_000_000))
	total.Add(total, bigMulInt(dur.seconds, 1_000_000_000))
	total.Add(total, bigMulInt(dur.milliseconds, 1_000_000))
	total.Add(total, bigMulInt(dur.microseconds, 1_000))
	total.Add(total, bigMulInt(dur.nanoseconds, 1))
	return total
}

// dayNanos folds the receiver's six fields into the nanosecond count since midnight, in
// [0, nsPerDay). It is the shared starting point for the wall-clock arithmetic and the
// rounding, which both add or round the count and split it back with plainTimeFromDayNanos.
func (pt *PlainTime) dayNanos() *big.Int {
	total := new(big.Int)
	total.Add(total, bigMulInt(float64(pt.hour), 3_600_000_000_000))
	total.Add(total, bigMulInt(float64(pt.minute), 60_000_000_000))
	total.Add(total, bigMulInt(float64(pt.second), 1_000_000_000))
	total.Add(total, bigMulInt(float64(pt.millisecond), 1_000_000))
	total.Add(total, bigMulInt(float64(pt.microsecond), 1_000))
	total.Add(total, big.NewInt(int64(pt.nanosecond)))
	return total
}

// Round implements Temporal.PlainTime.prototype.round: it rounds the wall clock to a
// multiple of roundingIncrement of smallestUnit under one of the nine rounding modes. The
// smallestUnit fixes the quantum in nanoseconds and the divisor the increment must divide,
// hour into 24, minute and second into 60, and each sub-second unit into 1000; an increment
// that is not a positive integer below the divisor and dividing it throws a RangeError. The
// rounded count wraps mod 24 hours, so rounding up from late in the day lands on the next
// day's clock. The receiver is unchanged.
func (pt *PlainTime) Round(smallestUnit string, increment float64, roundingMode string) *PlainTime {
	unitNs, dividend, _ := plainTimeUnitInfo(smallestUnit)
	inc := int64(toIntegerWithTruncation(increment))
	if inc < 1 || inc >= dividend || dividend%inc != 0 {
		Throw(NewRangeError(FromGoString("Temporal.PlainTime.prototype.round roundingIncrement is out of range")))
	}
	quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
	rounded := roundBigToIncrement(pt.dayNanos(), quantum, roundingMode)
	rounded.Mod(rounded, nsPerDay)
	return plainTimeFromDayNanos(rounded)
}

// plainTimeUnitInfo returns the nanosecond size of a Temporal time unit, the count of that
// unit in the next larger unit that a rounding increment must divide evenly and stay below,
// and the unit's rank from hour, 0, down to nanosecond, 5. The round and difference methods
// share it so their unit tables cannot drift apart.
func plainTimeUnitInfo(unit string) (unitNs, dividend int64, rank int) {
	switch unit {
	case "hour":
		return 3_600_000_000_000, 24, 0
	case "minute":
		return 60_000_000_000, 60, 1
	case "second":
		return 1_000_000_000, 60, 2
	case "millisecond":
		return 1_000_000, 1000, 3
	case "microsecond":
		return 1_000, 1000, 4
	case "nanosecond":
		return 1, 1000, 5
	}
	Throw(NewRangeError(FromGoString("Temporal.PlainTime time unit is invalid")))
	return 0, 0, 0
}

// Until returns the signed wall-clock difference from the receiver to other as a Duration,
// balanced from largestUnit down and rounded at smallestUnit under roundingMode. The two
// times sit within one day, so the difference is under 24 hours before balancing.
func (pt *PlainTime) Until(other *PlainTime, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	return plainTimeDifference(pt, other, largestUnit, smallestUnit, increment, roundingMode)
}

// Since returns the signed wall-clock difference from other to the receiver, the reverse of
// Until. Both round the signed difference so the mode acts on the true sign, which matches
// the specification's rule of negating the mode and the result for since.
func (pt *PlainTime) Since(other *PlainTime, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	return plainTimeDifference(other, pt, largestUnit, smallestUnit, increment, roundingMode)
}

func plainTimeDifference(from, to *PlainTime, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	unitNs, dividend, smallRank := plainTimeUnitInfo(smallestUnit)
	_, _, largeRank := plainTimeUnitInfo(largestUnit)
	if largeRank > smallRank {
		Throw(NewRangeError(FromGoString("Temporal.PlainTime difference largestUnit cannot be smaller than smallestUnit")))
	}
	inc := int64(toIntegerWithTruncation(increment))
	if inc < 1 || inc >= dividend || dividend%inc != 0 {
		Throw(NewRangeError(FromGoString("Temporal.PlainTime difference roundingIncrement is out of range")))
	}
	diff := new(big.Int).Sub(to.dayNanos(), from.dayNanos())
	quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
	diff = roundBigToIncrement(diff, quantum, roundingMode)
	return durationFromDayNanos(diff, largeRank)
}

// durationFromDayNanos balances a signed nanosecond difference into a Duration's six time
// fields, the field at largeRank unbounded and each smaller one wrapping, all sharing the
// sign of the difference. The rank runs hour, 0, down to nanosecond, 5, and the fields above
// largeRank stay zero. Truncated division keeps every field on the same side of zero.
func durationFromDayNanos(total *big.Int, largeRank int) *Duration {
	sizes := [6]int64{3_600_000_000_000, 60_000_000_000, 1_000_000_000, 1_000_000, 1_000, 1}
	var fields [6]float64
	rem := new(big.Int).Set(total)
	q := new(big.Int)
	for i := largeRank; i < 6; i++ {
		size := big.NewInt(sizes[i])
		q.Quo(rem, size)
		fields[i] = float64(q.Int64())
		rem.Rem(rem, size)
	}
	return NewDuration(0, 0, 0, 0, fields[0], fields[1], fields[2], fields[3], fields[4], fields[5])
}

// roundBigToIncrement rounds x to a multiple of increment (a positive value) under one of
// the nine Temporal rounding modes, ties resolved by the mode. It is the shared rounding
// primitive the Temporal round methods call. The quotient is a Euclidean floor, so low is
// the multiple at or below x and high the one above, and the mode picks between them; the
// sign of x decides trunc, expand, and the half-toward-zero ties, so the helper is correct
// for the signed counts the other Temporal types will round.
func roundBigToIncrement(x, increment *big.Int, mode string) *big.Int {
	q := new(big.Int)
	rem := new(big.Int)
	q.DivMod(x, increment, rem)
	if rem.Sign() == 0 {
		return new(big.Int).Set(x)
	}
	low := new(big.Int).Mul(q, increment)
	high := new(big.Int).Add(low, increment)
	cmp := new(big.Int).Lsh(rem, 1).Cmp(increment)
	pickHigh := false
	switch mode {
	case "ceil":
		pickHigh = true
	case "floor":
		pickHigh = false
	case "trunc":
		pickHigh = x.Sign() < 0
	case "expand":
		pickHigh = x.Sign() >= 0
	case "halfCeil":
		pickHigh = cmp >= 0
	case "halfFloor":
		pickHigh = cmp > 0
	case "halfExpand":
		pickHigh = cmp > 0 || (cmp == 0 && x.Sign() >= 0)
	case "halfTrunc":
		pickHigh = cmp > 0 || (cmp == 0 && x.Sign() < 0)
	case "halfEven":
		pickHigh = cmp > 0 || (cmp == 0 && q.Bit(0) == 1)
	default:
		Throw(NewRangeError(FromGoString("invalid roundingMode")))
	}
	if pickHigh {
		return high
	}
	return low
}

// plainTimeFromDayNanos rebuilds a PlainTime from a nanosecond count already reduced into
// a single day, splitting it into the six fields most-significant first.
func plainTimeFromDayNanos(total *big.Int) *PlainTime {
	n := new(big.Int).Set(total)
	q := new(big.Int)
	r := new(big.Int)
	split := func(div int64) int {
		q.QuoRem(n, big.NewInt(div), r)
		n.Set(r)
		return int(q.Int64())
	}
	hour := split(3_600_000_000_000)
	minute := split(60_000_000_000)
	second := split(1_000_000_000)
	millisecond := split(1_000_000)
	microsecond := split(1_000)
	nanosecond := int(n.Int64())
	return &PlainTime{hour, minute, second, millisecond, microsecond, nanosecond}
}

// rejectTime throws a RangeError unless every field is in its ISO range: the hour in
// 0..23, the minute and second in 0..59, and each of the three sub-second fields in
// 0..999. The arguments are the truncated float64s from ToIntegerWithTruncation.
func rejectTime(hour, minute, second, millisecond, microsecond, nanosecond float64) {
	if hour < 0 || hour > 23 ||
		minute < 0 || minute > 59 ||
		second < 0 || second > 59 ||
		millisecond < 0 || millisecond > 999 ||
		microsecond < 0 || microsecond > 999 ||
		nanosecond < 0 || nanosecond > 999 {
		Throw(NewRangeError(FromGoString("Temporal.PlainTime field is out of range")))
	}
}

// Hour returns the hour, 0..23.
func (pt *PlainTime) Hour() float64 { return float64(pt.hour) }

// Minute returns the minute, 0..59.
func (pt *PlainTime) Minute() float64 { return float64(pt.minute) }

// Second returns the second, 0..59.
func (pt *PlainTime) Second() float64 { return float64(pt.second) }

// Millisecond returns the millisecond, 0..999.
func (pt *PlainTime) Millisecond() float64 { return float64(pt.millisecond) }

// Microsecond returns the microsecond, 0..999.
func (pt *PlainTime) Microsecond() float64 { return float64(pt.microsecond) }

// Nanosecond returns the nanosecond, 0..999.
func (pt *PlainTime) Nanosecond() float64 { return float64(pt.nanosecond) }

// Equals implements Temporal.PlainTime.prototype.equals: two times are equal when all
// six fields match.
func (pt *PlainTime) Equals(other *PlainTime) bool {
	return pt.hour == other.hour && pt.minute == other.minute && pt.second == other.second &&
		pt.millisecond == other.millisecond && pt.microsecond == other.microsecond &&
		pt.nanosecond == other.nanosecond
}

// PlainTimeCompare implements Temporal.PlainTime.compare, the static comparator: -1 if
// a precedes b, 1 if a follows b, 0 if they are the same time. It compares the fields
// from the most significant down, stopping at the first that differs.
func PlainTimeCompare(a, b *PlainTime) float64 {
	for _, d := range [...]int{
		a.hour - b.hour,
		a.minute - b.minute,
		a.second - b.second,
		a.millisecond - b.millisecond,
		a.microsecond - b.microsecond,
		a.nanosecond - b.nanosecond,
	} {
		if d < 0 {
			return -1
		}
		if d > 0 {
			return 1
		}
	}
	return 0
}

// isoString renders the ISO 8601 time, HH:MM:SS, with a fractional-second part
// appended only when a sub-second field is set, rendered to the fewest digits (the
// nine-digit nanosecond total with trailing zeros trimmed). A time on the whole
// second renders without a fractional part at all. It is the Go string toString
// wraps, and the piece PlainDateTime joins with the date across a "T".
func (pt *PlainTime) isoString() string {
	s := twoDigit(pt.hour) + ":" + twoDigit(pt.minute) + ":" + twoDigit(pt.second)
	frac := pt.millisecond*1_000_000 + pt.microsecond*1_000 + pt.nanosecond
	if frac > 0 {
		s += "." + strings.TrimRight(zeroPad(frac, 9), "0")
	}
	return s
}

// ToString implements Temporal.PlainTime.prototype.toString for the default options:
// HH:MM:SS, with a fractional-second part appended only when a sub-second field is set,
// rendered to the fewest digits (the nine-digit nanosecond total with trailing zeros
// trimmed). A time on the whole second renders without a fractional part at all.
func (pt *PlainTime) ToString() BStr {
	return FromGoString(pt.isoString())
}

// ToJSON implements Temporal.PlainTime.prototype.toJSON, the same ISO string toString
// produces under default options.
func (pt *PlainTime) ToJSON() BStr { return pt.ToString() }

// PlainDateTime is bento's runtime representation of a Temporal.PlainDateTime (Temporal
// §5): a calendar date paired with a wall-clock time, no zone. It is exactly a PlainDate
// and a PlainTime carried together, so it holds one of each and delegates every field,
// every string rendering, and both comparisons to them rather than restating the
// calendar and the time math. It carries whatever calendar its date does, iso8601 or
// gregory in this slice, so era and eraYear and the [u-ca=...] annotation follow from
// the date half.
type PlainDateTime struct {
	date PlainDate
	time PlainTime
}

// NewPlainDateTime builds a PlainDateTime from the constructor's three date arguments and
// up to six time arguments (isoYear, isoMonth, isoDay, then hour, minute, second,
// millisecond, microsecond, nanosecond). It runs ToIntegerWithTruncation on every argument
// first, so a NaN or non-finite component throws a RangeError before any range check, then
// RejectISODate and RejectTime, so an out-of-range date or time throws a RangeError, the
// order new Temporal.PlainDateTime(...) follows in the specification. Every time argument
// defaults to zero; the lowerer pads the missing trailing components before the call, so
// this constructor always sees nine numbers.
func NewPlainDateTime(isoYear, isoMonth, isoDay, hour, minute, second, millisecond, microsecond, nanosecond float64) *PlainDateTime {
	y := toIntegerWithTruncation(isoYear)
	mo := toIntegerWithTruncation(isoMonth)
	d := toIntegerWithTruncation(isoDay)
	h := toIntegerWithTruncation(hour)
	mi := toIntegerWithTruncation(minute)
	s := toIntegerWithTruncation(second)
	ms := toIntegerWithTruncation(millisecond)
	us := toIntegerWithTruncation(microsecond)
	ns := toIntegerWithTruncation(nanosecond)
	rejectISODate(y, mo, d)
	rejectTime(h, mi, s, ms, us, ns)
	return &PlainDateTime{
		date: PlainDate{year: int(y), month: int(mo), day: int(d)},
		time: PlainTime{int(h), int(mi), int(s), int(ms), int(us), int(ns)},
	}
}

// PlainDateTimeFrom implements Temporal.PlainDateTime.from for a PlainDateTime argument: it
// returns a fresh PlainDateTime with the same date and time, the copy the specification
// makes so the result is a distinct object that compares equal to its source. from over a
// string or a property bag hands back at lowering, so this is only reached with a
// PlainDateTime in hand.
func PlainDateTimeFrom(pdt *PlainDateTime) *PlainDateTime {
	return &PlainDateTime{date: pdt.date, time: pdt.time}
}

// NewPlainDateTimeCal builds a PlainDateTime under a named calendar, the ten-argument
// constructor new Temporal.PlainDateTime(y, mo, d, h, mi, s, ms, us, ns, calendar). It
// mirrors NewPlainDateTime with the specification's calendar step folded in: the
// components truncate first, then the calendar is canonicalized, so an unhosted id
// throws a RangeError, then the date and time are rejected.
func NewPlainDateTimeCal(isoYear, isoMonth, isoDay, hour, minute, second, millisecond, microsecond, nanosecond float64, calendar string) *PlainDateTime {
	y := toIntegerWithTruncation(isoYear)
	mo := toIntegerWithTruncation(isoMonth)
	d := toIntegerWithTruncation(isoDay)
	h := toIntegerWithTruncation(hour)
	mi := toIntegerWithTruncation(minute)
	s := toIntegerWithTruncation(second)
	ms := toIntegerWithTruncation(millisecond)
	us := toIntegerWithTruncation(microsecond)
	ns := toIntegerWithTruncation(nanosecond)
	cal, ok := canonicalCalendar(calendar)
	if !ok {
		Throw(NewRangeError(FromGoString("invalid calendar identifier " + calendar)))
	}
	rejectISODate(y, mo, d)
	rejectTime(h, mi, s, ms, us, ns)
	return &PlainDateTime{
		date: PlainDate{year: int(y), month: int(mo), day: int(d), cal: cal},
		time: PlainTime{int(h), int(mi), int(s), int(ms), int(us), int(ns)},
	}
}

// PlainDateTimeWithCalendar implements Temporal.PlainDateTime.prototype.withCalendar: it
// reinterprets the same ISO date and time under another calendar, returning a fresh
// PlainDateTime. The id is canonicalized and validated, so an unhosted or invalid one
// throws a RangeError.
func PlainDateTimeWithCalendar(pdt *PlainDateTime, calendar string) *PlainDateTime {
	cal, ok := canonicalCalendar(calendar)
	if !ok {
		Throw(NewRangeError(FromGoString("invalid calendar identifier " + calendar)))
	}
	nd := pdt.date
	nd.cal = cal
	return &PlainDateTime{date: nd, time: pdt.time}
}

// Year returns the ISO year.
func (pdt *PlainDateTime) Year() float64 { return pdt.date.Year() }

// Month returns the ISO month, 1..12.
func (pdt *PlainDateTime) Month() float64 { return pdt.date.Month() }

// Day returns the ISO day of the month.
func (pdt *PlainDateTime) Day() float64 { return pdt.date.Day() }

// Hour returns the hour, 0..23.
func (pdt *PlainDateTime) Hour() float64 { return pdt.time.Hour() }

// Minute returns the minute, 0..59.
func (pdt *PlainDateTime) Minute() float64 { return pdt.time.Minute() }

// Second returns the second, 0..59.
func (pdt *PlainDateTime) Second() float64 { return pdt.time.Second() }

// Millisecond returns the millisecond, 0..999.
func (pdt *PlainDateTime) Millisecond() float64 { return pdt.time.Millisecond() }

// Microsecond returns the microsecond, 0..999.
func (pdt *PlainDateTime) Microsecond() float64 { return pdt.time.Microsecond() }

// Nanosecond returns the nanosecond, 0..999.
func (pdt *PlainDateTime) Nanosecond() float64 { return pdt.time.Nanosecond() }

// CalendarId returns the calendar identifier the date half carries.
func (pdt *PlainDateTime) CalendarId() BStr { return pdt.date.CalendarId() }

// MonthCode returns the ISO month code, "M" followed by the two-digit month.
func (pdt *PlainDateTime) MonthCode() BStr { return pdt.date.MonthCode() }

// DayOfWeek returns the ISO day of the week, Monday=1 through Sunday=7.
func (pdt *PlainDateTime) DayOfWeek() float64 { return pdt.date.DayOfWeek() }

// DayOfYear returns the 1-based ordinal day within the year.
func (pdt *PlainDateTime) DayOfYear() float64 { return pdt.date.DayOfYear() }

// DaysInWeek is always 7 in the ISO calendar.
func (pdt *PlainDateTime) DaysInWeek() float64 { return pdt.date.DaysInWeek() }

// DaysInMonth returns the number of days in this date's month.
func (pdt *PlainDateTime) DaysInMonth() float64 { return pdt.date.DaysInMonth() }

// DaysInYear returns 366 in a leap year and 365 otherwise.
func (pdt *PlainDateTime) DaysInYear() float64 { return pdt.date.DaysInYear() }

// MonthsInYear is always 12 in the ISO calendar.
func (pdt *PlainDateTime) MonthsInYear() float64 { return pdt.date.MonthsInYear() }

// InLeapYear reports whether this date's year is an ISO leap year.
func (pdt *PlainDateTime) InLeapYear() bool { return pdt.date.InLeapYear() }

// Era, EraYear, WeekOfYear, and YearOfWeek read the calendar-dependent fields off
// the date half, so a date-time answers them the same as the date it carries: era
// and eraYear undefined under ISO and the gregory era under gregory, weekOfYear and
// yearOfWeek the ISO 8601 week date.
func (pdt *PlainDateTime) Era() Opt[BStr]           { return pdt.date.Era() }
func (pdt *PlainDateTime) EraYear() Opt[float64]    { return pdt.date.EraYear() }
func (pdt *PlainDateTime) WeekOfYear() Opt[float64] { return pdt.date.WeekOfYear() }
func (pdt *PlainDateTime) YearOfWeek() Opt[float64] { return pdt.date.YearOfWeek() }

// Equals implements Temporal.PlainDateTime.prototype.equals: two date-times are equal when
// their dates and their times are each equal under the same (ISO) calendar.
func (pdt *PlainDateTime) Equals(other *PlainDateTime) bool {
	return pdt.date.Equals(&other.date) && pdt.time.Equals(&other.time)
}

// PlainDateTimeCompare implements Temporal.PlainDateTime.compare, the static comparator:
// -1 if a precedes b, 1 if a follows b, 0 if they are the same instant on the wall clock.
// It compares the dates first and falls to the times only when the dates are equal.
func PlainDateTimeCompare(a, b *PlainDateTime) float64 {
	if c := PlainDateCompare(&a.date, &b.date); c != 0 {
		return c
	}
	return PlainTimeCompare(&a.time, &b.time)
}

// ToString implements Temporal.PlainDateTime.prototype.toString for the default options:
// the ISO 8601 date and time joined by "T", each rendered as its own type renders it, so
// the fractional-second part appears only when a sub-second field is set.
func (pdt *PlainDateTime) ToString() BStr {
	return FromGoString(pdt.date.dateCore() + "T" + pdt.time.isoString() + pdt.date.calendarAnnotation())
}

// ToJSON implements Temporal.PlainDateTime.prototype.toJSON, the same ISO string toString
// produces under default options.
func (pdt *PlainDateTime) ToJSON() BStr { return pdt.ToString() }

// ToPlainDate implements Temporal.PlainDateTime.prototype.toPlainDate: the calendar date half
// of the date-time, carrying the same calendar, with the clock dropped.
func (pdt *PlainDateTime) ToPlainDate() *PlainDate {
	d := pdt.date
	return &d
}

// ToPlainTime implements Temporal.PlainDateTime.prototype.toPlainTime: the wall-clock half of
// the date-time, with the calendar date dropped.
func (pdt *PlainDateTime) ToPlainTime() *PlainTime {
	t := pdt.time
	return &t
}

// ToZonedDateTime implements Temporal.PlainDateTime.prototype.toZonedDateTime: it pins the
// date-time's wall clock to a time zone, resolving the exact instant under the given
// disambiguation. compatible (the default) takes the earlier reading in a fall-back overlap and
// shifts forward across a spring-forward gap; earlier and later pick the two overlap readings and
// the two sides of a gap; reject throws on an ambiguous or gapped reading. The result keeps the
// date-time's calendar, so a non-ISO date-time stays under its calendar.
func (pdt *PlainDateTime) ToZonedDateTime(timeZone, disambiguation string) *ZonedDateTime {
	loc, canon := resolveTimeZone(timeZone)
	wall := wallNanoseconds(isoParse{
		year: pdt.date.year, month: pdt.date.month, day: pdt.date.day,
		hour: pdt.time.hour, minute: pdt.time.minute, second: pdt.time.second,
		millisecond: pdt.time.millisecond, microsecond: pdt.time.microsecond, nanosecond: pdt.time.nanosecond,
	})
	epoch := disambiguatePossible(loc, wall, disambiguation, canon)
	validateEpochNanoseconds(epoch)
	return &ZonedDateTime{ns: epoch, loc: loc, tzID: FromGoString(canon), cal: pdt.date.cal}
}

// WithFields implements Temporal.PlainDateTime.prototype.with: it lays the bag's present date and
// time fields over the receiver's own and regulates each half with the overflow option, so an
// omitted field keeps its current value. The date half regulates exactly as PlainDate.with, so the
// year is read in the receiver's calendar reckoning and the day clamps to the resulting month's
// length under constrain; the time half clamps to its ISO maxima under constrain. Under reject an
// out-of-range field in either half throws a RangeError. monthCode and the era fields are not read
// here, the lowerer hands back a bag that carries them. The receiver is unchanged and its calendar
// carries through.
func (pdt *PlainDateTime) WithFields(year, month, day, hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], overflow string) *PlainDateTime {
	date := pdt.date.WithFields(year, month, day, overflow)
	base := [6]float64{
		float64(pdt.time.hour), float64(pdt.time.minute), float64(pdt.time.second),
		float64(pdt.time.millisecond), float64(pdt.time.microsecond), float64(pdt.time.nanosecond),
	}
	time := regulatePlainTime(base, [6]Opt[float64]{hour, minute, second, millisecond, microsecond, nanosecond}, overflow)
	return &PlainDateTime{date: *date, time: *time}
}

// WithPlainTime implements Temporal.PlainDateTime.prototype.withPlainTime: it keeps the calendar
// date and replaces the wall clock, defaulting to midnight when no time is given. The result keeps
// this date-time's calendar, so a non-ISO date-time stays under its calendar. The receiver's date
// is copied, so the new PlainDateTime shares no state with it.
func (pdt *PlainDateTime) WithPlainTime(time *PlainTime) *PlainDateTime {
	t := PlainTime{}
	if time != nil {
		t = *time
	}
	return &PlainDateTime{date: pdt.date, time: t}
}

// AddDateTime implements Temporal.PlainDateTime.prototype.add and, over a negated Duration,
// subtract. Unlike a PlainDate, which drops the duration's sub-day time part, a PlainDateTime
// carries a wall clock, so the time part folds into the clock first: the duration's six time
// components add to the receiver's time, and the total splits into a time of day in [0, one day)
// and a whole-day carry, floored so a net-negative time lands on the previous day's clock. That
// carry joins the duration's days, and the years, months, weeks, and days add to the date through
// addISODate under the overflow rule. The result keeps the receiver's calendar, and an
// out-of-range date throws a RangeError.
func (pdt *PlainDateTime) AddDateTime(dur *Duration, overflow string) *PlainDateTime {
	total := pdt.time.dayNanos()
	total.Add(total, durationTimeNanos(dur))
	dayCarry := new(big.Int)
	timeOfDay := new(big.Int)
	dayCarry.DivMod(total, nsPerDay, timeOfDay)
	days := new(big.Int).Add(big.NewInt(int64(dur.days)), dayCarry)
	y, m, d := addISODate(pdt.date.year, pdt.date.month, pdt.date.day, int(dur.years), int(dur.months), int(dur.weeks), days, overflow)
	if !isoDateWithinLimits(y, m, d) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDateTime is outside the representable range")))
	}
	return &PlainDateTime{
		date: PlainDate{year: y, month: m, day: d, cal: pdt.date.cal},
		time: *plainTimeFromDayNanos(timeOfDay),
	}
}

// Until returns the difference from the receiver to other as a Duration balanced from
// largestUnit down. Since returns the difference from other to the receiver, the negation of
// Until, so the month anchoring stays on the receiver the way the specification requires.
func (pdt *PlainDateTime) Until(other *PlainDateTime, largestUnit string) *Duration {
	return plainDateTimeDifference(pdt, other, largestUnit)
}

// Since returns the negation of the receiver-to-other difference.
func (pdt *PlainDateTime) Since(other *PlainDateTime, largestUnit string) *Duration {
	return plainDateTimeDifference(pdt, other, largestUnit).Negated()
}

// plainDateTimeDifference implements the specification's DifferenceISODateTime: it splits the
// distance from one date-time to another into a calendar date part and a wall-clock time part
// that share one sign. It first takes the signed time difference within a day. When that points
// opposite the calendar direction, it borrows a day: the start date steps one day toward the
// target and twenty-four hours fold into the time difference, so the time part comes to share the
// date's sign and stays under a day. The date part is then the calendar difference from the
// adjusted start to the target at largestUnit, and the time part balances from hours down. For a
// time largestUnit the whole date difference in days folds into the time and balances from that
// unit. The two date-times must share a calendar.
func plainDateTimeDifference(from, to *PlainDateTime, largestUnit string) *Duration {
	if from.date.calendarID() != to.date.calendarID() {
		Throw(NewRangeError(FromGoString("Temporal.PlainDateTime difference between two calendars is not allowed")))
	}
	timeDiff := new(big.Int).Sub(to.time.dayNanos(), from.time.dayNanos())
	timeSign := timeDiff.Sign()
	dateSign := isoDateCompare(to.date.year, to.date.month, to.date.day, from.date.year, from.date.month, from.date.day)
	ay, am, ad := from.date.year, from.date.month, from.date.day
	if timeSign != 0 && dateSign != 0 && timeSign == -dateSign {
		ay, am, ad = epochDaysToISO(isoToEpochDays(ay, am, ad) + dateSign)
		timeDiff.Add(timeDiff, new(big.Int).Mul(big.NewInt(int64(dateSign)), nsPerDay))
	}
	switch largestUnit {
	case "year", "month", "week", "day":
		years, months, weeks, days := differenceISODate(ay, am, ad, to.date.year, to.date.month, to.date.day, largestUnit)
		t := durationFromDayNanos(timeDiff, 0)
		return NewDuration(float64(years), float64(months), float64(weeks), float64(days), t.hours, t.minutes, t.seconds, t.milliseconds, t.microseconds, t.nanoseconds)
	default:
		_, _, _, days := differenceISODate(ay, am, ad, to.date.year, to.date.month, to.date.day, "day")
		timeDiff.Add(timeDiff, new(big.Int).Mul(big.NewInt(int64(days)), nsPerDay))
		_, _, largeRank := plainTimeUnitInfo(largestUnit)
		return durationFromDayNanos(timeDiff, largeRank)
	}
}

// Round implements Temporal.PlainDateTime.prototype.round: it rounds the wall clock to a
// multiple of roundingIncrement of smallestUnit under one of the nine rounding modes, carrying a
// whole day into the date when the clock rounds up past midnight. The day unit rounds the whole
// date-time to the nearest midnight, so its increment must be exactly one; the time units fix
// their quantum and the divisor the increment must divide the same way PlainTime.round does. An
// increment out of range throws a RangeError, and an out-of-range carried date does too. The
// receiver is unchanged.
func (pdt *PlainDateTime) Round(smallestUnit string, increment float64, roundingMode string) *PlainDateTime {
	inc := int64(toIntegerWithTruncation(increment))
	var rounded *big.Int
	if smallestUnit == "day" {
		if inc != 1 {
			Throw(NewRangeError(FromGoString("Temporal.PlainDateTime.prototype.round roundingIncrement is out of range")))
		}
		rounded = roundBigToIncrement(pdt.time.dayNanos(), nsPerDay, roundingMode)
	} else {
		unitNs, dividend, _ := plainTimeUnitInfo(smallestUnit)
		if inc < 1 || inc >= dividend || dividend%inc != 0 {
			Throw(NewRangeError(FromGoString("Temporal.PlainDateTime.prototype.round roundingIncrement is out of range")))
		}
		quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
		rounded = roundBigToIncrement(pdt.time.dayNanos(), quantum, roundingMode)
	}
	dayCarry := new(big.Int)
	timeOfDay := new(big.Int)
	dayCarry.DivMod(rounded, nsPerDay, timeOfDay)
	y, m, d := addISODate(pdt.date.year, pdt.date.month, pdt.date.day, 0, 0, 0, dayCarry, "constrain")
	if !isoDateWithinLimits(y, m, d) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDateTime is outside the representable range")))
	}
	return &PlainDateTime{
		date: PlainDate{year: y, month: m, day: d, cal: pdt.date.cal},
		time: *plainTimeFromDayNanos(timeOfDay),
	}
}

// PlainYearMonth is bento's runtime representation of a Temporal.PlainYearMonth (Temporal
// §9): a calendar year and month with no day, no time, and no zone, the way a credit card
// carries an expiry. Like PlainDate it hosts only the ISO 8601 calendar; a non-ISO calendar
// hands back at lowering. The specification anchors a year-month to a reference ISO day so a
// calendar can resolve calendar-dependent fields, but the ISO calendar needs no reference,
// so this type stores only the year and the month and derives every getter from them.
type PlainYearMonth struct {
	year  int    // proleptic Gregorian (ISO) year, may be negative or above 9999
	month int    // 1..12
	cal   string // calendar id; "" reads as iso8601
}

// NewPlainYearMonth builds a PlainYearMonth from the constructor's two number arguments,
// running ToIntegerWithTruncation on each and then RejectISOYearMonth, so a fractional
// argument truncates toward zero, a non-finite one throws a RangeError, and a month outside
// 1..12 or a year-month outside the representable range throws a RangeError. A third calendar
// argument and a fourth reference-day argument are not accepted here; both hand back at
// lowering, so this constructor is only ever reached for the ISO calendar with the default
// reference day.
func NewPlainYearMonth(isoYear, isoMonth float64) *PlainYearMonth {
	y := toIntegerWithTruncation(isoYear)
	m := toIntegerWithTruncation(isoMonth)
	rejectISOYearMonth(y, m)
	return &PlainYearMonth{year: int(y), month: int(m)}
}

// PlainYearMonthFrom implements Temporal.PlainYearMonth.from for a PlainYearMonth argument:
// it returns a fresh PlainYearMonth with the same fields, the copy the specification makes.
// from over a string or a property bag hands back at lowering.
func PlainYearMonthFrom(ym *PlainYearMonth) *PlainYearMonth {
	return &PlainYearMonth{year: ym.year, month: ym.month}
}

// rejectISOYearMonth throws a RangeError unless (year, month) is a real ISO year-month
// within Temporal's representable range: the month in 1..12 and the year-month between
// -271821-04 and +275760-09 inclusive, the bounds ISOYearMonthWithinLimits fixes. The
// arguments are the truncated float64s so the year bound is checked before the value is
// narrowed to an int, which keeps a wildly out-of-range year from wrapping on the conversion.
func rejectISOYearMonth(year, month float64) {
	if month < 1 || month > 12 {
		Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth month must be between 1 and 12")))
	}
	if year < -271821 || year > 275760 {
		Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth is outside the representable range")))
	}
	if !isoYearMonthWithinLimits(int(year), int(month)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth is outside the representable range")))
	}
}

// isoYearMonthWithinLimits reports whether a year-month falls in Temporal's representable
// range, -271821-04 through +275760-09 inclusive. Unlike a full date it has no day to bound,
// so the check turns only on the year and, at each end, the month.
func isoYearMonthWithinLimits(year, month int) bool {
	if year < -271821 || year > 275760 {
		return false
	}
	if year == -271821 && month < 4 {
		return false
	}
	if year == 275760 && month > 9 {
		return false
	}
	return true
}

// displayYear returns the year the calendar counts, the ISO year for iso8601, gregory, and
// japanese, and the ISO year minus 1911 for roc, the Minguo calendar. It mirrors
// PlainDate.displayYear so a year-month reports the same year the date it came from does.
func (ym *PlainYearMonth) displayYear() int {
	if ym.cal == "roc" {
		return ym.year - 1911
	}
	return ym.year
}

// Year returns the year the calendar counts: the ISO year under iso8601, gregory, and
// japanese, and the ISO year minus 1911 under roc.
func (ym *PlainYearMonth) Year() float64 { return float64(ym.displayYear()) }

// Month returns the ISO month, 1..12.
func (ym *PlainYearMonth) Month() float64 { return float64(ym.month) }

// calendarID maps the empty stored calendar to "iso8601" so a year-month built without a
// calendar reads as ISO.
func (ym *PlainYearMonth) calendarID() string {
	if ym.cal == "" {
		return "iso8601"
	}
	return ym.cal
}

// CalendarId returns the calendar identifier, "iso8601", "gregory", "roc", or "japanese".
func (ym *PlainYearMonth) CalendarId() BStr { return FromGoString(ym.calendarID()) }

// calendarAnnotation returns the RFC 9557 calendar suffix a non-ISO calendar appends to a
// toString, "[u-ca=<id>]", or "" for the ISO calendar, which prints no annotation.
func (ym *PlainYearMonth) calendarAnnotation() string {
	if ym.cal == "" || ym.cal == "iso8601" {
		return ""
	}
	return "[u-ca=" + ym.cal + "]"
}

// MonthCode returns the ISO month code, "M" followed by the two-digit month. The ISO
// calendar has no leap months, so the code never carries the trailing "L".
func (ym *PlainYearMonth) MonthCode() BStr {
	code := "M"
	if ym.month < 10 {
		code += "0"
	}
	return FromGoString(code + strconv.Itoa(ym.month))
}

// DaysInMonth returns the number of days in this year-month's month.
func (ym *PlainYearMonth) DaysInMonth() float64 { return float64(isoDaysInMonth(ym.year, ym.month)) }

// DaysInYear returns 366 in a leap year and 365 otherwise.
func (ym *PlainYearMonth) DaysInYear() float64 {
	if isLeapISO(ym.year) {
		return 366
	}
	return 365
}

// MonthsInYear is always 12 in the ISO calendar.
func (ym *PlainYearMonth) MonthsInYear() float64 { return 12 }

// InLeapYear reports whether this year-month's year is an ISO leap year.
func (ym *PlainYearMonth) InLeapYear() bool { return isLeapISO(ym.year) }

// Equals implements Temporal.PlainYearMonth.prototype.equals: two year-months are equal
// when their year and month match under the same (ISO) calendar.
func (ym *PlainYearMonth) Equals(other *PlainYearMonth) bool {
	return ym.year == other.year && ym.month == other.month && ym.calendarID() == other.calendarID()
}

// PlainYearMonthCompare implements Temporal.PlainYearMonth.compare, the static comparator:
// -1 if a precedes b, 1 if a follows b, 0 if they are the same year-month.
func PlainYearMonthCompare(a, b *PlainYearMonth) float64 {
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
	default:
		return 0
	}
}

// ToString implements Temporal.PlainYearMonth.prototype.toString for the default options.
// The ISO calendar prints YYYY-MM, the year expanded to a signed six-digit form outside
// 0..9999 and the reference day hidden. A non-ISO calendar prints the full ISO reference
// date YYYY-MM-DD followed by its "[u-ca=<id>]" annotation; the four hosted calendars align
// their months with ISO months, so the reference day is the first of the month.
func (ym *PlainYearMonth) ToString() BStr {
	core := formatISOYear(ym.year) + "-" + twoDigit(ym.month)
	if ann := ym.calendarAnnotation(); ann != "" {
		return FromGoString(core + "-01" + ann)
	}
	return FromGoString(core)
}

// ToJSON implements Temporal.PlainYearMonth.prototype.toJSON, the same ISO string toString
// produces under default options.
func (ym *PlainYearMonth) ToJSON() BStr { return ym.ToString() }

// AddDuration implements Temporal.PlainYearMonth.prototype.add and, over a negated duration,
// subtract. A year-month has no day, so the specification anchors the arithmetic to a reference
// day: the first of the month when the duration runs forward, the last of the month when it runs
// backward, so a month step that would only survive by clamping a large day is never counted.
// The reference-day date carries the full duration through PlainDate.AddDate, which folds the
// time part into whole days, and the moved date narrows back to a year-month. The result keeps
// the receiver's calendar, and an out-of-range step or, under reject, a clamped day throws a
// RangeError.
func (ym *PlainYearMonth) AddDuration(dur *Duration, overflow string) *PlainYearMonth {
	day := 1
	if durationSign(dur) < 0 {
		day = isoDaysInMonth(ym.year, ym.month)
	}
	base := &PlainDate{year: ym.year, month: ym.month, day: day, cal: ym.cal}
	return base.AddDate(dur, overflow).ToPlainYearMonth()
}

// SubtractDuration implements Temporal.PlainYearMonth.prototype.subtract as AddDuration over the
// negated duration, so the reference day is chosen from the negated sign.
func (ym *PlainYearMonth) SubtractDuration(dur *Duration, overflow string) *PlainYearMonth {
	return ym.AddDuration(dur.Negated(), overflow)
}

// Until implements Temporal.PlainYearMonth.prototype.until, the span from the receiver to the
// argument as a years-and-months Duration. Since implements the mirror by negating it.
func (ym *PlainYearMonth) Until(other *PlainYearMonth, largestUnit string) *Duration {
	return plainYearMonthDifference(ym, other, largestUnit)
}

// Since implements Temporal.PlainYearMonth.prototype.since as the negation of Until, so
// a.since(b) is the span from b to a.
func (ym *PlainYearMonth) Since(other *PlainYearMonth, largestUnit string) *Duration {
	return plainYearMonthDifference(ym, other, largestUnit).Negated()
}

// plainYearMonthDifference measures the calendar distance between two year-months at the first
// of each month, so the day part is always zero and the result carries only years and months.
// largestUnit is "year" or "month"; under "month" the years roll into the month count. A
// difference between two calendars throws a RangeError.
func plainYearMonthDifference(from, to *PlainYearMonth, largestUnit string) *Duration {
	if from.calendarID() != to.calendarID() {
		Throw(NewRangeError(FromGoString("cannot compute the difference between two Temporal.PlainYearMonth values in different calendars")))
	}
	years, months, _, _ := differenceISODate(from.year, from.month, 1, to.year, to.month, 1, largestUnit)
	return NewDuration(float64(years), float64(months), 0, 0, 0, 0, 0, 0, 0, 0)
}

// WithFields implements Temporal.PlainYearMonth.prototype.with: it lays the bag's present year and
// month over the receiver's own fields and regulates the result with the overflow option, so an
// omitted field keeps its current value. The year is read in the receiver's calendar reckoning, so
// under roc a bag year maps back to the ISO year by adding 1911; the other hosted calendars count
// the ISO year directly. Under constrain the month clamps to 1..12; under reject an out-of-range
// month throws a RangeError. A month code lowers to a numeric month at compile time, and a bag
// carrying the era fields or a day hands back there, so only year and month reach here. The
// receiver is unchanged.
func (ym *PlainYearMonth) WithFields(year, month Opt[float64], overflow string) *PlainYearMonth {
	calYear := toIntegerWithTruncation(year.Or(float64(ym.displayYear())))
	m := toIntegerWithTruncation(month.Or(float64(ym.month)))
	isoYear := calYear
	if ym.cal == "roc" {
		isoYear = calYear + 1911
	}
	if overflow == timeOverflowReject {
		rejectISOYearMonth(isoYear, m)
	} else {
		m = clampFloat(m, 1, 12)
		if isoYear < -271821 || isoYear > 275760 {
			Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth is outside the representable range")))
		}
	}
	if !isoYearMonthWithinLimits(int(isoYear), int(m)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth is outside the representable range")))
	}
	return &PlainYearMonth{year: int(isoYear), month: int(m), cal: ym.cal}
}

// ToPlainDate implements Temporal.PlainYearMonth.prototype.toPlainDate: it combines the year-month
// with the day from the argument bag into a PlainDate in the receiver's calendar. The specification
// gives toPlainDate no overflow option, so the day always constrains to the month's length. An
// out-of-range result throws a RangeError.
func (ym *PlainYearMonth) ToPlainDate(day float64) *PlainDate {
	d := clampFloat(toIntegerWithTruncation(day), 1, float64(isoDaysInMonth(ym.year, ym.month)))
	if !isoDateWithinLimits(ym.year, ym.month, int(d)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return &PlainDate{year: ym.year, month: ym.month, day: int(d), cal: ym.cal}
}

// Era implements Temporal.PlainYearMonth.prototype.era by resolving the era at the first of the
// year-month's month, so a year-month reports the same era the date it came from does. It is
// undefined under the ISO calendar.
func (ym *PlainYearMonth) Era() Opt[BStr] {
	return (&PlainDate{year: ym.year, month: ym.month, day: 1, cal: ym.cal}).Era()
}

// EraYear implements Temporal.PlainYearMonth.prototype.eraYear, the year counted within the era at
// the first of the month. It is undefined under the ISO calendar.
func (ym *PlainYearMonth) EraYear() Opt[float64] {
	return (&PlainDate{year: ym.year, month: ym.month, day: 1, cal: ym.cal}).EraYear()
}

// PlainMonthDay is bento's runtime representation of a Temporal.PlainMonthDay (Temporal §10):
// a calendar month and day with no year, no time, and no zone, the way a birthday or a
// holiday recurs every year. Like PlainDate it hosts only the ISO 8601 calendar; a non-ISO
// calendar hands back at lowering. The specification anchors a month-day to a reference ISO
// year so a calendar can resolve which day the pair falls on; the ISO calendar needs it only
// to admit February 29, so this type stores the month and day and validates against the fixed
// leap reference year without keeping it.
type PlainMonthDay struct {
	month int    // 1..12
	day   int    // 1..isoDaysInMonth(monthDayReferenceYear, month)
	cal   string // calendar id; "" reads as iso8601
}

// monthDayReferenceYear is the ISO year a PlainMonthDay is validated against, 1972, a leap
// year so February 29 is a valid month-day. It is the reference the specification uses for
// the ISO calendar.
const monthDayReferenceYear = 1972

// NewPlainMonthDay builds a PlainMonthDay from the constructor's two number arguments, the
// month first and the day second, running ToIntegerWithTruncation on each and then
// RejectISOMonthDay, so a fractional argument truncates toward zero, a non-finite one throws
// a RangeError, and a month outside 1..12 or a day out of range for that month throws a
// RangeError. A third calendar argument and a fourth reference-year argument are not accepted
// here; both hand back at lowering, so this constructor is only ever reached for the ISO
// calendar with the default reference year.
func NewPlainMonthDay(isoMonth, isoDay float64) *PlainMonthDay {
	m := toIntegerWithTruncation(isoMonth)
	d := toIntegerWithTruncation(isoDay)
	rejectISOMonthDay(m, d)
	return &PlainMonthDay{month: int(m), day: int(d)}
}

// PlainMonthDayFrom implements Temporal.PlainMonthDay.from for a PlainMonthDay argument: it
// returns a fresh PlainMonthDay with the same fields, the copy the specification makes. from
// over a string or a property bag hands back at lowering.
func PlainMonthDayFrom(md *PlainMonthDay) *PlainMonthDay {
	return &PlainMonthDay{month: md.month, day: md.day}
}

// rejectISOMonthDay throws a RangeError unless (month, day) is a real ISO month-day: the
// month in 1..12 and the day in 1..(days in that month) measured against the leap reference
// year 1972, so February 29 is admitted and February 30 is rejected.
func rejectISOMonthDay(month, day float64) {
	if month < 1 || month > 12 {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay month must be between 1 and 12")))
	}
	m := int(month)
	if day < 1 || day > float64(isoDaysInMonth(monthDayReferenceYear, m)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay day is out of range for the month")))
	}
}

// MonthCode returns the ISO month code, "M" followed by the two-digit month. The ISO
// calendar has no leap months, so the code never carries the trailing "L". A month-day
// exposes its month only through this code, not through a numeric month getter.
func (md *PlainMonthDay) MonthCode() BStr {
	code := "M"
	if md.month < 10 {
		code += "0"
	}
	return FromGoString(code + strconv.Itoa(md.month))
}

// Day returns the ISO day of the month.
func (md *PlainMonthDay) Day() float64 { return float64(md.day) }

// calendarID maps the empty stored calendar to "iso8601" so a month-day built without a
// calendar reads as ISO.
func (md *PlainMonthDay) calendarID() string {
	if md.cal == "" {
		return "iso8601"
	}
	return md.cal
}

// CalendarId returns the calendar identifier, "iso8601", "gregory", "roc", or "japanese".
func (md *PlainMonthDay) CalendarId() BStr { return FromGoString(md.calendarID()) }

// calendarAnnotation returns the RFC 9557 calendar suffix a non-ISO calendar appends to a
// toString, "[u-ca=<id>]", or "" for the ISO calendar, which prints no annotation.
func (md *PlainMonthDay) calendarAnnotation() string {
	if md.cal == "" || md.cal == "iso8601" {
		return ""
	}
	return "[u-ca=" + md.cal + "]"
}

// Equals implements Temporal.PlainMonthDay.prototype.equals: two month-days are equal when
// their month and day match under the same calendar.
func (md *PlainMonthDay) Equals(other *PlainMonthDay) bool {
	return md.month == other.month && md.day == other.day && md.calendarID() == other.calendarID()
}

// ToString implements Temporal.PlainMonthDay.prototype.toString for the default options. The
// ISO calendar prints MM-DD, hiding the reference year. A non-ISO calendar prints the full ISO
// reference date, the leap reference year 1972 followed by the month and day, then its
// "[u-ca=<id>]" annotation, so the calendar can resolve which day the pair falls on.
func (md *PlainMonthDay) ToString() BStr {
	if ann := md.calendarAnnotation(); ann != "" {
		return FromGoString(formatISOYear(monthDayReferenceYear) + "-" + twoDigit(md.month) + "-" + twoDigit(md.day) + ann)
	}
	return FromGoString(twoDigit(md.month) + "-" + twoDigit(md.day))
}

// ToJSON implements Temporal.PlainMonthDay.prototype.toJSON, the same ISO string toString
// produces under default options.
func (md *PlainMonthDay) ToJSON() BStr { return md.ToString() }

// WithFields implements Temporal.PlainMonthDay.prototype.with: it lays the bag's present month and
// day over the receiver's own fields and regulates the result with the overflow option, so an
// omitted field keeps its current value. The day is validated against the leap reference year 1972,
// so February 29 is admitted. Under constrain the month clamps to 1..12 and the day to that month's
// length; under reject an out-of-range field throws a RangeError. A month code lowers to a numeric
// month at compile time, and a bag carrying a year hands back there, so only month and day reach
// here. The receiver is unchanged.
func (md *PlainMonthDay) WithFields(month, day Opt[float64], overflow string) *PlainMonthDay {
	m := toIntegerWithTruncation(month.Or(float64(md.month)))
	d := toIntegerWithTruncation(day.Or(float64(md.day)))
	if overflow == timeOverflowReject {
		rejectISOMonthDay(m, d)
	} else {
		m = clampFloat(m, 1, 12)
		d = clampFloat(d, 1, float64(isoDaysInMonth(monthDayReferenceYear, int(m))))
	}
	return &PlainMonthDay{month: int(m), day: int(d), cal: md.cal}
}

// ToPlainDate implements Temporal.PlainMonthDay.prototype.toPlainDate: it combines the month-day
// with the year from the argument bag into a PlainDate in the receiver's calendar. The year is read
// in the calendar's own reckoning, so a roc bag year maps back to the ISO year by adding 1911 while
// the other hosted calendars count the ISO year directly. The specification gives toPlainDate no
// overflow option, so the day always constrains to that year's month length, dropping February 29
// to the 28th in a common year. An out-of-range result throws a RangeError.
func (md *PlainMonthDay) ToPlainDate(year float64) *PlainDate {
	isoYear := toIntegerWithTruncation(year)
	if md.cal == "roc" {
		isoYear += 1911
	}
	d := clampFloat(float64(md.day), 1, float64(isoDaysInMonth(int(isoYear), md.month)))
	if !isoDateWithinLimits(int(isoYear), md.month, int(d)) {
		Throw(NewRangeError(FromGoString("Temporal.PlainDate is outside the representable range")))
	}
	return &PlainDate{year: int(isoYear), month: md.month, day: int(d), cal: md.cal}
}

// Duration is bento's runtime representation of a Temporal.Duration (Temporal §7):
// a span of time as ten independent components, from years down to nanoseconds, with
// no anchor to a point on the timeline. It carries no calendar and no zone; it is a
// bag of signed integer counts that all share one sign. The fields are stored as the
// float64s ToIntegerIfIntegral validated, which is exactly what the JS getters return,
// and every rendering recomputes from them.
//
// This slice hosts the shape of a Duration and the arithmetic that needs no reference
// point: construction with the sign and range rules, the ten field getters, sign and
// blank, negated and abs, toString and toJSON, and from over a Duration. The methods
// that balance or round across units (round, total, add, subtract, with, compare over
// mixed calendar units, and from over a string or a property bag) each need a
// relativeTo reference and the calendar model, so they hand back at lowering and are a
// later slice.
type Duration struct {
	years        float64
	months       float64
	weeks        float64
	days         float64
	hours        float64
	minutes      float64
	seconds      float64
	milliseconds float64
	microseconds float64
	nanoseconds  float64
}

// durationUnitLimit is the exclusive bound on the absolute value of the years, months,
// and weeks fields: each must be strictly less than 2^32, the limit IsValidDuration
// fixes for the calendar units.
const durationUnitLimit = 1 << 32

// NewDuration builds a Duration from the constructor's ten optional number arguments,
// every one defaulting to zero. It runs ToIntegerIfIntegral on each, so a fractional,
// NaN, or non-finite component throws a RangeError (unlike PlainDate and PlainTime, a
// Duration does not truncate a fractional argument, it rejects it), then RejectDuration,
// so a mixed-sign set or an out-of-range magnitude throws a RangeError, the order
// new Temporal.Duration(...) follows in the specification. The lowerer pads the missing
// trailing components with zero before the call, so this constructor always sees ten
// numbers.
func NewDuration(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds float64) *Duration {
	y := toIntegerIfIntegral(years)
	mo := toIntegerIfIntegral(months)
	w := toIntegerIfIntegral(weeks)
	d := toIntegerIfIntegral(days)
	h := toIntegerIfIntegral(hours)
	mi := toIntegerIfIntegral(minutes)
	s := toIntegerIfIntegral(seconds)
	ms := toIntegerIfIntegral(milliseconds)
	us := toIntegerIfIntegral(microseconds)
	ns := toIntegerIfIntegral(nanoseconds)
	rejectDuration(y, mo, w, d, h, mi, s, ms, us, ns)
	return &Duration{y, mo, w, d, h, mi, s, ms, us, ns}
}

// DurationFrom implements Temporal.Duration.from for a Duration argument: it returns a
// fresh Duration with the same fields, the copy the specification makes so the result is
// a distinct object equal to its source. from over a string or a property bag hands back
// at lowering, so this is only reached with a Duration in hand.
func DurationFrom(d *Duration) *Duration {
	c := *d
	return &c
}

// DurationFromString implements Temporal.Duration.from over a string. It parses the ISO 8601
// duration grammar, PnYnMnWnDTnHnMnS with an optional leading sign, where only the smallest
// present time component may carry a fraction that cascades into the finer fields down to
// nanoseconds. A grammar the parser rejects, an empty duration with no component, or a field
// out of the valid Duration range each throws a RangeError.
func DurationFromString(s string) *Duration {
	p, ok := parseTemporalDurationString(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.Duration")))
	}
	rejectDuration(p.years, p.months, p.weeks, p.days, p.hours, p.minutes, p.seconds, p.milliseconds, p.microseconds, p.nano)
	return &Duration{p.years, p.months, p.weeks, p.days, p.hours, p.minutes, p.seconds, p.milliseconds, p.microseconds, p.nano}
}

// With implements Temporal.Duration.prototype.with: it overlays the present fields of a
// partial-duration bag onto the receiver, each absent field keeping the receiver's value.
// At least one field must be present, matching ToTemporalPartialDurationRecord, or a TypeError
// is thrown; NewDuration then runs ToIntegerIfIntegral and RejectDuration over the merged ten,
// so a fractional or mixed-sign field throws a RangeError. with does no balancing, it only
// reshapes, so it needs no relativeTo reference.
func (d *Duration) With(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds Opt[float64]) *Duration {
	if !anyDurationFieldPresent(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds) {
		Throw(NewTypeError(FromGoString("Temporal.Duration.prototype.with needs at least one duration field")))
	}
	pick := func(o Opt[float64], base float64) float64 {
		if o.IsUndefined() {
			return base
		}
		return o.Get()
	}
	return NewDuration(
		pick(years, d.years), pick(months, d.months), pick(weeks, d.weeks), pick(days, d.days),
		pick(hours, d.hours), pick(minutes, d.minutes), pick(seconds, d.seconds),
		pick(milliseconds, d.milliseconds), pick(microseconds, d.microseconds), pick(nanoseconds, d.nanoseconds),
	)
}

// DurationFromFields implements Temporal.Duration.from over a property bag: it reads the ten
// optional unit fields, each absent field defaulting to zero. At least one field must be
// present, matching ToTemporalDurationRecord, or a TypeError is thrown; NewDuration then runs
// ToIntegerIfIntegral and RejectDuration over the ten, so a fractional or mixed-sign field
// throws a RangeError. A duration carries no calendar, so the bag needs no calendar gate.
func DurationFromFields(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds Opt[float64]) *Duration {
	if !anyDurationFieldPresent(years, months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds) {
		Throw(NewTypeError(FromGoString("Temporal.Duration.from needs at least one duration field")))
	}
	pick := func(o Opt[float64]) float64 {
		if o.IsUndefined() {
			return 0
		}
		return o.Get()
	}
	return NewDuration(
		pick(years), pick(months), pick(weeks), pick(days),
		pick(hours), pick(minutes), pick(seconds),
		pick(milliseconds), pick(microseconds), pick(nanoseconds),
	)
}

// anyDurationFieldPresent reports whether at least one of the ten optional duration fields is
// present, the precondition both with and from over a bag enforce as a TypeError.
func anyDurationFieldPresent(fields ...Opt[float64]) bool {
	for _, f := range fields {
		if !f.IsUndefined() {
			return true
		}
	}
	return false
}

// Add implements Temporal.Duration.prototype.add, and Subtract implements subtract, which is
// add over a negated operand. The reduced Temporal profile drops the relativeTo option from
// both, so neither can balance calendar units: if the receiver or the operand carries years,
// months, or weeks, a RangeError is thrown. Otherwise the two durations fold to one signed
// nanosecond count over a fixed 24-hour day and re-balance to the coarser of their two default
// largest units, days when either counts whole days and the largest time unit present
// otherwise.
func (d *Duration) Add(other *Duration) *Duration {
	return addDurations(d, other)
}

// Subtract implements Temporal.Duration.prototype.subtract as add over a negated operand.
func (d *Duration) Subtract(other *Duration) *Duration {
	return addDurations(d, other.Negated())
}

func addDurations(a, b *Duration) *Duration {
	if a.years != 0 || a.months != 0 || a.weeks != 0 || b.years != 0 || b.months != 0 || b.weeks != 0 {
		Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.add and subtract cannot balance years, months, or weeks without a relativeTo reference, which they do not accept")))
	}
	total := new(big.Int).Add(durationDayTimeNanos(a), durationDayTimeNanos(b))
	rank := a.defaultLargestRank()
	if r := b.defaultLargestRank(); r < rank {
		rank = r
	}
	return balanceDayTimeNanos(total, rank)
}

// defaultLargestRank returns the rank of a Duration's coarsest non-zero field on the scale
// addDurations balances against, day counting as -1 and hour through nanosecond as 0 through 5
// to match durationFromDayNanos. An all-zero duration reports nanosecond, the finest, so two
// empty durations fold to an empty result. Only the day and time fields are consulted, since
// addDurations rejects any duration that carries years, months, or weeks.
func (d *Duration) defaultLargestRank() int {
	switch {
	case d.days != 0:
		return -1
	case d.hours != 0:
		return 0
	case d.minutes != 0:
		return 1
	case d.seconds != 0:
		return 2
	case d.milliseconds != 0:
		return 3
	case d.microseconds != 0:
		return 4
	default:
		return 5
	}
}

// durationDayTimeNanos folds a Duration's days and six time fields into one signed nanosecond
// count, each day nsPerDay over a fixed 24-hour day. The years, months, and weeks fields are
// not consulted; addDurations rejects a duration that carries them.
func durationDayTimeNanos(d *Duration) *big.Int {
	total := bigMulInt(d.days, 86_400_000_000_000)
	total.Add(total, durationTimeNanos(d))
	return total
}

// balanceDayTimeNanos splits a signed nanosecond count into a Duration whose coarsest field is
// fixed by rank, day at -1 and hour through nanosecond at 0 through 5. At day rank the whole
// days come off first at nsPerDay each and the sub-day remainder balances across the six time
// fields; at a time rank durationFromDayNanos balances the whole count with no day field.
func balanceDayTimeNanos(total *big.Int, rank int) *Duration {
	if rank > -1 {
		return durationFromDayNanos(total, rank)
	}
	days := new(big.Int).Quo(total, nsPerDay)
	rem := new(big.Int).Rem(total, nsPerDay)
	t := durationFromDayNanos(rem, 0)
	t.days = float64(days.Int64())
	return t
}

// Total implements Temporal.Duration.prototype.total. unit names the output unit, already
// normalized to its singular form, and rel is the PlainDate the calendar units resolve against,
// or nil when no relativeTo was given. Without a reference the duration may carry no years,
// months, or weeks and unit must be day or finer, since week, month, and year each need a
// calendar, else a RangeError; a day then counts as a fixed 24 hours. With a reference every
// field resolves against the calendar: the date part lands on an end date, the sub-day time
// adds over a fixed 24-hour day, and the signed nanosecond span from rel to that endpoint
// converts to unit. Days and weeks are fixed lengths that divide directly; months and years
// vary, so the fraction interpolates between the two unit boundaries that bracket the endpoint.
func (d *Duration) Total(unit string, rel *PlainDate) float64 {
	if rel == nil {
		if d.years != 0 || d.months != 0 || d.weeks != 0 || unit == "week" || unit == "month" || unit == "year" {
			Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.total needs a relativeTo reference for years, months, weeks, or a calendar unit")))
		}
		return ratToFloat(durationDayTimeNanos(d), durationUnitNanos(unit))
	}
	destNs, target := durationReferenceEndpoint(d, rel)
	if unit == "year" || unit == "month" {
		return durationTotalCalendar(rel, target, destNs, unit)
	}
	return ratToFloat(destNs, durationUnitNanos(unit))
}

// durationUnitNanos returns the nanosecond length of a fixed Temporal unit: nsPerDay for a day,
// seven of those for a week, and the time-unit table for hour through nanosecond. It is called
// only for the units that hold a constant length, month and year being interpolated instead.
func durationUnitNanos(unit string) *big.Int {
	switch unit {
	case "day":
		return new(big.Int).Set(nsPerDay)
	case "week":
		return new(big.Int).Mul(nsPerDay, big.NewInt(7))
	default:
		unitNs, _, _ := plainTimeUnitInfo(unit)
		return big.NewInt(unitNs)
	}
}

// ratToFloat divides num by den as an exact rational and returns the nearest float64, the
// rounding the specification's total performs when it converts the balanced count to a Number.
func ratToFloat(num, den *big.Int) float64 {
	f, _ := new(big.Rat).SetFrac(num, den).Float64()
	return f
}

// durationReferenceEndpoint resolves a Duration against a PlainDate anchor into the end date the
// date part reaches and the signed nanosecond span from the anchor to the endpoint. The time
// part folds its whole 24-hour days into the date, truncated toward zero, and the sub-day
// remainder joins the span, so the endpoint is a real calendar date and the remainder stays
// under a day. The years, months, weeks, and days add through addISODate under constrain.
func durationReferenceEndpoint(d *Duration, rel *PlainDate) (*big.Int, *PlainDate) {
	timeNs := durationTimeNanos(d)
	dayCarry := new(big.Int).Quo(timeNs, nsPerDay)
	residual := new(big.Int).Rem(timeNs, nsPerDay)
	days := new(big.Int).Add(big.NewInt(int64(d.days)), dayCarry)
	y, m, dd := addISODate(rel.year, rel.month, rel.day, int(d.years), int(d.months), int(d.weeks), days, "constrain")
	if !isoDateWithinLimits(y, m, dd) {
		Throw(NewRangeError(FromGoString("Temporal.Duration relativeTo arithmetic is outside the representable range")))
	}
	target := &PlainDate{year: y, month: m, day: dd, cal: rel.cal}
	span := new(big.Int).Mul(big.NewInt(int64(isoToEpochDays(y, m, dd)-isoToEpochDays(rel.year, rel.month, rel.day))), nsPerDay)
	span.Add(span, residual)
	return span, target
}

// durationTotalCalendar interpolates the fractional count of an irregular unit, month or year,
// from a PlainDate anchor to the endpoint. It takes the whole count of the unit that fits, then
// measures how far the endpoint sits between that unit boundary and the next, both boundaries
// found by stepping the anchor whole units so their varying lengths are exact. The two spans
// share the duration's sign, so the fraction is non-negative and carries the sign back.
func durationTotalCalendar(rel, target *PlainDate, destNs *big.Int, unit string) float64 {
	if destNs.Sign() == 0 {
		return 0
	}
	years, months, _, _ := differenceISODate(rel.year, rel.month, rel.day, target.year, target.month, target.day, unit)
	wholeU := months
	if unit == "year" {
		wholeU = years
	}
	s := destNs.Sign()
	startNs := durationUnitBoundaryNanos(rel, unit, wholeU)
	endNs := durationUnitBoundaryNanos(rel, unit, wholeU+s)
	num := new(big.Int).Sub(destNs, startNs)
	den := new(big.Int).Sub(endNs, startNs)
	return float64(wholeU) + float64(s)*ratToFloat(num, den)
}

// durationUnitBoundaryNanos returns the nanosecond span from rel to the date reached by stepping
// it n whole units of unit, month or year, under constrain. durationTotalCalendar brackets the
// endpoint between two such boundaries to size the varying unit exactly.
func durationUnitBoundaryNanos(rel *PlainDate, unit string, n int) *big.Int {
	years, months := 0, n
	if unit == "year" {
		years, months = n, 0
	}
	y, m, dd := addISODate(rel.year, rel.month, rel.day, years, months, 0, big.NewInt(0), "constrain")
	return new(big.Int).Mul(big.NewInt(int64(isoToEpochDays(y, m, dd)-isoToEpochDays(rel.year, rel.month, rel.day))), nsPerDay)
}

// DurationCompare implements Temporal.Duration.compare. rel is the PlainDate the calendar units
// resolve against, or nil when no relativeTo was given. Without a reference neither operand may
// carry years, months, or weeks, else a RangeError, and each folds to a signed nanosecond count
// over a fixed 24-hour day; with a reference each resolves against the calendar to an endpoint.
// The result is the sign of the first span minus the second, -1, 0, or 1.
func DurationCompare(a, b *Duration, rel *PlainDate) float64 {
	var spanA, spanB *big.Int
	if rel == nil {
		if a.years != 0 || a.months != 0 || a.weeks != 0 || b.years != 0 || b.months != 0 || b.weeks != 0 {
			Throw(NewRangeError(FromGoString("Temporal.Duration.compare needs a relativeTo reference to compare years, months, or weeks")))
		}
		spanA = durationDayTimeNanos(a)
		spanB = durationDayTimeNanos(b)
	} else {
		spanA, _ = durationReferenceEndpoint(a, rel)
		spanB, _ = durationReferenceEndpoint(b, rel)
	}
	return float64(spanA.Cmp(spanB))
}

// durationUnitRank orders the ten Duration units from coarsest to finest, year 0 through
// nanosecond 9, so a smaller rank is a coarser unit. Round compares smallestUnit and
// largestUnit by this order and tests whether a unit sits at day or finer, rank 3 or above.
func durationUnitRank(unit string) int {
	switch unit {
	case "year":
		return 0
	case "month":
		return 1
	case "week":
		return 2
	case "day":
		return 3
	case "hour":
		return 4
	case "minute":
		return 5
	case "second":
		return 6
	case "millisecond":
		return 7
	case "microsecond":
		return 8
	case "nanosecond":
		return 9
	}
	Throw(NewRangeError(FromGoString("Temporal.Duration unit is invalid")))
	return 0
}

// defaultLargestUnitName returns the coarsest non-zero field of the duration as a unit name,
// the largestUnit round and total default to when none is given. An all-zero duration reports
// nanosecond, the finest.
func (d *Duration) defaultLargestUnitName() string {
	switch {
	case d.years != 0:
		return "year"
	case d.months != 0:
		return "month"
	case d.weeks != 0:
		return "week"
	case d.days != 0:
		return "day"
	case d.hours != 0:
		return "hour"
	case d.minutes != 0:
		return "minute"
	case d.seconds != 0:
		return "second"
	case d.milliseconds != 0:
		return "millisecond"
	case d.microseconds != 0:
		return "microsecond"
	default:
		return "nanosecond"
	}
}

// coarserDurationUnit returns whichever of the two units is coarser, the one with the smaller
// rank. Round resolves an unset largestUnit to the coarser of the duration's default largest
// unit and the smallestUnit, so the result never balances finer than requested.
func coarserDurationUnit(a, b string) string {
	if durationUnitRank(a) <= durationUnitRank(b) {
		return a
	}
	return b
}

// Round implements Temporal.Duration.prototype.round. smallestUnit and largestUnit are the
// singular unit names, either empty when the option was absent; smallestUnit then defaults to
// nanosecond and largestUnit to the coarser of the duration's default largest unit and the
// smallestUnit. rel is the PlainDate the calendar units resolve against, or nil when no
// relativeTo was given. Without a reference the duration may carry no years, months, or weeks
// and both units must be day or finer, since week, month, and year each need a calendar, else
// a RangeError; the duration then rounds over a fixed 24-hour day and balances to largestUnit.
// With a reference every field resolves against the calendar to an endpoint, the endpoint
// rounds at smallestUnit, and the rounded date rebalances to largestUnit. An irregular
// smallestUnit, month or year, rounds by bracketing the endpoint between two unit boundaries;
// a fixed one rounds the nanosecond span and splits it back into whole days and a remainder.
func (d *Duration) Round(smallestUnit, largestUnit string, increment float64, mode string, rel *PlainDate) *Duration {
	sm := smallestUnit
	if sm == "" {
		sm = "nanosecond"
	}
	lg := largestUnit
	if lg == "" || lg == "auto" {
		lg = coarserDurationUnit(d.defaultLargestUnitName(), sm)
	}
	smRank := durationUnitRank(sm)
	lgRank := durationUnitRank(lg)
	if lgRank > smRank {
		Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.round largestUnit cannot be smaller than smallestUnit")))
	}
	inc := int(toIntegerWithTruncation(increment))
	if inc < 1 {
		Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.round roundingIncrement must be a positive integer")))
	}
	if smRank >= 4 {
		_, dividend, _ := plainTimeUnitInfo(sm)
		if int64(inc) >= dividend || dividend%int64(inc) != 0 {
			Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.round roundingIncrement is out of range")))
		}
	}
	if rel == nil {
		if d.years != 0 || d.months != 0 || d.weeks != 0 || smRank < 3 || lgRank < 3 {
			Throw(NewRangeError(FromGoString("Temporal.Duration.prototype.round needs a relativeTo reference for years, months, weeks, or a calendar unit")))
		}
		quantum := new(big.Int).Mul(big.NewInt(int64(inc)), durationUnitNanos(sm))
		roundedNs := roundBigToIncrement(durationDayTimeNanos(d), quantum, mode)
		return balanceDayTimeNanos(roundedNs, fixedLargeRank(lg))
	}
	destNs, target := durationReferenceEndpoint(d, rel)
	var roundedDate *PlainDate
	residual := big.NewInt(0)
	if sm == "year" || sm == "month" {
		roundedDate = roundDurationCalendarUnit(rel, target, destNs, sm, inc, mode)
	} else {
		quantum := new(big.Int).Mul(big.NewInt(int64(inc)), durationUnitNanos(sm))
		roundedNs := roundBigToIncrement(destNs, quantum, mode)
		dayTrunc := new(big.Int).Quo(roundedNs, nsPerDay)
		residual = new(big.Int).Rem(roundedNs, nsPerDay)
		y, m, dd := addISODate(rel.year, rel.month, rel.day, 0, 0, 0, dayTrunc, "constrain")
		roundedDate = &PlainDate{year: y, month: m, day: dd, cal: rel.cal}
	}
	return rebalanceRoundedDuration(rel, roundedDate, residual, lg)
}

// fixedLargeRank maps a largestUnit for the no-reference path, always day or finer, to the
// rank balanceDayTimeNanos expects: day at -1 so its whole days come off first, and hour
// through nanosecond at 0 through 5.
func fixedLargeRank(unit string) int {
	if unit == "day" {
		return -1
	}
	_, _, rank := plainTimeUnitInfo(unit)
	return rank
}

// roundDurationCalendarUnit rounds an endpoint to a whole count of an irregular unit, month or
// year, from the anchor rel. It brackets the endpoint between the boundary at the count that
// fits toward zero, rounded down to a multiple of increment, and the next one increment away,
// then rounds the position between them under mode. The mode flips ceil against floor for a
// negative duration so the direction stays in wall-clock terms, and a half-even tie picks the
// even count. It returns the date the rounded count reaches, stepped under constrain.
func roundDurationCalendarUnit(rel, target *PlainDate, destNs *big.Int, unit string, increment int, mode string) *PlainDate {
	if destNs.Sign() == 0 {
		return &PlainDate{year: rel.year, month: rel.month, day: rel.day, cal: rel.cal}
	}
	years, months, _, _ := differenceISODate(rel.year, rel.month, rel.day, target.year, target.month, target.day, unit)
	wholeU := months
	if unit == "year" {
		wholeU = years
	}
	s := destNs.Sign()
	mag := wholeU
	if mag < 0 {
		mag = -mag
	}
	lowMag := (mag / increment) * increment
	low, high := lowMag, lowMag+increment
	if s < 0 {
		low, high = -low, -high
	}
	startNs := durationUnitBoundaryNanos(rel, unit, low)
	endNs := durationUnitBoundaryNanos(rel, unit, high)
	pos := new(big.Int).Abs(new(big.Int).Sub(destNs, startNs))
	span := new(big.Int).Abs(new(big.Int).Sub(endNs, startNs))
	count := low
	if roundBigToIncrement(pos, span, flipModeForSign(mode, s)).Cmp(span) == 0 {
		count = high
	}
	if mode == "halfEven" && new(big.Int).Lsh(pos, 1).Cmp(span) == 0 {
		if low%2 == 0 {
			count = low
		} else {
			count = high
		}
	}
	years2, months2 := 0, count
	if unit == "year" {
		years2, months2 = count, 0
	}
	y, m, dd := addISODate(rel.year, rel.month, rel.day, years2, months2, 0, big.NewInt(0), "constrain")
	return &PlainDate{year: y, month: m, day: dd, cal: rel.cal}
}

// flipModeForSign flips the direction of a rounding mode for a negative duration, so a mode
// stated in wall-clock terms rounds the magnitude the roundDurationCalendarUnit bracket works
// in. Ceil trades with floor and half-ceil with half-floor; the sign-symmetric modes, expand,
// trunc, their half forms, and half-even, are unchanged.
func flipModeForSign(mode string, sign int) string {
	if sign >= 0 {
		return mode
	}
	switch mode {
	case "ceil":
		return "floor"
	case "floor":
		return "ceil"
	case "halfCeil":
		return "halfFloor"
	case "halfFloor":
		return "halfCeil"
	}
	return mode
}

// rebalanceRoundedDuration re-expresses the rounded date and its sub-day remainder as a
// Duration balanced to largestUnit from the anchor rel. A largestUnit of day or coarser splits
// the date difference into calendar fields through differenceISODate and balances the remainder
// across the time fields below a day; a time largestUnit folds the whole span, the date's days
// plus the remainder, into the time fields with no date part.
func rebalanceRoundedDuration(rel, roundedDate *PlainDate, residual *big.Int, largestUnit string) *Duration {
	if durationUnitRank(largestUnit) <= 3 {
		y, mo, w, dd := differenceISODate(rel.year, rel.month, rel.day, roundedDate.year, roundedDate.month, roundedDate.day, largestUnit)
		t := durationFromDayNanos(residual, 0)
		return NewDuration(float64(y), float64(mo), float64(w), float64(dd), t.hours, t.minutes, t.seconds, t.milliseconds, t.microseconds, t.nanoseconds)
	}
	dayDiff := isoToEpochDays(roundedDate.year, roundedDate.month, roundedDate.day) - isoToEpochDays(rel.year, rel.month, rel.day)
	totalNs := new(big.Int).Mul(big.NewInt(int64(dayDiff)), nsPerDay)
	totalNs.Add(totalNs, residual)
	_, _, rank := plainTimeUnitInfo(largestUnit)
	return durationFromDayNanos(totalNs, rank)
}

// toIntegerIfIntegral implements the abstract operation ToIntegerIfIntegral (Temporal):
// a NaN, non-finite, or non-integral value throws a RangeError, and an integral value is
// returned unchanged. It is the gate Temporal.Duration uses on every field, and it
// differs from ToIntegerWithTruncation in that it rejects a fractional argument rather
// than truncating it: new Temporal.Duration(1.5) raises a RangeError. A negative zero is
// normalized to positive zero so it counts as zero in the sign rules and never renders
// with a stray minus.
func toIntegerIfIntegral(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || math.Trunc(x) != x {
		Throw(NewRangeError(FromGoString("Temporal.Duration component must be an integer")))
	}
	if x == 0 {
		return 0
	}
	return x
}

// rejectDuration throws a RangeError unless the ten fields form a valid Duration under
// IsValidDuration: every non-zero field shares one sign, the years, months, and weeks
// each have absolute value below 2^32, and the day-and-below fields together stay within
// the representable range, a total-seconds magnitude below 2^53. The arguments are the
// integral float64s ToIntegerIfIntegral returned.
func rejectDuration(y, mo, w, d, h, mi, s, ms, us, ns float64) {
	if !durationSignsConsistent(y, mo, w, d, h, mi, s, ms, us, ns) {
		Throw(NewRangeError(FromGoString("Temporal.Duration fields must all share one sign")))
	}
	if math.Abs(y) >= durationUnitLimit || math.Abs(mo) >= durationUnitLimit || math.Abs(w) >= durationUnitLimit {
		Throw(NewRangeError(FromGoString("Temporal.Duration years, months, or weeks is out of range")))
	}
	if durationSecondsOverflow(d, h, mi, s, ms, us, ns) {
		Throw(NewRangeError(FromGoString("Temporal.Duration is out of range")))
	}
}

// durationSignsConsistent reports whether every non-zero field carries the same sign, the
// rule a valid Duration obeys. A field of zero (including a normalized negative zero)
// belongs to either sign and is skipped.
func durationSignsConsistent(fields ...float64) bool {
	sign := 0
	for _, v := range fields {
		switch {
		case v < 0:
			if sign > 0 {
				return false
			}
			sign = -1
		case v > 0:
			if sign < 0 {
				return false
			}
			sign = 1
		}
	}
	return true
}

// durationSecondsOverflow reports whether the day-and-below fields exceed the
// representable range: their normalized total seconds, days times 86,400 plus the hours,
// minutes, and seconds plus the floored whole-second part of each sub-second field, must
// have absolute value below 2^53. It works in big.Int so no intermediate product loses
// precision at the boundary, the one place a float64 sum could round across it.
func durationSecondsOverflow(d, h, mi, s, ms, us, ns float64) bool {
	total := new(big.Int)
	total.Add(total, bigMulInt(d, 86400))
	total.Add(total, bigMulInt(h, 3600))
	total.Add(total, bigMulInt(mi, 60))
	total.Add(total, big.NewInt(int64(s)))
	total.Add(total, bigFloorDiv(ms, 1000))
	total.Add(total, bigFloorDiv(us, 1_000_000))
	total.Add(total, bigFloorDiv(ns, 1_000_000_000))
	total.Abs(total)
	limit := new(big.Int).Lsh(big.NewInt(1), 53)
	return total.Cmp(limit) >= 0
}

// bigMulInt returns the exact product of an integral float64 and an int64 multiplier as
// a big.Int. The float64 is below 2^53 in magnitude, so int64(x) is exact.
func bigMulInt(x float64, m int64) *big.Int {
	n := big.NewInt(int64(x))
	return n.Mul(n, big.NewInt(m))
}

// bigFloorDiv returns floor(x / div) for an integral float64 and a positive int64
// divisor. big.Int.Div is Euclidean division, which equals the floor for a positive
// divisor, so this matches the specification's floor over signed inputs.
func bigFloorDiv(x float64, div int64) *big.Int {
	n := big.NewInt(int64(x))
	return n.Div(n, big.NewInt(div))
}

// durationSign returns the sign of the whole Duration: 1 if the first non-zero field is
// positive, -1 if it is negative, 0 if every field is zero. Because the fields share one
// sign, the first non-zero field decides it.
func durationSign(d *Duration) int {
	for _, v := range []float64{d.years, d.months, d.weeks, d.days, d.hours, d.minutes, d.seconds, d.milliseconds, d.microseconds, d.nanoseconds} {
		if v > 0 {
			return 1
		}
		if v < 0 {
			return -1
		}
	}
	return 0
}

// Years returns the years field.
func (d *Duration) Years() float64 { return d.years }

// Months returns the months field.
func (d *Duration) Months() float64 { return d.months }

// Weeks returns the weeks field.
func (d *Duration) Weeks() float64 { return d.weeks }

// Days returns the days field.
func (d *Duration) Days() float64 { return d.days }

// Hours returns the hours field.
func (d *Duration) Hours() float64 { return d.hours }

// Minutes returns the minutes field.
func (d *Duration) Minutes() float64 { return d.minutes }

// Seconds returns the seconds field.
func (d *Duration) Seconds() float64 { return d.seconds }

// Milliseconds returns the milliseconds field.
func (d *Duration) Milliseconds() float64 { return d.milliseconds }

// Microseconds returns the microseconds field.
func (d *Duration) Microseconds() float64 { return d.microseconds }

// Nanoseconds returns the nanoseconds field.
func (d *Duration) Nanoseconds() float64 { return d.nanoseconds }

// Sign returns the sign of the whole Duration, 1, -1, or 0.
func (d *Duration) Sign() float64 { return float64(durationSign(d)) }

// Blank reports whether the Duration is all zeros, the case where sign is 0.
func (d *Duration) Blank() bool { return durationSign(d) == 0 }

// Negated implements Temporal.Duration.prototype.negated: a Duration with every field's
// sign flipped. A zero field stays a positive zero.
func (d *Duration) Negated() *Duration {
	return &Duration{
		negateField(d.years), negateField(d.months), negateField(d.weeks), negateField(d.days),
		negateField(d.hours), negateField(d.minutes), negateField(d.seconds),
		negateField(d.milliseconds), negateField(d.microseconds), negateField(d.nanoseconds),
	}
}

// Abs implements Temporal.Duration.prototype.abs: a Duration with every field made
// non-negative.
func (d *Duration) Abs() *Duration {
	return &Duration{
		math.Abs(d.years), math.Abs(d.months), math.Abs(d.weeks), math.Abs(d.days),
		math.Abs(d.hours), math.Abs(d.minutes), math.Abs(d.seconds),
		math.Abs(d.milliseconds), math.Abs(d.microseconds), math.Abs(d.nanoseconds),
	}
}

// negateField flips the sign of a field, mapping a zero to a positive zero so a negated
// empty component never renders with a stray minus.
func negateField(x float64) float64 {
	if x == 0 {
		return 0
	}
	return -x
}

// ToString implements Temporal.Duration.prototype.toString for the default options: the
// ISO 8601 duration form, an optional leading minus for a negative Duration, then P, the
// non-zero date components (years, months, weeks, days), then T and the non-zero time
// components (hours, minutes, and a combined seconds field). The seconds field folds the
// seconds, milliseconds, microseconds, and nanoseconds into one decimal with the
// fraction trimmed of trailing zeros. An all-zero Duration renders as "PT0S".
func (d *Duration) ToString() BStr {
	var b strings.Builder
	if durationSign(d) < 0 {
		b.WriteByte('-')
	}
	b.WriteByte('P')
	appendDurationField(&b, d.years, 'Y')
	appendDurationField(&b, d.months, 'M')
	appendDurationField(&b, d.weeks, 'W')
	appendDurationField(&b, d.days, 'D')
	hasHours := d.hours != 0
	hasMinutes := d.minutes != 0
	secStr, hasSeconds := durationSecondsString(d)
	if hasHours || hasMinutes || hasSeconds {
		b.WriteByte('T')
		if hasHours {
			b.WriteString(durationAbsInt(d.hours))
			b.WriteByte('H')
		}
		if hasMinutes {
			b.WriteString(durationAbsInt(d.minutes))
			b.WriteByte('M')
		}
		if hasSeconds {
			b.WriteString(secStr)
			b.WriteByte('S')
		}
	}
	return FromGoString(b.String())
}

// ToJSON implements Temporal.Duration.prototype.toJSON, the same ISO string toString
// produces under default options.
func (d *Duration) ToJSON() BStr { return d.ToString() }

// appendDurationField writes a non-zero date component as its absolute value followed by
// the unit letter, and writes nothing for a zero component.
func appendDurationField(b *strings.Builder, v float64, unit byte) {
	if v == 0 {
		return
	}
	b.WriteString(durationAbsInt(v))
	b.WriteByte(unit)
}

// durationAbsInt renders the absolute value of an integral float64 as a decimal. Every
// field is below 2^53 in magnitude, so int64 holds it exactly.
func durationAbsInt(v float64) string {
	n := int64(v)
	if n < 0 {
		n = -n
	}
	return strconv.FormatInt(n, 10)
}

// durationSecondsString folds the seconds, milliseconds, microseconds, and nanoseconds
// into a single decimal seconds value with the fractional part trimmed of trailing
// zeros, and reports whether the seconds component should appear at all. It appears when
// any of the four fields is non-zero, and also for an all-zero Duration so the rendering
// is "PT0S". The fold runs in big.Int because seconds times a billion can exceed both
// int64 and the exact float64 range.
func durationSecondsString(d *Duration) (string, bool) {
	if d.seconds == 0 && d.milliseconds == 0 && d.microseconds == 0 && d.nanoseconds == 0 && durationSign(d) != 0 {
		return "", false
	}
	total := new(big.Int)
	total.Add(total, bigMulInt(d.seconds, 1_000_000_000))
	total.Add(total, bigMulInt(d.milliseconds, 1_000_000))
	total.Add(total, bigMulInt(d.microseconds, 1_000))
	total.Add(total, big.NewInt(int64(d.nanoseconds)))
	total.Abs(total)
	whole := new(big.Int)
	frac := new(big.Int)
	whole.QuoRem(total, big.NewInt(1_000_000_000), frac)
	s := whole.String()
	if frac.Sign() != 0 {
		s += "." + strings.TrimRight(zeroPad(int(frac.Int64()), 9), "0")
	}
	return s, true
}

// twoDigit renders a month or day as exactly two digits.
func twoDigit(n int) string { return zeroPad(n, 2) }

// Instant is bento's runtime representation of a Temporal.Instant (Temporal §8): an
// exact point on the UTC time line, counted as a whole number of nanoseconds since the
// epoch 1970-01-01T00:00:00Z. It carries no calendar and no zone, only the count, so a
// single arbitrary-precision integer captures it; the nanosecond total runs to ±8.64e21,
// past a float64's exact-integer range, so it is a big.Int rather than a double.
//
// The stored count is validated against the representable range at construction, so an
// Instant that reached a getter or a comparison is always in range. The value is copied
// in and copied out, so a caller cannot mutate the shared big.Int and reach through to
// the Instant's field.
type Instant struct {
	ns *big.Int // nanoseconds since the epoch, in [nsMinInstant, nsMaxInstant]
}

// The Instant range bounds and the two divisors the field math leans on. The maximum is
// 8.64e21 nanoseconds, 10^8 days of nanoseconds each side of the epoch, the range
// Temporal fixes for an exact time; nsPerDay and nsPerMilli split the count into a day
// index and a within-day remainder, and into whole milliseconds, for the getters and the
// string rendering.
var (
	nsMaxInstant, _ = new(big.Int).SetString("8640000000000000000000", 10)
	nsMinInstant    = new(big.Int).Neg(nsMaxInstant)
	nsPerDay        = big.NewInt(86_400_000_000_000)
	nsPerMilli      = big.NewInt(1_000_000)
)

// validateEpochNanoseconds throws a RangeError unless ns is within the Instant range, the
// IsValidEpochNanoseconds gate the constructor and both epoch factories run before they
// build an Instant.
func validateEpochNanoseconds(ns *big.Int) {
	if ns.Cmp(nsMinInstant) < 0 || ns.Cmp(nsMaxInstant) > 0 {
		Throw(NewRangeError(FromGoString("Temporal.Instant is outside the representable range")))
	}
}

// newInstant validates a nanosecond count and stores a copy, the shared body of the
// constructor and the epoch factories. The copy means the big.Int a caller passes in
// stays independent of the Instant's field, so a later mutation of the argument cannot
// change the Instant.
func newInstant(ns *big.Int) *Instant {
	validateEpochNanoseconds(ns)
	return &Instant{ns: new(big.Int).Set(ns)}
}

// NewInstant builds an Instant from the constructor's single bigint argument, the
// nanoseconds since the epoch, running IsValidEpochNanoseconds so an out-of-range count
// throws a RangeError the way new Temporal.Instant(ns) does.
func NewInstant(epochNanoseconds *big.Int) *Instant {
	return newInstant(epochNanoseconds)
}

// InstantFromEpochNanoseconds implements Temporal.Instant.fromEpochNanoseconds: it is the
// constructor under another name, a bigint nanosecond count validated and stored.
func InstantFromEpochNanoseconds(epochNanoseconds *big.Int) *Instant {
	return newInstant(epochNanoseconds)
}

// InstantFromEpochMilliseconds implements Temporal.Instant.fromEpochMilliseconds: the
// number of milliseconds must be an integer, so a NaN, non-finite, or fractional value
// throws a RangeError (the NumberToBigInt step the specification runs), then the count is
// scaled to nanoseconds and validated against the Instant range. A whole millisecond
// count up to the range bound stays inside a float64's exact-integer range, so the int64
// narrowing is lossless.
func InstantFromEpochMilliseconds(epochMilliseconds float64) *Instant {
	if math.IsNaN(epochMilliseconds) || math.IsInf(epochMilliseconds, 0) || epochMilliseconds != math.Trunc(epochMilliseconds) {
		Throw(NewRangeError(FromGoString("Temporal.Instant epoch milliseconds must be an integer")))
	}
	ns := new(big.Int).SetInt64(int64(epochMilliseconds))
	ns.Mul(ns, nsPerMilli)
	return newInstant(ns)
}

// InstantFrom implements Temporal.Instant.from for an Instant argument: it returns a fresh
// Instant with the same count, the copy the specification makes. from over a string routes
// to InstantFromString instead, so this is reached only with an Instant in hand.
func InstantFrom(inst *Instant) *Instant {
	return newInstant(inst.ns)
}

// InstantFromString implements Temporal.Instant.from over a string. An Instant is an exact
// point on the UTC time line, so the string must fix the offset from UTC: a Z designator or a
// numeric offset is required, and a date-only or offset-less date-time string throws a
// RangeError. The wall-clock reading the date and time name is taken as UTC and the offset is
// subtracted to reach the epoch count, which newInstant range-checks. A calendar annotation is
// accepted and ignored whatever it names, since an Instant carries no calendar; the shared
// parser still rejects a malformed annotation or a critical non-calendar one.
func InstantFromString(s string) *Instant {
	p, ok := parseTemporalISOString(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.Instant")))
	}
	if !p.hasZ && !p.hasOffset {
		Throw(NewRangeError(FromGoString("a Temporal.Instant string requires a UTC offset or a Z designator")))
	}
	secs := int64(isoToEpochDays(p.year, p.month, p.day))*86_400 +
		int64(p.hour)*3600 + int64(p.minute)*60 + int64(p.second)
	ns := new(big.Int).SetInt64(secs)
	ns.Mul(ns, big.NewInt(1_000_000_000))
	sub := int64(p.millisecond)*1_000_000 + int64(p.microsecond)*1_000 + int64(p.nanosecond)
	ns.Add(ns, big.NewInt(sub))
	if p.hasOffset {
		ns.Sub(ns, big.NewInt(p.offsetNanoseconds))
	}
	return newInstant(ns)
}

// EpochNanoseconds returns the nanoseconds since the epoch as a fresh big.Int, so the
// caller holds a bigint independent of the Instant's field.
func (i *Instant) EpochNanoseconds() *big.Int { return new(big.Int).Set(i.ns) }

// EpochMilliseconds returns the whole milliseconds since the epoch, floor(ns / 10^6). The
// floor runs through big.Int Euclidean division, so a negative instant rounds toward minus
// infinity the way the specification's mathematical floor does; the result is within a
// float64's exact-integer range across the whole Instant range.
func (i *Instant) EpochMilliseconds() float64 {
	q := new(big.Int).Div(i.ns, nsPerMilli)
	return float64(q.Int64())
}

// Equals implements Temporal.Instant.prototype.equals for an Instant argument: two
// instants are equal exactly when their nanosecond counts match.
func (i *Instant) Equals(other *Instant) bool { return i.ns.Cmp(other.ns) == 0 }

// AddDuration implements Temporal.Instant.prototype.add: it folds the duration's time part into
// the epoch nanosecond count. An Instant has no calendar and no wall clock, so a nonzero years,
// months, weeks, or days field is meaningless and throws a RangeError, matching the specification's
// rejection of the calendar units. The fold runs in big.Int, so an hour field near the safe-integer
// ceiling does not overflow, and newInstant re-validates the range so a result past the Instant
// bounds throws. subtract reuses this over a negated duration.
func (i *Instant) AddDuration(dur *Duration) *Instant {
	if dur.years != 0 || dur.months != 0 || dur.weeks != 0 || dur.days != 0 {
		Throw(NewRangeError(FromGoString("Temporal.Instant arithmetic does not accept the calendar units years, months, weeks, or days")))
	}
	total := new(big.Int).Add(i.ns, durationTimeNanos(dur))
	return newInstant(total)
}

// Until returns the signed exact-time difference from the receiver to other as a Duration,
// balanced from largestUnit down and rounded at smallestUnit under roundingMode. An Instant
// carries no calendar, so the units run hour down to nanosecond only; a day or larger unit is
// rejected at the boundary before this method by the caller's unit set.
func (i *Instant) Until(other *Instant, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	return instantDifference(i, other, largestUnit, smallestUnit, increment, roundingMode)
}

// Since returns the signed exact-time difference from other to the receiver, the reverse of
// Until. Both round the signed difference so the mode acts on the true sign, matching the
// specification's rule of negating the mode and the result for since.
func (i *Instant) Since(other *Instant, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	return instantDifference(other, i, largestUnit, smallestUnit, increment, roundingMode)
}

// instantDifference is the exact-time analogue of plainTimeDifference: it works over the full
// epoch nanosecond gap rather than a within-day gap, so the same unit lookup, increment check,
// shared rounding, and balancing serve both. The gap can span many hours, and durationFromDayNanos
// rolls the whole gap into the field at largeRank, so largestUnit hour reports 2h15m where the
// default largestUnit second reports the flat second count.
func instantDifference(from, to *Instant, largestUnit, smallestUnit string, increment float64, roundingMode string) *Duration {
	unitNs, dividend, smallRank := plainTimeUnitInfo(smallestUnit)
	_, _, largeRank := plainTimeUnitInfo(largestUnit)
	if largeRank > smallRank {
		Throw(NewRangeError(FromGoString("Temporal.Instant difference largestUnit cannot be smaller than smallestUnit")))
	}
	inc := int64(toIntegerWithTruncation(increment))
	if inc < 1 || inc >= dividend || dividend%inc != 0 {
		Throw(NewRangeError(FromGoString("Temporal.Instant difference roundingIncrement is out of range")))
	}
	diff := new(big.Int).Sub(to.ns, from.ns)
	quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
	diff = roundBigToIncrement(diff, quantum, roundingMode)
	return durationFromDayNanos(diff, largeRank)
}

// Round implements Temporal.Instant.prototype.round: it rounds the epoch nanosecond count to a
// multiple of roundingIncrement of smallestUnit under one of the nine rounding modes. Unlike
// PlainTime.round, whose increment divides the next larger unit, an Instant rounds against the
// whole day, so the increment must divide the number of the unit in a 24-hour day, hour into 24,
// minute into 1440, second into 86400, and each sub-second unit correspondingly. An increment that
// is not a positive integer at or below that count and dividing it throws a RangeError. The quantum
// divides a day evenly, so the rounding aligns to the day boundary. The receiver is unchanged.
func (i *Instant) Round(smallestUnit string, increment float64, roundingMode string) *Instant {
	unitNs, _, _ := plainTimeUnitInfo(smallestUnit)
	maxInc := new(big.Int).Quo(nsPerDay, big.NewInt(unitNs)).Int64()
	inc := int64(toIntegerWithTruncation(increment))
	if inc < 1 || inc > maxInc || maxInc%inc != 0 {
		Throw(NewRangeError(FromGoString("Temporal.Instant.prototype.round roundingIncrement is out of range")))
	}
	quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
	rounded := roundBigToIncrement(i.ns, quantum, roundingMode)
	return newInstant(rounded)
}

// ToZonedDateTimeISO implements Temporal.Instant.prototype.toZonedDateTimeISO: it pairs the exact
// instant with a time zone under the ISO 8601 calendar, giving the count a wall-clock reading. The
// epoch nanosecond count is already in range, so newZonedDateTime only resolves the zone, throwing
// a RangeError on an unrecognized identifier, and stores a copy under the empty ISO calendar.
func (i *Instant) ToZonedDateTimeISO(timeZone string) *ZonedDateTime {
	return newZonedDateTime(i.ns, timeZone)
}

// InstantCompare implements Temporal.Instant.compare: -1, 0, or 1 as the first instant is
// earlier than, equal to, or later than the second, the sign of the big.Int comparison.
func InstantCompare(a, b *Instant) float64 { return float64(a.ns.Cmp(b.ns)) }

// ToString implements Temporal.Instant.prototype.toString under the default options: the
// ISO 8601 date-time in UTC with a Z designator, a fractional-second part appended only
// when a sub-second field is set. The count is split into a day index and a within-day
// nanosecond remainder by Euclidean division, so a negative instant lands on the correct
// earlier day with a positive time of day, then the day index becomes an ISO date and the
// remainder the wall-clock time.
func (i *Instant) ToString() BStr {
	return FromGoString(i.isoString())
}

// ToJSON implements Temporal.Instant.prototype.toJSON, the same UTC ISO string toString
// produces under default options.
func (i *Instant) ToJSON() BStr { return i.ToString() }

func (i *Instant) isoString() string {
	q := new(big.Int)
	m := new(big.Int)
	q.DivMod(i.ns, nsPerDay, m)
	year, month, day := epochDaysToISO(int(q.Int64()))
	rem := m.Int64()
	hour := int(rem / 3_600_000_000_000)
	rem %= 3_600_000_000_000
	minute := int(rem / 60_000_000_000)
	rem %= 60_000_000_000
	second := int(rem / 1_000_000_000)
	frac := int(rem % 1_000_000_000)
	s := formatISOYear(year) + "-" + twoDigit(month) + "-" + twoDigit(day) +
		"T" + twoDigit(hour) + ":" + twoDigit(minute) + ":" + twoDigit(second)
	if frac > 0 {
		s += "." + strings.TrimRight(zeroPad(frac, 9), "0")
	}
	return s + "Z"
}

// epochDaysToISO is the inverse of isoToEpochDays: it turns a count of days since the
// epoch into the proleptic Gregorian year, month, and day, the civil-from-days algorithm
// that pairs with the days-from-civil count isoToEpochDays runs. It is exact across the
// whole Instant range, where the day index reaches ±10^8.
func epochDaysToISO(z int) (year, month, day int) {
	z += 719468
	era := z
	if z < 0 {
		era = z - 146096
	}
	era /= 146097
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y := yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	day = doy - (153*mp+2)/5 + 1
	month = mp + 3
	if mp >= 10 {
		month = mp - 9
	}
	if month <= 2 {
		y++
	}
	return y, month, day
}

// zeroPad renders n as a decimal left-padded with zeros to at least width digits.
func zeroPad(n, width int) string {
	s := strconv.Itoa(n)
	for len(s) < width {
		s = "0" + s
	}
	return s
}

// nsPerSecond is the third divisor the exact-time math leans on, alongside nsPerDay and
// nsPerMilli: it splits an epoch-nanosecond count into whole seconds and a sub-second
// remainder for the offset lookup, which the standard library keys on Unix seconds.
var nsPerSecond = big.NewInt(1_000_000_000)

// ZonedDateTime is bento's runtime representation of a Temporal.ZonedDateTime (Temporal
// §7): an exact point on the time line, the same nanosecond count an Instant holds, paired
// with a time zone that gives the count a wall-clock reading and a calendar. Like the plain
// types this slice hosts only the ISO 8601 calendar; a non-ISO calendar hands back at
// lowering, so calendarId always reports iso8601.
//
// The three fields are the epoch-nanosecond count, the resolved standard-library location
// the offset lookup runs against, and the canonical time-zone identifier the getters and
// toString report. The wall-clock getters do not cache a second copy of the date and time:
// each derives the local reading by adding the zone's offset at this instant to the count
// and splitting the result, so a getter always reflects the offset in force at its own
// instant, which is what makes a reading across a daylight-saving transition come out right.
type ZonedDateTime struct {
	ns   *big.Int       // epoch nanoseconds, in [nsMinInstant, nsMaxInstant]
	loc  *time.Location // resolved zone the offset lookup runs against
	tzID BStr           // canonical time-zone identifier, reported by timeZoneId and toString
	cal  string         // calendar id, empty for iso8601, that reads the wall-clock fields
}

// calendarID maps the stored calendar to its identifier, reading the empty default as the ISO
// calendar so a zoned date-time built with no calendar reports iso8601.
func (z *ZonedDateTime) calendarID() string {
	if z.cal == "" {
		return "iso8601"
	}
	return z.cal
}

// resolveTimeZone turns a Temporal time-zone identifier into a standard-library location and
// its canonical spelling, throwing a RangeError the way ToTemporalTimeZoneIdentifier does
// when the identifier is neither UTC, a numeric offset, nor a named IANA zone the host knows.
// UTC is answered directly, a numeric offset becomes a fixed zone, and any other identifier
// is looked up in the IANA database.
func resolveTimeZone(id string) (*time.Location, string) {
	if id == "UTC" {
		return time.UTC, "UTC"
	}
	if loc, canon, ok := parseOffsetZone(id); ok {
		return loc, canon
	}
	loc, err := time.LoadLocation(id)
	if err != nil {
		Throw(NewRangeError(FromGoString("Temporal.ZonedDateTime time zone " + id + " is not recognized")))
	}
	return loc, id
}

// parseOffsetZone reads a numeric UTC-offset identifier, one of ±HH, ±HHMM, ±HH:MM, or
// ±HH:MM:SS, into a fixed zone and its canonical ±HH:MM[:SS] spelling. It reports false for
// anything that is not a well-formed offset, so resolveTimeZone falls through to the named
// lookup.
func parseOffsetZone(id string) (*time.Location, string, bool) {
	if len(id) < 3 || (id[0] != '+' && id[0] != '-') {
		return nil, "", false
	}
	sign := 1
	if id[0] == '-' {
		sign = -1
	}
	digits := strings.ReplaceAll(id[1:], ":", "")
	if len(digits) != 2 && len(digits) != 4 && len(digits) != 6 {
		return nil, "", false
	}
	for i := 0; i < len(digits); i++ {
		if digits[i] < '0' || digits[i] > '9' {
			return nil, "", false
		}
	}
	hour, _ := strconv.Atoi(digits[0:2])
	minute, second := 0, 0
	if len(digits) >= 4 {
		minute, _ = strconv.Atoi(digits[2:4])
	}
	if len(digits) == 6 {
		second, _ = strconv.Atoi(digits[4:6])
	}
	if hour > 23 || minute > 59 || second > 59 {
		return nil, "", false
	}
	total := sign * (hour*3600 + minute*60 + second)
	canon := formatOffset(total)
	return time.FixedZone(canon, total), canon, true
}

// formatOffset renders a signed offset in seconds as the ±HH:MM spelling, extended to
// ±HH:MM:SS only when the offset carries a sub-minute part, the shape Temporal's offset
// getter and toString both use.
func formatOffset(seconds int) string {
	sign := "+"
	if seconds < 0 {
		sign = "-"
		seconds = -seconds
	}
	hour := seconds / 3600
	minute := seconds / 60 % 60
	second := seconds % 60
	s := sign + twoDigit(hour) + ":" + twoDigit(minute)
	if second != 0 {
		s += ":" + twoDigit(second)
	}
	return s
}

// newZonedDateTime validates a nanosecond count, resolves the time zone, and stores a copy
// of the count, the shared body of the constructor and the epoch factories. The count is
// validated before the zone is resolved, the order the specification's constructor follows.
func newZonedDateTime(ns *big.Int, tzID string) *ZonedDateTime {
	validateEpochNanoseconds(ns)
	loc, canon := resolveTimeZone(tzID)
	return &ZonedDateTime{ns: new(big.Int).Set(ns), loc: loc, tzID: FromGoString(canon)}
}

// NewZonedDateTime builds a ZonedDateTime from the constructor's bigint epoch count and
// time-zone identifier, running IsValidEpochNanoseconds and then ToTemporalTimeZoneIdentifier
// the way new Temporal.ZonedDateTime(ns, tz) does. The optional calendar argument is not
// accepted here; a non-ISO calendar hands back at lowering.
func NewZonedDateTime(epochNanoseconds *big.Int, timeZone BStr) *ZonedDateTime {
	return newZonedDateTime(epochNanoseconds, timeZone.ToGoString())
}

// ZonedDateTimeFrom implements Temporal.ZonedDateTime.from for a ZonedDateTime argument: it
// returns a fresh ZonedDateTime with the same count, zone, and calendar, the copy the
// specification makes. from over a string or a property bag needs the parser and the option
// handling and hands back at lowering, so this body is only reached with a ZonedDateTime.
func ZonedDateTimeFrom(z *ZonedDateTime) *ZonedDateTime {
	return &ZonedDateTime{ns: new(big.Int).Set(z.ns), loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// ZonedDateTimeFromString implements Temporal.ZonedDateTime.from over a string. The string
// must carry a time-zone annotation in brackets, the identifier the wall-clock reading is
// resolved against; a string with none throws a RangeError the way the specification does,
// since a zoned date-time has no zone without it. The wall-clock date and time are read
// through the shared parser, then folded to an exact instant one of three ways: a Z
// designator names the instant exactly, so the wall clock is read as UTC; a bare string with
// no offset resolves through the zone with the default compatible disambiguation, which takes
// the earlier reading in a fall-back overlap and shifts forward across a spring-forward gap; a
// string with a numeric offset must match one of the zone's offsets for that wall clock under
// the default reject option, and a mismatch throws. bento's ZonedDateTime hosts only the ISO
// calendar, so a non-ISO calendar annotation throws, and the lowerer hands back any literal
// naming one before this is reached.
func ZonedDateTimeFromString(s string) *ZonedDateTime {
	p, ok := parseTemporalISOString(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.ZonedDateTime")))
	}
	if p.timeZone == "" {
		Throw(NewRangeError(FromGoString("a Temporal.ZonedDateTime string requires a time zone annotation in brackets")))
	}
	if p.calendar != "" && !strings.EqualFold(p.calendar, "iso8601") {
		Throw(NewRangeError(FromGoString("Temporal.ZonedDateTime from a string supports only the iso8601 calendar")))
	}
	rejectISODate(float64(p.year), float64(p.month), float64(p.day))
	loc, canon := resolveTimeZone(p.timeZone)
	wall := wallNanoseconds(p)
	var epoch *big.Int
	switch {
	case p.hasZ:
		epoch = wall // a Z designator gives the exact UTC instant, the zone only reads it back
	case !p.hasOffset:
		epoch = disambiguateCompatible(loc, wall)
	default:
		epoch = matchZoneOffset(loc, wall, p.offsetNanoseconds, s, canon)
	}
	validateEpochNanoseconds(epoch)
	return &ZonedDateTime{ns: epoch, loc: loc, tzID: FromGoString(canon)}
}

// offsetFieldPattern matches the offset a property bag carries in its offset field, the same
// grammar the ISO string offset uses: a sign, then hours and optional minutes, seconds, and a
// fractional second, with the colons optional. A leading U+2212 minus is accepted alongside the
// ASCII hyphen the way the specification's grammar allows.
var offsetFieldPattern = regexp.MustCompile(`^([+\-\x{2212}])(\d{2})(?::?(\d{2}))?(?::?(\d{2}))?(?:[.,](\d{1,9}))?$`)

// parseOffsetFieldNanos reads a property bag's offset field into a signed nanosecond offset. The
// field is a UTC offset like "-05:00" or "+0530", not a zone name, so it names a fixed shift the
// offset option weighs against the zone. A field that does not match the offset grammar throws a
// RangeError, the way Temporal's ParseDateTimeUTCOffset does.
func parseOffsetFieldNanos(s string) int64 {
	m := offsetFieldPattern.FindStringSubmatch(s)
	if m == nil {
		Throw(NewRangeError(FromGoString("offset " + s + " is not a valid UTC offset")))
	}
	sign := int64(1)
	if m[1] != "+" {
		sign = -1
	}
	hour, _ := strconv.Atoi(m[2])
	minute := 0
	if m[3] != "" {
		minute, _ = strconv.Atoi(m[3])
	}
	second := 0
	if m[4] != "" {
		second, _ = strconv.Atoi(m[4])
	}
	var fracNanos int64
	if m[5] != "" {
		frac := m[5]
		for len(frac) < 9 {
			frac += "0"
		}
		fracNanos, _ = strconv.ParseInt(frac, 10, 64)
	}
	total := (int64(hour)*3600+int64(minute)*60+int64(second))*1_000_000_000 + fracNanos
	return sign * total
}

// ZonedDateTimeFromFields implements Temporal.ZonedDateTime.from over a property bag. The date and
// time fields build a wall-clock reading through PlainDateTimeFromFields under the overflow option,
// the timeZone field names the zone, and the reading folds to an exact instant one of two ways. A
// bag with no offset field resolves through the zone under the disambiguation option, exactly as a
// bare string does. A bag that carries an offset field weighs it under the offset option: use takes
// the offset at face value and reads the instant as the wall clock less that offset; ignore drops
// the offset and resolves through disambiguation; prefer keeps a zone instant whose offset matches
// and otherwise falls to disambiguation; reject demands a zone instant whose offset matches and
// throws when none does. bento's ZonedDateTime hosts only the ISO calendar, so the lowerer hands
// back a bag naming another before this is reached.
func ZonedDateTimeFromFields(year, month, day float64, hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], timeZone string, offset Opt[string], overflow, disambiguation, offsetOption string) *ZonedDateTime {
	pdt := PlainDateTimeFromFields(year, month, day, hour, minute, second, millisecond, microsecond, nanosecond, "iso8601", overflow)
	loc, canon := resolveTimeZone(timeZone)
	wall := wallNanoseconds(isoParse{
		year: pdt.date.year, month: pdt.date.month, day: pdt.date.day,
		hour: pdt.time.hour, minute: pdt.time.minute, second: pdt.time.second,
		millisecond: pdt.time.millisecond, microsecond: pdt.time.microsecond, nanosecond: pdt.time.nanosecond,
	})
	var epoch *big.Int
	switch {
	case offset.IsUndefined() || offsetOption == "ignore":
		epoch = disambiguatePossible(loc, wall, disambiguation, canon)
	case offsetOption == "use":
		epoch = new(big.Int).Sub(wall, big.NewInt(parseOffsetFieldNanos(offset.Get())))
	case offsetOption == "prefer":
		offNs := parseOffsetFieldNanos(offset.Get())
		epoch = nil
		for _, cand := range possibleInstants(loc, wall) {
			off := new(big.Int).Sub(wall, cand)
			if off.IsInt64() && off.Int64() == offNs {
				epoch = cand
				break
			}
		}
		if epoch == nil {
			epoch = disambiguatePossible(loc, wall, disambiguation, canon)
		}
	default: // reject
		epoch = matchZoneOffset(loc, wall, parseOffsetFieldNanos(offset.Get()), timeZone, canon)
	}
	validateEpochNanoseconds(epoch)
	return &ZonedDateTime{ns: epoch, loc: loc, tzID: FromGoString(canon)}
}

// wallNanoseconds folds a parsed date and time into a nanosecond count, reading the wall clock
// as though it were UTC. The offset a time zone applies to reach the true instant is left to
// the caller, so this is the count Instant would hold for the same fields with no offset.
func wallNanoseconds(p isoParse) *big.Int {
	secs := int64(isoToEpochDays(p.year, p.month, p.day))*86_400 +
		int64(p.hour)*3600 + int64(p.minute)*60 + int64(p.second)
	ns := new(big.Int).SetInt64(secs)
	ns.Mul(ns, nsPerSecond)
	sub := int64(p.millisecond)*1_000_000 + int64(p.microsecond)*1_000 + int64(p.nanosecond)
	ns.Add(ns, big.NewInt(sub))
	return ns
}

// zoneOffsetSecondsAt reports the zone's UTC offset in seconds at the given Unix second, the
// standard-library lookup the disambiguation probes run against.
func zoneOffsetSecondsAt(loc *time.Location, unixSec int64) int {
	_, off := time.Unix(unixSec, 0).In(loc).Zone()
	return off
}

// possibleInstants returns the exact instants a wall-clock reading maps to in a zone: one for
// an ordinary reading, two across a fall-back overlap, and none inside a spring-forward gap.
// The wall clock, held as a UTC-relative count, is probed against the offset a day before and a
// day after; for each distinct offset the candidate instant is the wall count less that offset,
// kept only when the zone reports the same offset there, which is the round-trip the
// specification's GetPossibleEpochNanoseconds performs. The results come back in ascending order.
func possibleInstants(loc *time.Location, wall *big.Int) []*big.Int {
	sec := new(big.Int)
	rem := new(big.Int)
	sec.DivMod(wall, nsPerSecond, rem)
	wallSec := sec.Int64()
	offBefore := zoneOffsetSecondsAt(loc, wallSec-86_400)
	offAfter := zoneOffsetSecondsAt(loc, wallSec+86_400)
	var out []*big.Int
	add := func(off int) {
		epochSec := wallSec - int64(off)
		if zoneOffsetSecondsAt(loc, epochSec) == off {
			ns := new(big.Int).SetInt64(epochSec)
			ns.Mul(ns, nsPerSecond)
			ns.Add(ns, rem)
			out = append(out, ns)
		}
	}
	add(offBefore)
	if offAfter != offBefore {
		add(offAfter)
	}
	if len(out) == 2 && out[0].Cmp(out[1]) > 0 {
		out[0], out[1] = out[1], out[0]
	}
	return out
}

// disambiguateCompatible resolves a wall-clock reading to a single instant under Temporal's
// default compatible disambiguation, the option from over a bare string uses. It is the
// compatible arm of disambiguatePossible: an ordinary reading has one instant; a fall-back
// overlap takes the earlier of the two; a spring-forward gap shifts forward and takes the
// reading just after the transition. Compatible never rejects, so the zone name is unused.
func disambiguateCompatible(loc *time.Location, wall *big.Int) *big.Int {
	return disambiguatePossible(loc, wall, "compatible", "")
}

// disambiguatePossible resolves a wall-clock reading to a single instant under one of Temporal's
// four disambiguation options. An ordinary reading maps to one instant regardless of the option.
// A fall-back overlap maps to two instants: earlier and compatible take the first, later takes
// the second, reject throws. A spring-forward gap maps to none: reject throws, earlier shifts the
// reading back by the gap and takes the instant just before the transition, and compatible and
// later shift it forward and take the instant just after. The zone name canon appears in the
// reject error only.
func disambiguatePossible(loc *time.Location, wall *big.Int, disambiguation, canon string) *big.Int {
	p := possibleInstants(loc, wall)
	if len(p) == 1 {
		return p[0]
	}
	if len(p) > 1 {
		switch disambiguation {
		case "earlier", "compatible":
			return p[0]
		case "later":
			return p[len(p)-1]
		default: // reject
			Throw(NewRangeError(FromGoString("wall-clock time is ambiguous in " + canon)))
			return nil
		}
	}
	// A gap: the reading names no instant.
	if disambiguation == "reject" {
		Throw(NewRangeError(FromGoString("wall-clock time falls in a gap in " + canon)))
		return nil
	}
	sec := new(big.Int)
	rem := new(big.Int)
	sec.DivMod(wall, nsPerSecond, rem)
	wallSec := sec.Int64()
	offBefore := zoneOffsetSecondsAt(loc, wallSec-86_400)
	offAfter := zoneOffsetSecondsAt(loc, wallSec+86_400)
	gap := int64(offAfter-offBefore) * 1_000_000_000
	if disambiguation == "earlier" {
		shifted := new(big.Int).Sub(wall, big.NewInt(gap))
		if q := possibleInstants(loc, shifted); len(q) > 0 {
			return q[0]
		}
	} else { // compatible, later
		shifted := new(big.Int).Add(wall, big.NewInt(gap))
		if q := possibleInstants(loc, shifted); len(q) > 0 {
			return q[len(q)-1]
		}
	}
	epochSec := wallSec - int64(offAfter)
	ns := new(big.Int).SetInt64(epochSec)
	ns.Mul(ns, nsPerSecond)
	ns.Add(ns, rem)
	return ns
}

// matchZoneOffset resolves a wall-clock reading that carried a numeric offset under the default
// reject option: the offset must equal the zone's offset for one of the instants the reading
// maps to, and that instant is the result. A reading whose offset the zone never applies there
// throws a RangeError, the reject behavior, since a wall clock and an offset that disagree with
// the zone name an instant the string does not mean.
func matchZoneOffset(loc *time.Location, wall *big.Int, offsetNanoseconds int64, s, canon string) *big.Int {
	for _, cand := range possibleInstants(loc, wall) {
		off := new(big.Int).Sub(wall, cand)
		if off.IsInt64() && off.Int64() == offsetNanoseconds {
			return cand
		}
	}
	Throw(NewRangeError(FromGoString("offset " + formatOffset(int(offsetNanoseconds/1_000_000_000)) +
		" is invalid for " + s + " in " + canon)))
	return nil
}

// offsetSeconds returns the zone's UTC offset in seconds at this instant. The count splits
// into Unix seconds and a sub-second remainder by Euclidean division, so a negative count
// keys the standard-library lookup on the correct earlier second, and the location reports
// the offset in force there, daylight-saving transitions included.
func (z *ZonedDateTime) offsetSeconds() int {
	sec := new(big.Int)
	nsec := new(big.Int)
	sec.DivMod(z.ns, nsPerSecond, nsec)
	_, off := time.Unix(sec.Int64(), nsec.Int64()).In(z.loc).Zone()
	return off
}

// localDateTime builds the wall-clock reading this instant has in its zone: the offset at the
// instant is added to the count and the sum is split into an ISO date and a time of day. The
// fields are placed directly rather than run through NewPlainDateTime, since the sum is
// already a valid reading and the constructor's range check would reject the boundary
// instants the offset legitimately pushes a day past the plain-type limits.
func (z *ZonedDateTime) localDateTime() *PlainDateTime {
	local := new(big.Int).Add(z.ns, big.NewInt(int64(z.offsetSeconds())*1_000_000_000))
	q := new(big.Int)
	m := new(big.Int)
	q.DivMod(local, nsPerDay, m)
	year, month, day := epochDaysToISO(int(q.Int64()))
	rem := m.Int64()
	hour := int(rem / 3_600_000_000_000)
	rem %= 3_600_000_000_000
	minute := int(rem / 60_000_000_000)
	rem %= 60_000_000_000
	second := int(rem / 1_000_000_000)
	frac := rem % 1_000_000_000
	return &PlainDateTime{
		date: PlainDate{year: year, month: month, day: day, cal: z.cal},
		time: PlainTime{
			hour:        hour,
			minute:      minute,
			second:      second,
			millisecond: int(frac / 1_000_000),
			microsecond: int(frac / 1_000 % 1_000),
			nanosecond:  int(frac % 1_000),
		},
	}
}

// The exact-time getters read the instant directly, independent of the zone.

// EpochNanoseconds returns a fresh copy of the count, so a caller holds a bigint independent
// of the ZonedDateTime's field.
func (z *ZonedDateTime) EpochNanoseconds() *big.Int { return new(big.Int).Set(z.ns) }

// EpochMilliseconds returns the count floored to whole milliseconds, the same Euclidean
// division Instant uses.
func (z *ZonedDateTime) EpochMilliseconds() float64 {
	q := new(big.Int).Div(z.ns, nsPerMilli)
	return float64(q.Int64())
}

// TimeZoneId reports the canonical time-zone identifier.
func (z *ZonedDateTime) TimeZoneId() BStr { return z.tzID }

// CalendarId reports the calendar the wall-clock fields read under, iso8601 by default.
func (z *ZonedDateTime) CalendarId() BStr { return FromGoString(z.calendarID()) }

// OffsetNanoseconds reports the zone's UTC offset at this instant in nanoseconds. The offset
// stays within ±14 hours, so the nanosecond product is exact in a float64.
func (z *ZonedDateTime) OffsetNanoseconds() float64 {
	return float64(int64(z.offsetSeconds()) * 1_000_000_000)
}

// Offset reports the zone's UTC offset at this instant in the ±HH:MM[:SS] spelling.
func (z *ZonedDateTime) Offset() BStr { return FromGoString(formatOffset(z.offsetSeconds())) }

// The wall-clock getters delegate to the local reading, which resolves the offset at this
// instant and splits the adjusted count into an ISO date and time.

func (z *ZonedDateTime) Year() float64        { return z.localDateTime().Year() }
func (z *ZonedDateTime) Month() float64       { return z.localDateTime().Month() }
func (z *ZonedDateTime) Day() float64         { return z.localDateTime().Day() }
func (z *ZonedDateTime) Hour() float64        { return z.localDateTime().Hour() }
func (z *ZonedDateTime) Minute() float64      { return z.localDateTime().Minute() }
func (z *ZonedDateTime) Second() float64      { return z.localDateTime().Second() }
func (z *ZonedDateTime) Millisecond() float64 { return z.localDateTime().Millisecond() }
func (z *ZonedDateTime) Microsecond() float64 { return z.localDateTime().Microsecond() }
func (z *ZonedDateTime) Nanosecond() float64  { return z.localDateTime().Nanosecond() }
func (z *ZonedDateTime) MonthCode() BStr      { return z.localDateTime().MonthCode() }
func (z *ZonedDateTime) DayOfWeek() float64   { return z.localDateTime().DayOfWeek() }
func (z *ZonedDateTime) DayOfYear() float64   { return z.localDateTime().DayOfYear() }
func (z *ZonedDateTime) DaysInWeek() float64  { return z.localDateTime().DaysInWeek() }
func (z *ZonedDateTime) DaysInMonth() float64 { return z.localDateTime().DaysInMonth() }
func (z *ZonedDateTime) DaysInYear() float64  { return z.localDateTime().DaysInYear() }
func (z *ZonedDateTime) MonthsInYear() float64 {
	return z.localDateTime().MonthsInYear()
}
func (z *ZonedDateTime) InLeapYear() bool         { return z.localDateTime().InLeapYear() }
func (z *ZonedDateTime) Era() Opt[BStr]           { return z.localDateTime().Era() }
func (z *ZonedDateTime) EraYear() Opt[float64]    { return z.localDateTime().EraYear() }
func (z *ZonedDateTime) WeekOfYear() Opt[float64] { return z.localDateTime().WeekOfYear() }
func (z *ZonedDateTime) YearOfWeek() Opt[float64] { return z.localDateTime().YearOfWeek() }

// ToInstant implements Temporal.ZonedDateTime.prototype.toInstant: the exact time with the
// zone dropped, the same nanosecond count as an Instant.
func (z *ZonedDateTime) ToInstant() *Instant { return newInstant(z.ns) }

// ToPlainDateTime implements Temporal.ZonedDateTime.prototype.toPlainDateTime: the wall-clock
// reading with the zone dropped.
func (z *ZonedDateTime) ToPlainDateTime() *PlainDateTime { return z.localDateTime() }

// ToPlainDate implements Temporal.ZonedDateTime.prototype.toPlainDate: the calendar date of
// the wall-clock reading.
func (z *ZonedDateTime) ToPlainDate() *PlainDate {
	d := z.localDateTime().date
	return &d
}

// ToPlainTime implements Temporal.ZonedDateTime.prototype.toPlainTime: the time of day of the
// wall-clock reading.
func (z *ZonedDateTime) ToPlainTime() *PlainTime {
	t := z.localDateTime().time
	return &t
}

// AddDuration implements Temporal.ZonedDateTime.prototype.add and, over a negated Duration,
// subtract. Following the specification's AddZonedDateTime, the calendar part and the exact-time
// part move separately. When the duration carries no years, months, weeks, or days the addition
// is pure exact time: the time fields fold to nanoseconds and add straight onto the count, so an
// hour added stays an hour on the time line even across a daylight-saving change. Otherwise the
// calendar part first adds to the wall-clock reading in the calendar under the overflow rule, the
// moved wall clock re-resolves to an instant through the zone under the default compatible
// disambiguation, and the exact-time part then folds onto that instant as plain nanoseconds. That
// order is what makes a day added across a transition land on the same wall-clock time a day later
// while the offset it reports updates. The zone and calendar carry through and the result is
// range-checked.
func (z *ZonedDateTime) AddDuration(dur *Duration, overflow string) *ZonedDateTime {
	if dur.years == 0 && dur.months == 0 && dur.weeks == 0 && dur.days == 0 {
		result := new(big.Int).Add(z.ns, durationTimeNanos(dur))
		validateEpochNanoseconds(result)
		return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
	}
	wall := z.localDateTime()
	dateDur := &Duration{years: dur.years, months: dur.months, weeks: dur.weeks, days: dur.days}
	moved := wall.date.AddDate(dateDur, overflow)
	intermediate := disambiguateCompatible(z.loc, wallNanoseconds(isoParse{
		year: moved.year, month: moved.month, day: moved.day,
		hour: wall.time.hour, minute: wall.time.minute, second: wall.time.second,
		millisecond: wall.time.millisecond, microsecond: wall.time.microsecond, nanosecond: wall.time.nanosecond,
	}))
	result := new(big.Int).Add(intermediate, durationTimeNanos(dur))
	validateEpochNanoseconds(result)
	return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// Until returns the difference from the receiver to other as a Duration balanced from
// largestUnit down. Since returns the difference from other to the receiver, the negation of
// Until, so the calendar anchoring stays on the receiver the way the specification requires.
func (z *ZonedDateTime) Until(other *ZonedDateTime, largestUnit string) *Duration {
	return zonedDateTimeDifference(z, other, largestUnit)
}

// Since returns the negation of the receiver-to-other difference.
func (z *ZonedDateTime) Since(other *ZonedDateTime, largestUnit string) *Duration {
	return zonedDateTimeDifference(z, other, largestUnit).Negated()
}

// zonedDateTimeDifference implements the specification's DifferenceZonedDateTime. A time-unit
// largestUnit needs no calendar and no zone: the difference is the exact nanosecond gap balanced
// from that unit down, which is what makes a difference in hours across a daylight-saving change
// count the real elapsed hours, twenty-three or twenty-five on a transition day rather than a flat
// twenty-four. A calendar-unit largestUnit splits the distance into a calendar date part and an
// exact-time part that must share one sign. It walks the end date back by whole days until the
// intermediate wall clock, taken at the start's time of day and resolved through the zone under
// the compatible disambiguation, sits on the near side of the target, so the leftover exact time
// keeps the overall sign and stays within one zoned day, whose length the transition may stretch
// or shrink. The date part is then the calendar difference from the start date to that
// intermediate date, and the time part balances from hours down. The two values must share a
// zone and calendar, which two ZonedDateTimes reached here always do.
func zonedDateTimeDifference(from, to *ZonedDateTime, largestUnit string) *Duration {
	switch largestUnit {
	case "year", "month", "week", "day":
	default:
		diff := new(big.Int).Sub(to.ns, from.ns)
		_, _, largeRank := plainTimeUnitInfo(largestUnit)
		return durationFromDayNanos(diff, largeRank)
	}
	if from.ns.Cmp(to.ns) == 0 {
		return NewDuration(0, 0, 0, 0, 0, 0, 0, 0, 0, 0)
	}
	startDT := from.localDateTime()
	endDT := to.localDateTime()
	sign := to.ns.Cmp(from.ns)
	maxDayCorrection := 1
	if sign == 1 {
		maxDayCorrection = 2
	}
	dayCorrection := 0
	if new(big.Int).Sub(endDT.time.dayNanos(), startDT.time.dayNanos()).Sign() == -sign {
		dayCorrection = 1
	}
	endEpochDays := isoToEpochDays(endDT.date.year, endDT.date.month, endDT.date.day)
	var iy, im, id int
	timeDuration := new(big.Int)
	for {
		iy, im, id = epochDaysToISO(endEpochDays - dayCorrection*sign)
		intermediate := disambiguateCompatible(from.loc, wallNanoseconds(isoParse{
			year: iy, month: im, day: id,
			hour: startDT.time.hour, minute: startDT.time.minute, second: startDT.time.second,
			millisecond: startDT.time.millisecond, microsecond: startDT.time.microsecond, nanosecond: startDT.time.nanosecond,
		}))
		timeDuration.Sub(to.ns, intermediate)
		if sign != -timeDuration.Sign() || dayCorrection >= maxDayCorrection {
			break
		}
		dayCorrection++
	}
	years, months, weeks, days := differenceISODate(startDT.date.year, startDT.date.month, startDT.date.day, iy, im, id, largestUnit)
	t := durationFromDayNanos(timeDuration, 0)
	return NewDuration(float64(years), float64(months), float64(weeks), float64(days), t.hours, t.minutes, t.seconds, t.milliseconds, t.microseconds, t.nanoseconds)
}

// startOfDayInstant returns the first instant of the given local calendar day in the receiver's
// zone: the wall-clock midnight resolved through the compatible disambiguation, so an ordinary day
// starts at 00:00, a fall-back day whose midnight occurs twice takes the earlier, and a rare
// spring-forward at midnight takes the instant just after the gap. It is the building block the
// day-unit round and the day-length query lean on.
func (z *ZonedDateTime) startOfDayInstant(year, month, day int) *big.Int {
	return disambiguateCompatible(z.loc, wallNanoseconds(isoParse{year: year, month: month, day: day}))
}

// resolvePreferOffset resolves a wall-clock count to an instant, preferring the receiver's current
// offset: when the reading is ambiguous, a fall-back overlap, and one of its instants carries that
// same offset, that instant is kept, so rounding a value inside an overlap stays on the branch it
// started on. Otherwise it falls to the compatible disambiguation. This is the InterpretISODateTime
// offset-prefer behavior the specification's round uses.
func (z *ZonedDateTime) resolvePreferOffset(localCount *big.Int, preferOffsetNs int64) *big.Int {
	for _, cand := range possibleInstants(z.loc, localCount) {
		off := new(big.Int).Sub(localCount, cand)
		if off.IsInt64() && off.Int64() == preferOffsetNs {
			return cand
		}
	}
	return disambiguateCompatible(z.loc, localCount)
}

// Round implements Temporal.ZonedDateTime.prototype.round. A day smallestUnit rounds the instant
// within the zoned day, whose length the daylight-saving transitions stretch or shrink: the day
// progress from the day's start is rounded to the whole day length, twenty-three or twenty-five
// hours on a transition day rather than a flat twenty-four, and lands on this midnight or the next.
// A time smallestUnit rounds the wall clock the way a PlainDateTime does, carrying past midnight
// when it must, and the rounded wall clock re-resolves to an instant preferring the original offset,
// so a value rounded inside a fall-back overlap keeps the branch it was on. Only increment one is
// allowed for the day unit; a time increment must divide its next larger unit.
func (z *ZonedDateTime) Round(smallestUnit string, increment float64, roundingMode string) *ZonedDateTime {
	inc := int64(toIntegerWithTruncation(increment))
	wall := z.localDateTime()
	if smallestUnit == "day" {
		if inc != 1 {
			Throw(NewRangeError(FromGoString("Temporal.ZonedDateTime.prototype.round roundingIncrement is out of range")))
		}
		startNs := z.startOfDayInstant(wall.date.year, wall.date.month, wall.date.day)
		ny, nm, nd := addISODate(wall.date.year, wall.date.month, wall.date.day, 0, 0, 0, big.NewInt(1), "constrain")
		endNs := z.startOfDayInstant(ny, nm, nd)
		dayLengthNs := new(big.Int).Sub(endNs, startNs)
		dayProgressNs := new(big.Int).Sub(z.ns, startNs)
		rounded := roundBigToIncrement(dayProgressNs, dayLengthNs, roundingMode)
		result := new(big.Int).Add(startNs, rounded)
		validateEpochNanoseconds(result)
		return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
	}
	unitNs, dividend, _ := plainTimeUnitInfo(smallestUnit)
	if inc < 1 || inc >= dividend || dividend%inc != 0 {
		Throw(NewRangeError(FromGoString("Temporal.ZonedDateTime.prototype.round roundingIncrement is out of range")))
	}
	quantum := new(big.Int).Mul(big.NewInt(inc), big.NewInt(unitNs))
	rounded := roundBigToIncrement(wall.time.dayNanos(), quantum, roundingMode)
	dayCarry := new(big.Int)
	timeOfDay := new(big.Int)
	dayCarry.DivMod(rounded, nsPerDay, timeOfDay)
	y, m, d := addISODate(wall.date.year, wall.date.month, wall.date.day, 0, 0, 0, dayCarry, "constrain")
	localCount := new(big.Int).Mul(big.NewInt(int64(isoToEpochDays(y, m, d))), nsPerDay)
	localCount.Add(localCount, timeOfDay)
	result := z.resolvePreferOffset(localCount, int64(z.offsetSeconds())*1_000_000_000)
	validateEpochNanoseconds(result)
	return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// wallCount returns the naive nanosecond count of a wall-clock reading, the epoch-day count of its
// ISO date scaled to nanoseconds plus its time of day, the input possibleInstants and the
// disambiguation helpers take.
func wallCount(dt *PlainDateTime) *big.Int {
	c := new(big.Int).Mul(big.NewInt(int64(isoToEpochDays(dt.date.year, dt.date.month, dt.date.day))), nsPerDay)
	c.Add(c, dt.time.dayNanos())
	return c
}

// WithFields implements Temporal.ZonedDateTime.prototype.with. It overlays the bag's present date
// and time fields onto the wall-clock reading under the overflow rule, reusing the PlainDateTime
// field overlay, then re-resolves the reshaped wall clock to an instant preferring the original
// offset, the default offset option with is: a field changed inside a fall-back overlap keeps the
// branch the value was on, and a field that lands the wall clock in a spring-forward gap shifts
// forward under the compatible fallback. The zone and the calendar carry through.
func (z *ZonedDateTime) WithFields(year, month, day, hour, minute, second, millisecond, microsecond, nanosecond Opt[float64], overflow string) *ZonedDateTime {
	reshaped := z.localDateTime().WithFields(year, month, day, hour, minute, second, millisecond, microsecond, nanosecond, overflow)
	result := z.resolvePreferOffset(wallCount(reshaped), int64(z.offsetSeconds())*1_000_000_000)
	validateEpochNanoseconds(result)
	return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// WithPlainTime implements Temporal.ZonedDateTime.prototype.withPlainTime. It keeps the wall-clock
// date, replaces the time of day, defaulting to midnight when none is given, and re-resolves through
// the compatible disambiguation rather than preferring the old offset, so a new time inside a
// fall-back overlap takes the earlier branch the way the specification's withPlainTime does.
func (z *ZonedDateTime) WithPlainTime(time *PlainTime) *ZonedDateTime {
	t := PlainTime{}
	if time != nil {
		t = *time
	}
	reshaped := &PlainDateTime{date: z.localDateTime().date, time: t}
	result := disambiguateCompatible(z.loc, wallCount(reshaped))
	validateEpochNanoseconds(result)
	return &ZonedDateTime{ns: result, loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// WithTimeZone implements Temporal.ZonedDateTime.prototype.withTimeZone. It keeps the exact instant
// and re-homes it in another zone, so the wall clock and the offset re-read there while the instant
// is unchanged. An unrecognized identifier throws a RangeError through the shared resolver.
func (z *ZonedDateTime) WithTimeZone(timeZone string) *ZonedDateTime {
	moved := newZonedDateTime(z.ns, timeZone)
	moved.cal = z.cal
	return moved
}

// WithCalendar implements Temporal.ZonedDateTime.prototype.withCalendar for the ISO calendar, the
// only one bento's ZonedDateTime hosts. It keeps the instant and the zone and returns a copy; a
// non-ISO calendar hands back at lowering, so this is only reached for iso8601, an identity move.
func (z *ZonedDateTime) WithCalendar() *ZonedDateTime {
	return &ZonedDateTime{ns: new(big.Int).Set(z.ns), loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// StartOfDay implements Temporal.ZonedDateTime.prototype.startOfDay. It returns the first instant of
// the receiver's local calendar day in its zone, wall-clock midnight resolved through the compatible
// rule, so an ordinary day starts at 00:00 and a rare spring-forward at midnight lands just past the
// gap. The zone and calendar carry over unchanged.
func (z *ZonedDateTime) StartOfDay() *ZonedDateTime {
	dt := z.localDateTime()
	start := z.startOfDayInstant(dt.date.year, dt.date.month, dt.date.day)
	return &ZonedDateTime{ns: start, loc: z.loc, tzID: z.tzID, cal: z.cal}
}

// HoursInDay implements Temporal.ZonedDateTime.prototype.hoursInDay. It reads the length of the
// receiver's local calendar day as the exact hours between this day's start and the next day's
// start, so an ordinary day is twenty-four, a spring-forward day twenty-three, and a fall-back day
// twenty-five. The gap divides in floating point so a zone whose transition is off the hour keeps
// its fractional part.
func (z *ZonedDateTime) HoursInDay() float64 {
	dt := z.localDateTime()
	today := z.startOfDayInstant(dt.date.year, dt.date.month, dt.date.day)
	ny, nm, nd := addISODate(dt.date.year, dt.date.month, dt.date.day, 0, 0, 0, big.NewInt(1), "constrain")
	next := z.startOfDayInstant(ny, nm, nd)
	gap := new(big.Int).Sub(next, today)
	return float64(gap.Int64()) / 3_600_000_000_000
}

// Equals implements Temporal.ZonedDateTime.prototype.equals for a ZonedDateTime argument: two
// zoned date-times are equal when they name the same instant in the same zone under the same
// calendar, so the check is the count, the canonical zone identifier, and the calendar.
func (z *ZonedDateTime) Equals(other *ZonedDateTime) bool {
	return z.ns.Cmp(other.ns) == 0 &&
		z.tzID.ToGoString() == other.tzID.ToGoString() &&
		z.calendarID() == other.calendarID()
}

// ZonedDateTimeCompare implements Temporal.ZonedDateTime.compare: -1, 0, or 1 as the first
// instant is before, at, or after the second. The comparison is on the exact time only; the
// zone and calendar do not enter it.
func ZonedDateTimeCompare(a, b *ZonedDateTime) float64 { return float64(a.ns.Cmp(b.ns)) }

// ToString implements Temporal.ZonedDateTime.prototype.toString under the default options:
// the local ISO 8601 date-time, the UTC offset at this instant, the time-zone identifier in
// brackets, and, for a non-ISO calendar, the calendar annotation after it, the round-trippable
// form.
func (z *ZonedDateTime) ToString() BStr {
	dt := z.localDateTime()
	return FromGoString(dt.date.dateCore() + "T" + dt.time.isoString() +
		formatOffset(z.offsetSeconds()) + "[" + z.tzID.ToGoString() + "]" +
		dt.date.calendarAnnotation())
}

// ToJSON implements Temporal.ZonedDateTime.prototype.toJSON, the same string toString
// produces under default options.
func (z *ZonedDateTime) ToJSON() BStr { return z.ToString() }

// nowNanoseconds returns the current instant as a nanosecond count since the Unix epoch, the
// reading every Temporal.Now function is built on. It reads the host wall clock, except that a
// BENTO_NOW_NS environment variable, a decimal nanosecond count, overrides it: the differential
// harness sets that variable so a Temporal.Now fixture prints a value it can pin in an oracle,
// while an unset variable leaves the real clock in place for a program run outside the harness.
func nowNanoseconds() *big.Int {
	if s := os.Getenv("BENTO_NOW_NS"); s != "" {
		if n, ok := new(big.Int).SetString(s, 10); ok {
			return n
		}
	}
	return big.NewInt(time.Now().UnixNano())
}

// systemTimeZoneId returns the identifier Temporal.Now reports as the host's default time zone.
// It reads the TZ environment variable, the same knob Go's time package and the differential
// harness use to pin the zone, and defaults to UTC when TZ is unset so the default is always a
// valid, deterministic identifier rather than the host-specific local zone Go cannot name.
func systemTimeZoneId() string {
	if tz := os.Getenv("TZ"); tz != "" {
		return tz
	}
	return "UTC"
}

// NowInstant implements Temporal.Now.instant, the current instant as an exact point on the time
// line with no zone.
func NowInstant() *Instant { return newInstant(nowNanoseconds()) }

// NowTimeZoneId implements Temporal.Now.timeZoneId, the host's default time-zone identifier.
func NowTimeZoneId() BStr { return FromGoString(systemTimeZoneId()) }

// NowZonedDateTimeISO implements Temporal.Now.zonedDateTimeISO, the current instant paired with a
// zone under the ISO calendar. With no argument the zone is the host default; an explicit
// identifier names another zone, which resolveTimeZone validates.
func NowZonedDateTimeISO() *ZonedDateTime {
	return newZonedDateTime(nowNanoseconds(), systemTimeZoneId())
}

// NowZonedDateTimeISOIn is Temporal.Now.zonedDateTimeISO(timeZone), the current instant in the
// named zone.
func NowZonedDateTimeISOIn(timeZone BStr) *ZonedDateTime {
	return newZonedDateTime(nowNanoseconds(), timeZone.ToGoString())
}

// NowPlainDateTimeISO implements Temporal.Now.plainDateTimeISO, the wall-clock date and time the
// host default zone reads at the current instant.
func NowPlainDateTimeISO() *PlainDateTime { return NowZonedDateTimeISO().ToPlainDateTime() }

// NowPlainDateTimeISOIn is Temporal.Now.plainDateTimeISO(timeZone), the wall-clock reading in the
// named zone.
func NowPlainDateTimeISOIn(timeZone BStr) *PlainDateTime {
	return NowZonedDateTimeISOIn(timeZone).ToPlainDateTime()
}

// NowPlainDateISO implements Temporal.Now.plainDateISO, the calendar date the host default zone
// reads at the current instant.
func NowPlainDateISO() *PlainDate { return NowZonedDateTimeISO().ToPlainDate() }

// NowPlainDateISOIn is Temporal.Now.plainDateISO(timeZone), the calendar date in the named zone.
func NowPlainDateISOIn(timeZone BStr) *PlainDate {
	return NowZonedDateTimeISOIn(timeZone).ToPlainDate()
}

// NowPlainTimeISO implements Temporal.Now.plainTimeISO, the wall-clock time the host default zone
// reads at the current instant.
func NowPlainTimeISO() *PlainTime { return NowZonedDateTimeISO().ToPlainTime() }

// NowPlainTimeISOIn is Temporal.Now.plainTimeISO(timeZone), the wall-clock time in the named zone.
func NowPlainTimeISOIn(timeZone BStr) *PlainTime {
	return NowZonedDateTimeISOIn(timeZone).ToPlainTime()
}
