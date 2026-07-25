package value

// This file builds a time value out of calendar components, the direction the rest of
// the Date surface was missing. It is what new Date(y, m, d, ...) and Date.UTC construct
// with, and what every setter rebuilds the date with after replacing a field.
//
// The rule that makes this more than arithmetic is that components do not have to be in
// range. Month 12 is January of the next year, day 0 is the last day of the previous
// month, and hour 25 is one in the morning of the next day. That is not leniency; it is
// how a program is supposed to do date arithmetic, so `d.setDate(d.getDate() + 45)` lands
// two months later without the caller counting month lengths.

import "math"

// componentCount is the number of calendar fields a date is built from, in the order the
// setters replace them: year, month, day, hour, minute, second, millisecond.
const componentCount = 7

// The index of each field in a component list. The setters name a starting field and
// write forward from it, which is exactly what setHours(h, m, s, ms) does.
const (
	fieldYear = iota
	fieldMonth
	fieldDay
	fieldHour
	fieldMinute
	fieldSecond
	fieldMilli
)

// timeValueFromComponents is the time value a calendar reading names, in UTC or in local
// time. Every component is truncated toward zero the way ToIntegerOrInfinity does, and an
// out-of-range one carries into the field above it rather than being rejected.
//
// A component that is not a finite number makes the whole reading the Invalid Date. That
// propagates rather than defaulting, because a date silently built from a zero where the
// caller passed a NaN is the wrong instant reported as a right one.
func timeValueFromComponents(c [componentCount]float64, utc bool) float64 {
	for _, v := range c {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			return math.NaN()
		}
	}
	year := int(math.Trunc(c[fieldYear]))
	month := int(math.Trunc(c[fieldMonth]))
	// The month carries into the year, which is the only overflow that cannot be done by
	// plain addition: months are not all the same length, so the year has to be settled
	// before a day count exists at all.
	year += floorDiv(month, 12)
	month -= floorDiv(month, 12) * 12

	days := isoToEpochDays(year, month+1, 1) + int(math.Trunc(c[fieldDay])) - 1
	ms := float64(days)*86_400_000 +
		math.Trunc(c[fieldHour])*3_600_000 +
		math.Trunc(c[fieldMinute])*60_000 +
		math.Trunc(c[fieldSecond])*1_000 +
		math.Trunc(c[fieldMilli])
	if utc {
		return ms
	}
	return utcReadingAsLocal(ms)
}

// utcReadingAsLocal turns a wall clock reading, computed as though it were UTC, into the
// instant it names in the local zone. The parts are split back out and handed to
// time.Date, which does the zone lookup and so gets the two days a year a zone skips or
// repeats an hour right; fixed-offset arithmetic gets both of them wrong.
//
// A reading far outside the representable range is the Invalid Date before the split,
// since there is no instant to look a zone up at. The margin covers the largest offset any
// zone has ever used, so a reading that only a zone shift could bring back into range is
// still converted.
func utcReadingAsLocal(ms float64) float64 {
	if math.Abs(ms) > maxTimeValue+2*86_400_000 {
		return math.NaN()
	}
	p := splitTimeValue(ms)
	return localWallClockToTimeValue(p.year, p.month, p.day, p.hour, p.minute, p.second, p.milli)
}

// legacyYear is the two-digit year rule: a year of 0 through 99 means the nineteen
// hundreds. It applies to the constructor and to Date.UTC and to nothing else, which is
// why setFullYear can set a date to the year 5 and the constructor cannot.
func legacyYear(y float64) float64 {
	if yi := math.Trunc(y); !math.IsNaN(yi) && yi >= 0 && yi <= 99 {
		return 1900 + yi
	}
	return y
}

// componentsWithDefaults fills a component list from the arguments a caller passed. The
// day defaults to the first of the month and the time to midnight, which is what makes
// new Date(2023, 0) the start of January rather than an error.
func componentsWithDefaults(args []float64) [componentCount]float64 {
	c := [componentCount]float64{0, 0, 1, 0, 0, 0, 0}
	for i, v := range args {
		if i >= componentCount {
			break
		}
		c[i] = v
	}
	return c
}

// NewDateFromComponents builds a Date from a local calendar reading, the lowering of
// new Date(year, month, day, ...). The reading is local time, not UTC, which is the one
// thing about this constructor that surprises people: new Date(2023, 0, 1) is midnight
// where the program runs, and its ISO string is a different day west of Greenwich.
func NewDateFromComponents(args ...float64) *Date {
	c := componentsWithDefaults(args)
	c[fieldYear] = legacyYear(c[fieldYear])
	return &Date{ms: timeClip(timeValueFromComponents(c, false))}
}

// DateUTC is the time value a UTC calendar reading names, the lowering of Date.UTC. It is
// the same construction as the component constructor with the zone left out, and like
// Date.now it gives a Number rather than a Date.
func DateUTC(args ...float64) float64 {
	c := componentsWithDefaults(args)
	c[fieldYear] = legacyYear(c[fieldYear])
	return timeClip(timeValueFromComponents(c, true))
}

// components takes this date apart into the list a setter writes into, in UTC or in local
// time. The month comes back zero-based, since that is the number a setter is handed.
func (d *Date) components(utc bool) [componentCount]float64 {
	base := d.ms
	var p timeParts
	if utc {
		p = splitTimeValue(base)
	} else {
		p = d.localParts()
	}
	return [componentCount]float64{
		float64(p.year), float64(p.month - 1), float64(p.day),
		float64(p.hour), float64(p.minute), float64(p.second), float64(p.milli),
	}
}

// setFields replaces the components from start onward with the arguments given and
// rebuilds the time value, which is what every setter does. Fields before start keep the
// value they had and fields after the last argument keep theirs too, so setHours(9) moves
// the hour and leaves the minutes alone.
//
// A setter on the Invalid Date leaves it invalid: there are no components to build on. The
// two year setters are the exception the specification carves out, since a year is enough
// to name a date on its own; they start from the epoch instead.
func (d *Date) setFields(utc bool, start int, recoverInvalid bool, args []float64) float64 {
	if math.IsNaN(d.ms) {
		if !recoverInvalid {
			return d.ms
		}
		d.ms = 0
		// The recovered date is read as UTC even for a local setter, which is what the
		// specification says: the base is the time value +0 itself, not the local reading
		// of it.
		utc = true
	}
	c := d.components(utc)
	for i, v := range args {
		if start+i >= componentCount {
			break
		}
		c[start+i] = v
	}
	d.ms = timeClip(timeValueFromComponents(c, utc))
	return d.ms
}

// SetTime replaces the whole time value, the lowering of date.setTime(ms). It is the one
// setter that takes an instant rather than a calendar field, so it does not go through the
// component rebuild at all.
func (d *Date) SetTime(ms float64) float64 {
	d.ms = timeClip(ms)
	return d.ms
}

// The local setters. Each takes its own field and, optionally, the fields below it, so
// setHours(9, 30) moves the hour and the minute in one call. Each gives back the new time
// value, which is what makes d.setDate(1) usable as an expression.
func (d *Date) SetFullYear(args ...float64) float64 {
	return d.setFields(false, fieldYear, true, args)
}
func (d *Date) SetMonth(args ...float64) float64 {
	return d.setFields(false, fieldMonth, false, args)
}
func (d *Date) SetDate(args ...float64) float64 {
	return d.setFields(false, fieldDay, false, args)
}
func (d *Date) SetHours(args ...float64) float64 {
	return d.setFields(false, fieldHour, false, args)
}
func (d *Date) SetMinutes(args ...float64) float64 {
	return d.setFields(false, fieldMinute, false, args)
}
func (d *Date) SetSeconds(args ...float64) float64 {
	return d.setFields(false, fieldSecond, false, args)
}
func (d *Date) SetMilliseconds(args ...float64) float64 {
	return d.setFields(false, fieldMilli, false, args)
}

// The UTC setters, the same writes against the UTC reading of the date.
func (d *Date) SetUTCFullYear(args ...float64) float64 {
	return d.setFields(true, fieldYear, true, args)
}
func (d *Date) SetUTCMonth(args ...float64) float64 {
	return d.setFields(true, fieldMonth, false, args)
}
func (d *Date) SetUTCDate(args ...float64) float64 {
	return d.setFields(true, fieldDay, false, args)
}
func (d *Date) SetUTCHours(args ...float64) float64 {
	return d.setFields(true, fieldHour, false, args)
}
func (d *Date) SetUTCMinutes(args ...float64) float64 {
	return d.setFields(true, fieldMinute, false, args)
}
func (d *Date) SetUTCSeconds(args ...float64) float64 {
	return d.setFields(true, fieldSecond, false, args)
}
func (d *Date) SetUTCMilliseconds(args ...float64) float64 {
	return d.setFields(true, fieldMilli, false, args)
}
