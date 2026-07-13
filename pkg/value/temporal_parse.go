package value

import "strings"

// This file carries the runtime ISO 8601 / RFC 9557 string parser the Temporal from
// methods share. It parses the calendar-date grammar Temporal's ParseISODateTime
// accepts: a required date, an optional time with an optional sub-second fraction, an
// optional UTC offset or Z designator, and zero or more bracketed annotations, one of
// which may name a calendar. The parser only reads the syntax; each caller decides which
// fields it keeps and which forms it rejects, so PlainDate keeps the date and calendar
// and rejects a Z designator while a later Instant caller will keep the exact time.

// isoParse holds the components a Temporal ISO string yields. A caller reads the fields
// its type needs and ignores the rest: PlainDate reads year, month, day, and calendar.
type isoParse struct {
	year, month, day                     int
	hasTime                              bool
	hour, minute, second                 int
	millisecond, microsecond, nanosecond int
	hasZ                                 bool   // a Z (UTC) designator followed the time
	hasOffset                            bool   // a numeric UTC offset followed the time
	calendar                             string // the raw [u-ca=id] value, "" when none was given
}

// isoScanner walks a Temporal ISO string one byte at a time. The whole string must be
// consumed for a parse to succeed, so trailing text, or leading or trailing spaces, fail.
type isoScanner struct {
	s   string
	pos int
}

func (sc *isoScanner) atEnd() bool { return sc.pos >= len(sc.s) }

func (sc *isoScanner) peek() byte {
	if sc.atEnd() {
		return 0
	}
	return sc.s[sc.pos]
}

// accept consumes b if it is next, reporting whether it did.
func (sc *isoScanner) accept(b byte) bool {
	if sc.peek() == b {
		sc.pos++
		return true
	}
	return false
}

// digits reads exactly n decimal digits and returns their value, ok=false if fewer than
// n digits are available. It reads exactly n, so a field with too many digits leaves the
// extra for the next production to reject.
func (sc *isoScanner) digits(n int) (int, bool) {
	if sc.pos+n > len(sc.s) {
		return 0, false
	}
	v := 0
	for i := 0; i < n; i++ {
		c := sc.s[sc.pos+i]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	sc.pos += n
	return v, true
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// parseTemporalISOString parses s as a Temporal ISO date-time string, returning the
// components and ok=false for any syntax the grammar does not accept. It validates the
// shape only; range checks on the date and time are the caller's, matching the
// specification order where the grammar runs before RejectISODate.
func parseTemporalISOString(s string) (isoParse, bool) {
	sc := &isoScanner{s: s}
	var p isoParse
	if !sc.scanDate(&p) {
		return p, false
	}
	// An optional time follows a "T", a "t", or a single space separator.
	if c := sc.peek(); c == 'T' || c == 't' || c == ' ' {
		sc.pos++
		if !sc.scanTime(&p) {
			return p, false
		}
		p.hasTime = true
		sc.scanOffsetOrZ(&p)
	}
	if !sc.scanAnnotations(&p) {
		return p, false
	}
	if !sc.atEnd() {
		return p, false
	}
	return p, true
}

// scanDate reads the date: a signed six-digit expanded year or a four-digit year, then a
// month and a day, either extended with "-" separators or basic with none. The two forms
// do not mix, so once a "-" follows the year the month and day both take separators.
func (sc *isoScanner) scanDate(p *isoParse) bool {
	sign := 1
	expanded := false
	if sc.accept('+') {
		expanded = true
	} else if sc.accept('-') {
		expanded = true
		sign = -1
	}
	var year int
	if expanded {
		v, ok := sc.digits(6)
		if !ok {
			return false
		}
		year = sign * v
	} else {
		v, ok := sc.digits(4)
		if !ok {
			return false
		}
		year = v
	}
	extended := sc.accept('-')
	month, ok := sc.digits(2)
	if !ok {
		return false
	}
	if extended && !sc.accept('-') {
		return false
	}
	day, ok := sc.digits(2)
	if !ok {
		return false
	}
	p.year, p.month, p.day = year, month, day
	return true
}

// scanTime reads the time after the date separator: an hour, an optional minute, an
// optional second, and an optional sub-second fraction of one to nine digits. Like the
// date it is extended with ":" separators or basic with none, chosen by the first
// separator after the hour.
func (sc *isoScanner) scanTime(p *isoParse) bool {
	hour, ok := sc.digits(2)
	if !ok || hour > 23 {
		return false
	}
	p.hour = hour
	extended := false
	if sc.peek() == ':' {
		extended = true
		sc.pos++
	} else if !isDigit(sc.peek()) {
		return true // hour only
	}
	minute, ok := sc.digits(2)
	if !ok || minute > 59 {
		return false
	}
	p.minute = minute
	haveSecond := false
	if extended {
		if sc.accept(':') {
			haveSecond = true
		}
	} else if isDigit(sc.peek()) {
		haveSecond = true
	}
	if !haveSecond {
		return true // a fractional part follows the second, so without one there is none
	}
	second, ok := sc.digits(2)
	if !ok || second > 60 {
		return false
	}
	if second == 60 {
		second = 59 // a leap second is always constrained to :59, whatever the overflow option
	}
	p.second = second
	if c := sc.peek(); c == '.' || c == ',' {
		sc.pos++
		return sc.scanFraction(p)
	}
	return true
}

// scanFraction reads one to nine fractional-second digits after the decimal separator and
// spreads them across the millisecond, microsecond, and nanosecond fields, padding the
// missing low digits with zeros the way a nine-digit fraction would.
func (sc *isoScanner) scanFraction(p *isoParse) bool {
	start := sc.pos
	for !sc.atEnd() && isDigit(sc.s[sc.pos]) && sc.pos-start < 9 {
		sc.pos++
	}
	n := sc.pos - start
	if n == 0 {
		return false
	}
	if !sc.atEnd() && isDigit(sc.s[sc.pos]) {
		return false // a tenth fractional digit is out of range
	}
	frac := sc.s[start:sc.pos]
	for len(frac) < 9 {
		frac += "0"
	}
	nanos := 0
	for i := 0; i < 9; i++ {
		nanos = nanos*10 + int(frac[i]-'0')
	}
	p.millisecond = nanos / 1000000
	p.microsecond = (nanos / 1000) % 1000
	p.nanosecond = nanos % 1000
	return true
}

// scanOffsetOrZ reads an optional UTC designator or numeric offset after the time. It
// records which one appeared; the caller decides whether either is allowed, since a Plain
// type rejects a Z while an Instant requires an offset or a Z.
func (sc *isoScanner) scanOffsetOrZ(p *isoParse) {
	if c := sc.peek(); c == 'Z' || c == 'z' {
		sc.pos++
		p.hasZ = true
		return
	}
	if c := sc.peek(); c != '+' && c != '-' {
		return
	}
	save := sc.pos
	sc.pos++ // the sign
	if h, ok := sc.digits(2); !ok || h > 23 {
		sc.pos = save
		return
	}
	// Minutes and seconds follow either extended with ":" separators or basic as two
	// more digit pairs, each in 0..59. The offset value does not matter to a Plain type,
	// so only its shape is validated before hasOffset is recorded.
	if sc.accept(':') {
		if m, ok := sc.digits(2); !ok || m > 59 {
			sc.pos = save
			return
		}
		if sc.accept(':') {
			if s, ok := sc.digits(2); !ok || s > 59 {
				sc.pos = save
				return
			}
		}
	} else {
		if m, ok := sc.digits(2); ok {
			if m > 59 {
				sc.pos = save
				return
			}
			if s, ok := sc.digits(2); ok && s > 59 {
				sc.pos = save
				return
			}
		}
	}
	if c := sc.peek(); c == '.' || c == ',' {
		sc.pos++
		for !sc.atEnd() && isDigit(sc.s[sc.pos]) {
			sc.pos++
		}
	}
	p.hasOffset = true
}

// scanAnnotations reads zero or more bracketed RFC 9557 annotations. A bracket that carries
// a "key=value" pair is an annotation whose key must match the annotation-key grammar (a
// lowercase leading letter or underscore then lowercase letters, digits, underscores, or
// hyphens); the "u-ca" key names the calendar and the first one wins, while any other key is
// ignored unless a leading "!" marks it critical, in which case an unrecognised key is
// rejected. A bracket with no "=" is a time-zone annotation this parser records nothing from
// and accepts whether or not it is critical, since a Plain type drops the zone.
func (sc *isoScanner) scanAnnotations(p *isoParse) bool {
	for sc.peek() == '[' {
		sc.pos++
		critical := sc.accept('!')
		start := sc.pos
		for !sc.atEnd() && sc.s[sc.pos] != ']' {
			sc.pos++
		}
		if sc.atEnd() {
			return false // an unterminated bracket
		}
		body := sc.s[start:sc.pos]
		sc.pos++ // the closing ]
		if body == "" {
			return false
		}
		eq := strings.IndexByte(body, '=')
		if eq < 0 {
			continue // a time-zone annotation, dropped by every Plain type
		}
		key := body[:eq]
		val := body[eq+1:]
		if !validAnnotationKey(key) {
			return false
		}
		if key == "u-ca" {
			if val == "" {
				return false
			}
			if p.calendar == "" {
				p.calendar = val // the first calendar annotation wins
			}
			continue
		}
		if critical {
			return false // a critical annotation whose key is not understood is an error
		}
	}
	return true
}

// validAnnotationKey reports whether key matches the RFC 9557 annotation-key grammar: a
// leading lowercase letter or underscore followed by lowercase letters, digits, underscores,
// or hyphens. An uppercase or otherwise out-of-grammar key, such as "U-CA" or "FOO", makes the
// annotation, and so the whole string, invalid.
func validAnnotationKey(key string) bool {
	if key == "" {
		return false
	}
	for i := 0; i < len(key); i++ {
		c := key[i]
		lead := (c >= 'a' && c <= 'z') || c == '_'
		if i == 0 {
			if !lead {
				return false
			}
			continue
		}
		if !lead && (c < '0' || c > '9') && c != '-' {
			return false
		}
	}
	return true
}

// PlainDateFromString implements Temporal.PlainDate.from over a string: it parses the ISO
// date, applies the calendar annotation, and builds the PlainDate. A string the grammar
// rejects, a date outside the representable range, a Z designator (a Plain type has no
// zone to resolve it against), or a calendar bento does not host each throws a RangeError,
// matching the specification. The time, offset, and time-zone annotation a full date-time
// string may carry are parsed for validation and then dropped, since a PlainDate keeps
// only the date.
func PlainDateFromString(s string) *PlainDate {
	p, ok := parseTemporalISOString(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainDate")))
	}
	if p.hasZ {
		Throw(NewRangeError(FromGoString("a Temporal.PlainDate string cannot carry a Z designator")))
	}
	rejectISODate(float64(p.year), float64(p.month), float64(p.day))
	cal := ""
	if p.calendar != "" {
		c, hosted := canonicalCalendar(p.calendar)
		if !hosted {
			Throw(NewRangeError(FromGoString("invalid calendar identifier " + p.calendar)))
		}
		cal = c
	}
	return &PlainDate{year: p.year, month: p.month, day: p.day, cal: cal}
}

// parseTemporalTimeOnly parses a time-only Temporal string, the form Temporal.PlainTime.from
// accepts when the string carries no date: an optional "T" or "t" designator, a time, an
// optional offset, and zero or more annotations. When no designator is present a bare
// basic-format time whose digits are byte-identical to a valid year-month or month-day date,
// "1230" reading as 12-30 or "120512" reading as 1205-12, is ambiguous and rejected, matching
// the grammar's not-ambiguous constraint; a fraction, an offset, or the ":" separators of the
// extended form all break the tie in the time's favour, and the designator removes it outright.
func parseTemporalTimeOnly(s string) (isoParse, bool) {
	sc := &isoScanner{s: s}
	var p isoParse
	designator := false
	if c := sc.peek(); c == 'T' || c == 't' {
		sc.pos++
		designator = true
	}
	specStart := sc.pos
	if !sc.scanTime(&p) {
		return p, false
	}
	specEnd := sc.pos
	sc.scanOffsetOrZ(&p)
	afterOffset := sc.pos
	if !sc.scanAnnotations(&p) {
		return p, false
	}
	if !sc.atEnd() {
		return p, false
	}
	p.hasTime = true
	// Only a bare time with nothing after its spec but annotations can collide with a date,
	// so an offset (which moved the position past the spec) already breaks the ambiguity.
	if !designator && specEnd == afterOffset && ambiguousTimeSpec(s[specStart:specEnd]) {
		return p, false
	}
	return p, true
}

// ambiguousTimeSpec reports whether spec, the exact bytes of a basic-format time with no
// separators, no fraction, and no offset, is byte-identical to a valid ISO date and so cannot
// be told apart from one without a time designator. Four digits "HHMM" collide with a month-day
// "MMDD" and six digits "HHMMSS" collide with a year-month "YYYYMM"; the month-day check uses
// the 1972 leap reference year so February 29 counts. Any other length, or a non-digit byte
// from a separator or fraction, is unambiguous.
func ambiguousTimeSpec(spec string) bool {
	for i := 0; i < len(spec); i++ {
		if !isDigit(spec[i]) {
			return false
		}
	}
	switch len(spec) {
	case 4:
		month := int(spec[0]-'0')*10 + int(spec[1]-'0')
		day := int(spec[2]-'0')*10 + int(spec[3]-'0')
		return month >= 1 && month <= 12 && day >= 1 && day <= isoDaysInMonth(1972, month)
	case 6:
		month := int(spec[4]-'0')*10 + int(spec[5]-'0')
		return month >= 1 && month <= 12
	}
	return false
}

// timeFromParse builds a PlainTime from the parsed time fields, the components shared by the
// time-only and the full date-time forms once the caller has decided the string is a time.
func timeFromParse(p isoParse) *PlainTime {
	return &PlainTime{p.hour, p.minute, p.second, p.millisecond, p.microsecond, p.nanosecond}
}

// PlainTimeFromString implements Temporal.PlainTime.from over a string. It reads either a full
// date-time string, whose date it validates and whose time it keeps, or a time-only string; a
// date-only string with no time, a Z designator (a wall-clock time has no zone to resolve it
// against), or a syntax the grammar rejects each throws a RangeError. A calendar annotation is
// accepted and ignored whatever it names, since a PlainTime carries no calendar, so an unhosted
// or unknown identifier is not an error the way it is for a PlainDate.
func PlainTimeFromString(s string) *PlainTime {
	if p, ok := parseTemporalISOString(s); ok {
		if p.hasZ {
			Throw(NewRangeError(FromGoString("a Temporal.PlainTime string cannot carry a Z designator")))
		}
		if !p.hasTime {
			Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainTime: no time")))
		}
		rejectISODate(float64(p.year), float64(p.month), float64(p.day))
		return timeFromParse(p)
	}
	p, ok := parseTemporalTimeOnly(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainTime")))
	}
	if p.hasZ {
		Throw(NewRangeError(FromGoString("a Temporal.PlainTime string cannot carry a Z designator")))
	}
	return timeFromParse(p)
}

// PlainDateTimeFromString implements Temporal.PlainDateTime.from over a string. It reads a
// date, optionally followed by a time it keeps, so a date-only string like "2024-06-30" is
// accepted with the time defaulting to midnight while a full date-time string keeps its time.
// A grammar the parser rejects, a date outside the representable range, a Z designator (a
// Plain type has no zone to resolve it against), or a calendar bento does not host each
// throws a RangeError. A time-only string with no date is rejected, since the grammar this
// method accepts always begins with a date. The offset and time-zone annotation a string may
// carry are parsed for validation and then dropped.
func PlainDateTimeFromString(s string) *PlainDateTime {
	p, ok := parseTemporalISOString(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainDateTime")))
	}
	if p.hasZ {
		Throw(NewRangeError(FromGoString("a Temporal.PlainDateTime string cannot carry a Z designator")))
	}
	rejectISODate(float64(p.year), float64(p.month), float64(p.day))
	cal := ""
	if p.calendar != "" {
		c, hosted := canonicalCalendar(p.calendar)
		if !hosted {
			Throw(NewRangeError(FromGoString("invalid calendar identifier " + p.calendar)))
		}
		cal = c
	}
	return &PlainDateTime{
		date: PlainDate{year: p.year, month: p.month, day: p.day, cal: cal},
		time: PlainTime{p.hour, p.minute, p.second, p.millisecond, p.microsecond, p.nanosecond},
	}
}

// parseTemporalYearMonthOnly parses a bare year-month Temporal string, the form
// Temporal.PlainYearMonth.from accepts when the string carries no day: a year, a month in the
// extended "YYYY-MM" or basic "YYYYMM" form, and zero or more annotations, with no time and no
// day. It is the year-month counterpart of the date the full parser requires, so "2024-06" and
// "202406" parse here while a day, a time, or a "T" designator either routes the string to the
// full parser or fails outright.
func parseTemporalYearMonthOnly(s string) (isoParse, bool) {
	sc := &isoScanner{s: s}
	var p isoParse
	sign := 1
	expanded := false
	if sc.accept('+') {
		expanded = true
	} else if sc.accept('-') {
		expanded = true
		sign = -1
	}
	var year int
	if expanded {
		v, ok := sc.digits(6)
		if !ok {
			return p, false
		}
		year = sign * v
	} else {
		v, ok := sc.digits(4)
		if !ok {
			return p, false
		}
		year = v
	}
	sc.accept('-')
	month, ok := sc.digits(2)
	if !ok {
		return p, false
	}
	p.year, p.month = year, month
	if !sc.scanAnnotations(&p) {
		return p, false
	}
	if !sc.atEnd() {
		return p, false
	}
	return p, true
}

// yearMonthRequireISO throws a RangeError unless cal, the calendar annotation a year-month
// string carried, is empty or names the ISO calendar. bento's PlainYearMonth is ISO-only, so a
// bare year-month string naming another calendar is an error the way the specification treats
// it, and the lowerer hands back any literal naming a non-ISO calendar before this is reached,
// so at run time cal is always "" or "iso8601".
func yearMonthRequireISO(cal string) {
	if cal != "" && !strings.EqualFold(cal, "iso8601") {
		Throw(NewRangeError(FromGoString("Temporal.PlainYearMonth from a string supports only the iso8601 calendar")))
	}
}

// PlainYearMonthFromString implements Temporal.PlainYearMonth.from over a string. It reads a
// bare year-month string like "2024-06", whose day the type does not carry, or a full date or
// date-time string like "2024-06-30", whose year and month it keeps and whose day and time it
// drops. A grammar the parser rejects, an out-of-range year-month, an out-of-range day on a
// full-date string, a Z designator, or a non-ISO calendar each throws a RangeError. The
// year-month-only form carries no time, so a "T" designator or a space after the month sends
// the string to the full parser, which needs a day, and both failing throws.
func PlainYearMonthFromString(s string) *PlainYearMonth {
	if p, ok := parseTemporalISOString(s); ok {
		if p.hasZ {
			Throw(NewRangeError(FromGoString("a Temporal.PlainYearMonth string cannot carry a Z designator")))
		}
		rejectISODate(float64(p.year), float64(p.month), float64(p.day))
		yearMonthRequireISO(p.calendar)
		rejectISOYearMonth(float64(p.year), float64(p.month))
		return &PlainYearMonth{year: p.year, month: p.month}
	}
	p, ok := parseTemporalYearMonthOnly(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainYearMonth")))
	}
	yearMonthRequireISO(p.calendar)
	rejectISOYearMonth(float64(p.year), float64(p.month))
	return &PlainYearMonth{year: p.year, month: p.month}
}

// parseTemporalMonthDayOnly parses a bare month-day Temporal string, the form
// Temporal.PlainMonthDay.from accepts when the string carries no year: an optional "--" prefix
// (the grammar's TwoDashes), a month, an optional "-", a day, and zero or more annotations,
// with no year and no time. So "10-01", "1001", "--10-01", and "--1001" all parse here, while a
// single leading dash, a year, or a time either routes the string to the full parser or fails.
// The grammar bounds the day at two digits with no per-month check, so the caller admits any day
// in 1..31 for the yearless form, matching the specification's yearless month-day validation.
func parseTemporalMonthDayOnly(s string) (isoParse, bool) {
	sc := &isoScanner{s: s}
	var p isoParse
	// An optional leading "--" precedes the month; a single leading dash is not a valid prefix,
	// so once one dash is seen the second is required.
	if sc.peek() == '-' {
		sc.pos++
		if !sc.accept('-') {
			return p, false
		}
	}
	month, ok := sc.digits(2)
	if !ok {
		return p, false
	}
	sc.accept('-')
	day, ok := sc.digits(2)
	if !ok {
		return p, false
	}
	p.month, p.day = month, day
	if !sc.scanAnnotations(&p) {
		return p, false
	}
	if !sc.atEnd() {
		return p, false
	}
	return p, true
}

// monthDayRequireISO throws a RangeError unless cal, the calendar annotation a month-day string
// carried, is empty or names the ISO calendar. bento's PlainMonthDay is ISO-only, so a bare
// month-day string naming another calendar is an error the way the specification treats it, and
// the lowerer hands back any literal naming a non-ISO calendar before this is reached, so at run
// time cal is always "" or "iso8601".
func monthDayRequireISO(cal string) {
	if cal != "" && !strings.EqualFold(cal, "iso8601") {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay from a string supports only the iso8601 calendar")))
	}
}

// rejectMonthDayStringRange throws a RangeError unless (month, day) is in the range the yearless
// month-day grammar admits: the month in 1..12 and the day in 1..31. Unlike rejectISOMonthDay,
// which the constructor uses to bound the day by the reference year's month length, the yearless
// string form has no year to measure against, so "02-30" and "06-31" are accepted the way the
// specification and the reference implementation accept them.
func rejectMonthDayStringRange(month, day float64) {
	if month < 1 || month > 12 {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay month must be between 1 and 12")))
	}
	if day < 1 || day > 31 {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay day must be between 1 and 31")))
	}
}

// rejectMonthDayFullDate throws a RangeError unless (month, day) is a real day of that month in
// the parsed year: the month in 1..12 and the day within the month's length, with the year read
// only for February's leap length. Unlike rejectISODate it does not range-check the year, since
// a month-day drops the year and keeps only the month and day, so an expanded-year full date like
// "-999999-10-01" is accepted the way the specification and the reference implementation accept
// it while "2024-06-31" and "2021-02-29" still throw.
func rejectMonthDayFullDate(year, month, day float64) {
	if month < 1 || month > 12 {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay month must be between 1 and 12")))
	}
	y, m, d := int(year), int(month), int(day)
	if d < 1 || d > isoDaysInMonth(y, m) {
		Throw(NewRangeError(FromGoString("Temporal.PlainMonthDay day is out of range for the month")))
	}
}

// PlainMonthDayFromString implements Temporal.PlainMonthDay.from over a string. It reads a bare
// month-day string like "10-01" or "--10-01", whose year the type does not carry, or a full date
// or date-time string like "1976-10-01", whose month and day it keeps and whose year and time it
// drops. A grammar the parser rejects, an out-of-range month-day, an out-of-range day on a
// full-date string, a Z designator, or a non-ISO calendar each throws a RangeError. The full-date
// form validates the day against its real year, so "2024-06-31" throws, while the yearless form
// has no year and admits any day in 1..31, so "06-31" parses, matching the specification.
func PlainMonthDayFromString(s string) *PlainMonthDay {
	if p, ok := parseTemporalISOString(s); ok {
		if p.hasZ {
			Throw(NewRangeError(FromGoString("a Temporal.PlainMonthDay string cannot carry a Z designator")))
		}
		rejectMonthDayFullDate(float64(p.year), float64(p.month), float64(p.day))
		monthDayRequireISO(p.calendar)
		return &PlainMonthDay{month: p.month, day: p.day}
	}
	p, ok := parseTemporalMonthDayOnly(s)
	if !ok {
		Throw(NewRangeError(FromGoString("cannot parse " + s + " as a Temporal.PlainMonthDay")))
	}
	monthDayRequireISO(p.calendar)
	rejectMonthDayStringRange(float64(p.month), float64(p.day))
	return &PlainMonthDay{month: p.month, day: p.day}
}
