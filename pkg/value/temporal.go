package value

import (
	"math"
	"math/big"
	"os"
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
	total := new(big.Int)
	total.Add(total, bigMulInt(float64(pt.hour), 3_600_000_000_000))
	total.Add(total, bigMulInt(float64(pt.minute), 60_000_000_000))
	total.Add(total, bigMulInt(float64(pt.second), 1_000_000_000))
	total.Add(total, bigMulInt(float64(pt.millisecond), 1_000_000))
	total.Add(total, bigMulInt(float64(pt.microsecond), 1_000))
	total.Add(total, big.NewInt(int64(pt.nanosecond)))
	total.Add(total, bigMulInt(dur.hours, 3_600_000_000_000))
	total.Add(total, bigMulInt(dur.minutes, 60_000_000_000))
	total.Add(total, bigMulInt(dur.seconds, 1_000_000_000))
	total.Add(total, bigMulInt(dur.milliseconds, 1_000_000))
	total.Add(total, bigMulInt(dur.microseconds, 1_000))
	total.Add(total, bigMulInt(dur.nanoseconds, 1))
	total.Mod(total, nsPerDay)
	return plainTimeFromDayNanos(total)
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

// PlainYearMonth is bento's runtime representation of a Temporal.PlainYearMonth (Temporal
// §9): a calendar year and month with no day, no time, and no zone, the way a credit card
// carries an expiry. Like PlainDate it hosts only the ISO 8601 calendar; a non-ISO calendar
// hands back at lowering. The specification anchors a year-month to a reference ISO day so a
// calendar can resolve calendar-dependent fields, but the ISO calendar needs no reference,
// so this type stores only the year and the month and derives every getter from them.
type PlainYearMonth struct {
	year  int // proleptic Gregorian year, may be negative or above 9999
	month int // 1..12
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

// Year returns the ISO year.
func (ym *PlainYearMonth) Year() float64 { return float64(ym.year) }

// Month returns the ISO month, 1..12.
func (ym *PlainYearMonth) Month() float64 { return float64(ym.month) }

// CalendarId returns the calendar identifier, always "iso8601" for this slice.
func (ym *PlainYearMonth) CalendarId() BStr { return FromGoString("iso8601") }

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
	return ym.year == other.year && ym.month == other.month
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

// ToString implements Temporal.PlainYearMonth.prototype.toString for the default options:
// the ISO 8601 year-month, YYYY-MM, with the year expanded to a signed six-digit form
// outside 0..9999. The ISO calendar hides the reference day, so no day appears.
func (ym *PlainYearMonth) ToString() BStr {
	return FromGoString(formatISOYear(ym.year) + "-" + twoDigit(ym.month))
}

// ToJSON implements Temporal.PlainYearMonth.prototype.toJSON, the same ISO string toString
// produces under default options.
func (ym *PlainYearMonth) ToJSON() BStr { return ym.ToString() }

// PlainMonthDay is bento's runtime representation of a Temporal.PlainMonthDay (Temporal §10):
// a calendar month and day with no year, no time, and no zone, the way a birthday or a
// holiday recurs every year. Like PlainDate it hosts only the ISO 8601 calendar; a non-ISO
// calendar hands back at lowering. The specification anchors a month-day to a reference ISO
// year so a calendar can resolve which day the pair falls on; the ISO calendar needs it only
// to admit February 29, so this type stores the month and day and validates against the fixed
// leap reference year without keeping it.
type PlainMonthDay struct {
	month int // 1..12
	day   int // 1..isoDaysInMonth(monthDayReferenceYear, month)
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

// CalendarId returns the calendar identifier, always "iso8601" for this slice.
func (md *PlainMonthDay) CalendarId() BStr { return FromGoString("iso8601") }

// Equals implements Temporal.PlainMonthDay.prototype.equals: two month-days are equal when
// their month and day match under the same (ISO) calendar.
func (md *PlainMonthDay) Equals(other *PlainMonthDay) bool {
	return md.month == other.month && md.day == other.day
}

// ToString implements Temporal.PlainMonthDay.prototype.toString for the default options:
// the ISO 8601 month-day, MM-DD. The ISO calendar hides the reference year, so no year
// appears.
func (md *PlainMonthDay) ToString() BStr {
	return FromGoString(twoDigit(md.month) + "-" + twoDigit(md.day))
}

// ToJSON implements Temporal.PlainMonthDay.prototype.toJSON, the same ISO string toString
// produces under default options.
func (md *PlainMonthDay) ToJSON() BStr { return md.ToString() }

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
	return &ZonedDateTime{ns: new(big.Int).Set(z.ns), loc: z.loc, tzID: z.tzID}
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
// default compatible disambiguation, the option from over a bare string uses. An ordinary
// reading has one instant; a fall-back overlap takes the earlier of the two; a spring-forward
// gap, where the reading names no instant, shifts forward by the size of the gap and takes the
// reading there, the instant just after the transition.
func disambiguateCompatible(loc *time.Location, wall *big.Int) *big.Int {
	if p := possibleInstants(loc, wall); len(p) > 0 {
		return p[0]
	}
	sec := new(big.Int)
	rem := new(big.Int)
	sec.DivMod(wall, nsPerSecond, rem)
	wallSec := sec.Int64()
	offBefore := zoneOffsetSecondsAt(loc, wallSec-86_400)
	offAfter := zoneOffsetSecondsAt(loc, wallSec+86_400)
	gap := int64(offAfter-offBefore) * 1_000_000_000
	shifted := new(big.Int).Add(wall, big.NewInt(gap))
	if p := possibleInstants(loc, shifted); len(p) > 0 {
		return p[len(p)-1]
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
		date: PlainDate{year: year, month: month, day: day},
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

// CalendarId reports iso8601, the only calendar this slice hosts.
func (z *ZonedDateTime) CalendarId() BStr { return FromGoString("iso8601") }

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

// Equals implements Temporal.ZonedDateTime.prototype.equals for a ZonedDateTime argument: two
// zoned date-times are equal when they name the same instant in the same zone under the same
// calendar. The calendar is iso8601 on both, so the check is the count and the canonical zone
// identifier.
func (z *ZonedDateTime) Equals(other *ZonedDateTime) bool {
	return z.ns.Cmp(other.ns) == 0 && z.tzID.ToGoString() == other.tzID.ToGoString()
}

// ZonedDateTimeCompare implements Temporal.ZonedDateTime.compare: -1, 0, or 1 as the first
// instant is before, at, or after the second. The comparison is on the exact time only; the
// zone and calendar do not enter it.
func ZonedDateTimeCompare(a, b *ZonedDateTime) float64 { return float64(a.ns.Cmp(b.ns)) }

// ToString implements Temporal.ZonedDateTime.prototype.toString under the default options:
// the local ISO 8601 date-time, the UTC offset at this instant, and the time-zone identifier
// in brackets, the round-trippable form.
func (z *ZonedDateTime) ToString() BStr {
	dt := z.localDateTime()
	return FromGoString(dt.date.isoString() + "T" + dt.time.isoString() +
		formatOffset(z.offsetSeconds()) + "[" + z.tzID.ToGoString() + "]")
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
