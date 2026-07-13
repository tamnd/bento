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
	if haveSecond {
		second, ok := sc.digits(2)
		if !ok || second > 59 {
			return false
		}
		p.second = second
	}
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

// scanAnnotations reads zero or more bracketed RFC 9557 annotations. A bracket that
// carries a "key=value" pair whose key is "u-ca" names the calendar; a bracket with no
// "=" is a time-zone annotation this parser records nothing from. A leading "!" marks a
// critical annotation and is accepted the same way.
func (sc *isoScanner) scanAnnotations(p *isoParse) bool {
	for sc.peek() == '[' {
		sc.pos++
		sc.accept('!')
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
		if eq := strings.IndexByte(body, '='); eq >= 0 {
			key := body[:eq]
			val := body[eq+1:]
			if key == "u-ca" {
				if val == "" {
					return false
				}
				p.calendar = val
			}
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
