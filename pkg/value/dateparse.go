package value

// This file parses a date out of a string, the input behind new Date(s) and Date.parse.
//
// The specification pins exactly one format, the ISO 8601 profile in 21 §21.4.1.15, and
// leaves every other spelling to the implementation. So the ISO grammar is written out
// here in full and matched strictly, and a small set of the formats a JavaScript program
// actually meets in the wild is tried after it: the HTTP and mail date, which is what
// toUTCString prints, and the toString spelling, which is what a date that was turned
// into a string and back arrives as.
//
// Anything else is the Invalid Date. That is a real answer rather than a stub: an
// unparsable string is specified to give NaN, and guessing at a spelling would be how a
// program silently reads January when the string said something else.

import (
	"math"
	"strings"
	"time"
)

// ParseDate is the time value a string names, or NaN when the string names no date. It
// is the whole of Date.parse and the string half of the Date constructor.
func ParseDate(s BStr) float64 {
	str := strings.TrimSpace(s.ToGoString())
	if ms, ok := parseISODateTime(str); ok {
		return timeClip(ms)
	}
	if ms, ok := parseLegacyDate(str); ok {
		return timeClip(ms)
	}
	return math.NaN()
}

// NewDateFromString builds a Date from a string, the lowering of new Date(s). A string
// that names no date gives the Invalid Date rather than throwing, which is what makes a
// bad date something a program has to check for with isNaN rather than catch.
func NewDateFromString(s BStr) *Date {
	return &Date{ms: ParseDate(s)}
}

// dateScanner walks a date string a field at a time. The grammar is fixed-width almost
// everywhere, so the scanner reads counted digits and single separators rather than
// tokenizing.
type dateScanner struct {
	s string
	i int
}

// digits reads exactly n digits and their value, reporting false if there are not n
// digits at the cursor. The count is exact on purpose: the ISO profile pads every field,
// so a two-digit month is written 01 and a lone 1 is not a month at all.
func (p *dateScanner) digits(n int) (int, bool) {
	if p.i+n > len(p.s) {
		return 0, false
	}
	v := 0
	for k := 0; k < n; k++ {
		c := p.s[p.i+k]
		if c < '0' || c > '9' {
			return 0, false
		}
		v = v*10 + int(c-'0')
	}
	p.i += n
	return v, true
}

// accept consumes one byte if it is the one expected.
func (p *dateScanner) accept(c byte) bool {
	if p.i < len(p.s) && p.s[p.i] == c {
		p.i++
		return true
	}
	return false
}

// peek is the byte at the cursor, or zero at the end of the string.
func (p *dateScanner) peek() byte {
	if p.i < len(p.s) {
		return p.s[p.i]
	}
	return 0
}

// done reports whether the whole string was consumed. A date string with anything left
// over is not a date: trailing text is how a typo silently parses as a valid instant.
func (p *dateScanner) done() bool { return p.i == len(p.s) }

// parseISODateTime matches the format the specification pins, the ISO 8601 profile. The
// date alone is YYYY, YYYY-MM or YYYY-MM-DD, with a signed six-digit year for the
// expanded range; the time that may follow is HH:mm with optional seconds and a fraction,
// and an optional offset.
//
// The one rule that is easy to miss and impossible to guess at is which zone a string
// without an offset means: a date-only string is UTC, and a date-and-time string is local
// time. So "2023-11-14" and "2023-11-14T00:00:00" are different instants everywhere but
// Greenwich, and that is the specified behavior rather than an accident.
func parseISODateTime(s string) (float64, bool) {
	p := &dateScanner{s: s}
	year, ok := parseISOYearField(p)
	if !ok {
		return 0, false
	}
	month, day := 1, 1
	if p.accept('-') {
		if month, ok = p.digits(2); !ok || month < 1 || month > 12 {
			return 0, false
		}
		if p.accept('-') {
			if day, ok = p.digits(2); !ok || day < 1 || day > isoDaysInMonth(year, month) {
				return 0, false
			}
		}
	}
	dateOnly := true
	var hour, minute, second, milli int
	if p.accept('T') {
		dateOnly = false
		if hour, minute, second, milli, ok = parseISOTimeField(p); !ok {
			return 0, false
		}
	}
	offsetMs, hasOffset, ok := parseISOOffsetField(p)
	if !ok || !p.done() {
		return 0, false
	}
	// A date-only string carrying an offset is outside the grammar; the specification
	// admits an offset only after a time.
	if dateOnly && hasOffset {
		return 0, false
	}
	if !dateOnly && !hasOffset {
		return localWallClockToTimeValue(year, month, day, hour, minute, second, milli), true
	}
	utc := float64(isoToEpochDays(year, month, day))*86_400_000 +
		float64(hour)*3_600_000 + float64(minute)*60_000 + float64(second)*1_000 + float64(milli)
	return utc - float64(offsetMs), true
}

// parseISOYearField reads the year, either the four-digit form or the signed six-digit
// expanded form. A negative zero year is excluded because it would name year zero twice,
// once with each sign, which the grammar forbids.
func parseISOYearField(p *dateScanner) (int, bool) {
	sign := 0
	if p.accept('+') {
		sign = 1
	} else if p.accept('-') {
		sign = -1
	}
	if sign == 0 {
		return p.digits(4)
	}
	y, ok := p.digits(6)
	if !ok || (sign < 0 && y == 0) {
		return 0, false
	}
	return sign * y, true
}

// parseISOTimeField reads HH:mm, with optional :ss and an optional fractional second.
// The hour may be 24 only at the exact start of the next day, the one spelling of
// midnight that names the end of a day rather than its beginning. The fraction is read to
// millisecond resolution and any further digits are dropped, since a time value has no
// room for them.
func parseISOTimeField(p *dateScanner) (hour, minute, second, milli int, ok bool) {
	if hour, ok = p.digits(2); !ok || hour > 24 {
		return 0, 0, 0, 0, false
	}
	if !p.accept(':') {
		return 0, 0, 0, 0, false
	}
	if minute, ok = p.digits(2); !ok || minute > 59 {
		return 0, 0, 0, 0, false
	}
	if p.accept(':') {
		if second, ok = p.digits(2); !ok || second > 59 {
			return 0, 0, 0, 0, false
		}
		if p.accept('.') {
			scale := 100
			read := 0
			for c := p.peek(); c >= '0' && c <= '9'; c = p.peek() {
				p.i++
				read++
				if scale >= 1 {
					milli += int(c-'0') * scale
					scale /= 10
				}
			}
			if read == 0 {
				return 0, 0, 0, 0, false
			}
		}
	}
	if hour == 24 && (minute|second|milli) != 0 {
		return 0, 0, 0, 0, false
	}
	return hour, minute, second, milli, true
}

// parseISOOffsetField reads the zone, either Z or a signed hour and minute. It reports
// whether an offset was present at all, since an absent one is not the same as +00:00:
// the first means local time and the second means UTC.
func parseISOOffsetField(p *dateScanner) (offsetMs int, hasOffset, ok bool) {
	if p.accept('Z') || p.accept('z') {
		return 0, true, true
	}
	sign := 0
	if p.accept('+') {
		sign = 1
	} else if p.accept('-') {
		sign = -1
	}
	if sign == 0 {
		return 0, false, true
	}
	h, ok := p.digits(2)
	if !ok || h > 23 {
		return 0, false, false
	}
	// The colon is optional: the profile writes +05:30 and the wild writes +0530.
	p.accept(':')
	m, ok := p.digits(2)
	if !ok || m > 59 {
		return 0, false, false
	}
	return sign * (h*3_600_000 + m*60_000), true, true
}

// localWallClockToTimeValue is the instant a local wall clock reading names. Go's own
// time.Date does the zone lookup, which matters for the two days a year that a zone with
// daylight saving skips or repeats an hour: the arithmetic a fixed offset would do lands
// on the wrong instant on both of them.
func localWallClockToTimeValue(year, month, day, hour, minute, second, milli int) float64 {
	t := time.Date(year, time.Month(month), day, hour, minute, second, milli*1_000_000, time.Local)
	return float64(t.UnixMilli())
}

// legacyDateLayouts are the non-ISO spellings this runtime reads. They are the two a
// JavaScript program actually meets: the mail and HTTP date, which is what toUTCString
// prints and what a server sends, and the toString spelling, which is what a date that
// went through a string and back arrives as. The rest of the historical zoo is not read,
// and says so by giving the Invalid Date rather than guessing.
var legacyDateLayouts = []string{
	"Mon, 02 Jan 2006 15:04:05 GMT",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"02 Jan 2006 15:04:05 GMT",
	"2 Jan 2006 15:04:05 -0700",
	"Mon Jan 02 2006 15:04:05 GMT-0700",
}

// parseLegacyDate tries the non-ISO spellings. The toString format carries a trailing
// zone name in parentheses that no layout can match, so it is cut before the layouts run;
// the offset that precedes it is what fixes the instant anyway.
func parseLegacyDate(s string) (float64, bool) {
	if i := strings.LastIndex(s, " ("); i > 0 && strings.HasSuffix(s, ")") {
		s = s[:i]
	}
	for _, layout := range legacyDateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return float64(t.UnixMilli()), true
		}
	}
	return 0, false
}
