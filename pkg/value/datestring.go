package value

// This file spells a Date as text: the five formats besides toISOString that a program
// reads a date through. They divide into two families and one oddity.
//
// The human family is toString, toDateString and toTimeString, which read the date in
// local time and are built from three pieces the specification names: DateString, the
// weekday and calendar date; TimeString, the clock; and TimeZoneString, the offset and
// the zone's name. toString is all three, and the other two are the halves, which is why
// they are written once here and composed rather than formatted three times over.
//
// The wire family is toUTCString, the format HTTP dates are written in. It reads UTC,
// puts the day before the month, and has a comma after the weekday, none of which the
// human family does; it is not a rearrangement of the same pieces and so is its own
// function.
//
// The oddity is toJSON, which is toISOString except that it answers null rather than
// throwing when the date is invalid. That is what makes JSON.stringify of an invalid
// date produce null instead of failing, and it is why this one returns a Value.

import (
	"math"
	"time"
)

// invalidDateText is what every human-readable format spells an unrepresentable date as.
// The Invalid Date is not an error state: it prints, it compares, and only the ISO
// format refuses it, so each format below returns this rather than throwing.
const invalidDateText = "Invalid Date"

// weekdayNames and monthNames are the three-letter English abbreviations every one of
// these formats uses. They are not locale-sensitive: toString is specified in English
// and stays in English wherever the program runs, which is what makes a date written to
// a log file readable by the machine that reads the log.
var weekdayNames = [7]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

var monthNames = [12]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// dateStringOf is the specification's DateString: the weekday, the month, the day of the
// month and the year, as in "Sun Jan 15 2023". The year is padded to four digits and
// carries a minus sign when it is negative, so the year before year one reads "-0001"
// rather than "-1"; that is the same padding the ISO format uses and it keeps the field
// widths of a printed date fixed.
func dateStringOf(p timeParts) string {
	year := p.year
	sign := ""
	if year < 0 {
		sign = "-"
		year = -year
	}
	return weekdayNames[p.weekday] + " " + monthNames[p.month-1] + " " + twoDigit(p.day) +
		" " + sign + zeroPad(year, 4)
}

// timeStringOf is the specification's TimeString: the clock to the second, followed by
// the literal " GMT" that the zone offset attaches to. The offset is not part of this
// piece even though the "GMT" is, which looks like a seam in the wrong place until you
// see the two composed: "03:04:05 GMT" and "+0000 (...)" join into the reading a program
// prints.
func timeStringOf(p timeParts) string {
	return twoDigit(p.hour) + ":" + twoDigit(p.minute) + ":" + twoDigit(p.second) + " GMT"
}

// timeZoneStringOf is the specification's TimeZoneString: the offset from UTC in the
// four-digit form, then the zone's long name in parentheses. The name is the part no
// tzdata carries, so it comes from the generated CLDR table; a zone that table does not
// name spells the offset again in the GMT+HH:MM form, which is what Node prints for an
// unnamed zone such as Etc/GMT+7.
func timeZoneStringOf(t time.Time) string {
	_, offsetSec := t.Zone()
	sign := "+"
	if offsetSec < 0 {
		sign = "-"
		offsetSec = -offsetSec
	}
	hours, minutes := offsetSec/3600, (offsetSec%3600)/60
	name := zoneLongName(t)
	if name == "" {
		name = "GMT" + sign + twoDigit(hours) + ":" + twoDigit(minutes)
	}
	return sign + twoDigit(hours) + twoDigit(minutes) + " (" + name + ")"
}

// zoneLongName is the zone's CLDR name at this instant, or the empty string for a zone
// CLDR does not name. The daylight side is picked by the flag Go's tzdata sets on the
// instant rather than by comparing offsets, so a southern-hemisphere zone in January
// reads as daylight the way Node reads it.
//
// A zone the table does not carry at all is answered as unnamed. That happens when Go
// cannot name the host's local zone, which it reports as "Local"; the offset form the
// caller falls back to is still the right offset, so the reading is correct and only the
// parenthesized name differs from what Node prints.
func zoneLongName(t time.Time) string {
	pair, ok := zoneLongNames[t.Location().String()]
	if !ok {
		return ""
	}
	if t.IsDST() {
		return zoneNameText[pair.daylight]
	}
	return zoneNameText[pair.standard]
}

// localTimeAt is the instant as Go's time package sees it in the local zone, the value
// the zone lookups read. The date's own component reads go through localParts, which
// shifts the time value instead; both are the same instant, and this one exists because
// naming a zone needs the zone object and not just the offset.
func (d *Date) localTimeAt() time.Time {
	return time.UnixMilli(int64(d.ms)).Local()
}

// ToString is the whole human-readable reading in local time, the lowering of
// date.toString() and the text a date coerces to in a string context. It is the one
// format a program gets without asking, since a Date coerces to its string form rather
// than to its number in ordinary concatenation.
func (d *Date) ToString() BStr {
	if math.IsNaN(d.ms) {
		return FromGoString(invalidDateText)
	}
	p := d.localParts()
	return FromGoString(dateStringOf(p) + " " + timeStringOf(p) + timeZoneStringOf(d.localTimeAt()))
}

// ToDateString is the calendar half of the local reading, the lowering of
// date.toDateString(): "Sun Jan 15 2023", with no clock and no zone.
func (d *Date) ToDateString() BStr {
	if math.IsNaN(d.ms) {
		return FromGoString(invalidDateText)
	}
	return FromGoString(dateStringOf(d.localParts()))
}

// ToTimeString is the clock half, the lowering of date.toTimeString(). It carries the
// zone with it, since a time of day with no zone names no instant.
func (d *Date) ToTimeString() BStr {
	if math.IsNaN(d.ms) {
		return FromGoString(invalidDateText)
	}
	return FromGoString(timeStringOf(d.localParts()) + timeZoneStringOf(d.localTimeAt()))
}

// ToUTCString is the date in UTC in the format HTTP headers use, the lowering of
// date.toUTCString(): "Sun, 15 Jan 2023 03:04:05 GMT". The comma after the weekday and
// the day before the month are what separate it from the human family, and they are the
// reason it is written out here rather than composed from the same pieces.
func (d *Date) ToUTCString() BStr {
	if math.IsNaN(d.ms) {
		return FromGoString(invalidDateText)
	}
	p := splitTimeValue(d.ms)
	year := p.year
	sign := ""
	if year < 0 {
		sign = "-"
		year = -year
	}
	return FromGoString(weekdayNames[p.weekday] + ", " + twoDigit(p.day) + " " + monthNames[p.month-1] +
		" " + sign + zeroPad(year, 4) + " " + twoDigit(p.hour) + ":" + twoDigit(p.minute) +
		":" + twoDigit(p.second) + " GMT")
}

// ToJSON is what JSON.stringify serializes a date as, the lowering of date.toJSON(). It
// is toISOString for a representable date and the value null for the Invalid Date, which
// is the whole reason it exists as its own method: toISOString throws there, and a
// program serializing a record that happens to hold an unparsable date gets null in the
// output rather than an exception out of JSON.stringify.
//
// It answers a Value rather than a BStr because null is one of the two answers. The
// checker types the method as returning a string, which is a lie the standard library
// tells; the lowering marks the call dynamic so the truthful value flows into a dynamic
// sink and a string slot hands the build back rather than taking null as "".
func (d *Date) ToJSON() Value {
	if math.IsNaN(d.ms) {
		return Null
	}
	return StringValue(d.ToISOString())
}

// DateToJSON is ToJSON reached as a function, the shape the lowerer emits so the call
// site does not have to name a method on a value whose Go type it is boxing away.
func DateToJSON(d *Date) Value { return d.ToJSON() }
