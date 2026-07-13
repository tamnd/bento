package lower

import (
	"go/ast"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the Temporal area (10_advanced group 6), one type per cut. Eight types are
// hosted so far: PlainDate, a calendar date with no time and no zone over the ISO 8601
// calendar; PlainTime, a wall-clock time with no date and no zone; PlainDateTime, a date
// paired with a wall-clock time; Duration, a span of time as ten signed component counts;
// PlainYearMonth, a calendar year and month with no day; PlainMonthDay, a calendar month and
// day with no year; Instant, an exact point on the UTC time line as a nanosecond count; and
// ZonedDateTime, that same exact time paired with a time zone that gives it a wall-clock
// reading and a calendar. For the plain types, construction, the static from over the same
// type (and, for the ordered types, compare), the clean field getters, and the equals,
// toString, and toJSON methods lower to the matching value runtime type. Duration hosts
// construction, the field getters plus sign and blank, negated and abs, toString and toJSON,
// and from over a Duration. Instant hosts construction and the two epoch factories, the
// epoch-milliseconds and epoch-nanoseconds getters, compare, equals, toString and toJSON, and
// from over an Instant. ZonedDateTime hosts construction from an epoch count and a zone, the
// exact-time and wall-clock getters (the offset and the ISO date and time in the zone),
// compare, equals, toString and toJSON, from over a ZonedDateTime, and the conversions to an
// Instant and to the plain types. Everything else, the arithmetic, the balancing and rounding,
// the reshaping with and withX, the time-zone transition queries, from over a string or a
// property bag, and toLocaleString, hands back with a named reason so the compiler reports the
// exact ceiling. The Temporal.Now namespace reads the clock: instant, timeZoneId, and the four
// ISO functions lower to value.Now* constructors that read the host wall clock, or the fixed
// instant BENTO_NOW_NS pins so the differential harness runs against a clock it can reproduce.
//
// Each Temporal type follows the host-type model RegExp and the collections use: it is a bare
// pointer in the generated Go (*value.PlainDate, *value.PlainTime, *value.PlainDateTime,
// *value.Duration, *value.PlainYearMonth, *value.PlainMonthDay, *value.Instant,
// *value.ZonedDateTime), recognized by its declaring symbol name rather than a dedicated type
// flag.
// The Temporal namespace is a two-level access (Temporal.PlainDate.compare), which no other
// built-in uses, so the call and new paths carry a small amount of namespace-chain recognition
// this file drives.

// plainDateType reports whether a checker type is the Temporal.PlainDate interface.
// Like the RegExp and DataView checks it is a shape test on the declaring symbol: an
// object type, not an array, whose symbol is named PlainDate. That is enough to tell a
// PlainDate receiver from a plain object, whose class the compiler would otherwise
// intern as struct fields.
func (r *Renderer) plainDateType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "PlainDate"
}

// isPlainDate reports whether the node's static type is a Temporal.PlainDate, the
// receiver test the getter, method, and type-slot paths use to route to the PlainDate
// machinery.
func (r *Renderer) isPlainDate(n frontend.Node) bool {
	return r.plainDateType(r.prog.TypeAt(n))
}

// plainTimeType reports whether a checker type is the Temporal.PlainTime interface, the
// same shape test as plainDateType over the symbol name PlainTime.
func (r *Renderer) plainTimeType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "PlainTime"
}

// isPlainTime reports whether the node's static type is a Temporal.PlainTime.
func (r *Renderer) isPlainTime(n frontend.Node) bool {
	return r.plainTimeType(r.prog.TypeAt(n))
}

// plainDateTimeType reports whether a checker type is the Temporal.PlainDateTime interface,
// the same shape test as plainDateType over the symbol name PlainDateTime.
func (r *Renderer) plainDateTimeType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "PlainDateTime"
}

// isPlainDateTime reports whether the node's static type is a Temporal.PlainDateTime.
func (r *Renderer) isPlainDateTime(n frontend.Node) bool {
	return r.plainDateTimeType(r.prog.TypeAt(n))
}

// durationType reports whether a checker type is the Temporal.Duration interface, the
// same shape test as plainDateType over the symbol name Duration.
func (r *Renderer) durationType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "Duration"
}

// isDuration reports whether the node's static type is a Temporal.Duration.
func (r *Renderer) isDuration(n frontend.Node) bool {
	return r.durationType(r.prog.TypeAt(n))
}

// plainYearMonthType reports whether a checker type is the Temporal.PlainYearMonth interface,
// the same shape test as plainDateType over the symbol name PlainYearMonth.
func (r *Renderer) plainYearMonthType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "PlainYearMonth"
}

// isPlainYearMonth reports whether the node's static type is a Temporal.PlainYearMonth.
func (r *Renderer) isPlainYearMonth(n frontend.Node) bool {
	return r.plainYearMonthType(r.prog.TypeAt(n))
}

// plainMonthDayType reports whether a checker type is the Temporal.PlainMonthDay interface,
// the same shape test as plainDateType over the symbol name PlainMonthDay.
func (r *Renderer) plainMonthDayType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "PlainMonthDay"
}

// isPlainMonthDay reports whether the node's static type is a Temporal.PlainMonthDay.
func (r *Renderer) isPlainMonthDay(n frontend.Node) bool {
	return r.plainMonthDayType(r.prog.TypeAt(n))
}

// instantType reports whether a checker type is the Temporal.Instant interface, the same
// shape test as plainDateType over the symbol name Instant.
func (r *Renderer) instantType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "Instant"
}

// isInstant reports whether the node's static type is a Temporal.Instant.
func (r *Renderer) isInstant(n frontend.Node) bool {
	return r.instantType(r.prog.TypeAt(n))
}

// zonedDateTimeType reports whether a checker type is the Temporal.ZonedDateTime interface,
// the same shape test as plainDateType over the symbol name ZonedDateTime.
func (r *Renderer) zonedDateTimeType(t frontend.Type) bool {
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	if _, isArray := r.prog.ElementType(t); isArray {
		return false
	}
	sym, ok := r.prog.TypeSymbol(t)
	return ok && sym.Name == "ZonedDateTime"
}

// isZonedDateTime reports whether the node's static type is a Temporal.ZonedDateTime.
func (r *Renderer) isZonedDateTime(n frontend.Node) bool {
	return r.zonedDateTimeType(r.prog.TypeAt(n))
}

// plainDateAccessor maps a PlainDate field getter to the value.PlainDate method that
// reads it, or reports ok=false for a name this slice does not host. The clean ISO
// getters (year, month, day, and the derived weekday, day-of-year, leap flag, fixed
// counts, month code, and calendar id) map to a method returning the field's plain
// type; the calendar-dependent getters the checker types as an optional (era,
// eraYear, weekOfYear, yearOfWeek) map to a method returning a value.Opt, which the
// member read boxes at any dynamic use, era and eraYear always undefined under the
// ISO calendar and the two week fields always present.
func plainDateAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "year":
		return "Year", true
	case "month":
		return "Month", true
	case "day":
		return "Day", true
	case "calendarId":
		return "CalendarId", true
	case "monthCode":
		return "MonthCode", true
	case "dayOfWeek":
		return "DayOfWeek", true
	case "dayOfYear":
		return "DayOfYear", true
	case "daysInWeek":
		return "DaysInWeek", true
	case "daysInMonth":
		return "DaysInMonth", true
	case "daysInYear":
		return "DaysInYear", true
	case "monthsInYear":
		return "MonthsInYear", true
	case "inLeapYear":
		return "InLeapYear", true
	case "era":
		return "Era", true
	case "eraYear":
		return "EraYear", true
	case "weekOfYear":
		return "WeekOfYear", true
	case "yearOfWeek":
		return "YearOfWeek", true
	}
	return "", false
}

// plainTimeAccessor maps a PlainTime field getter to the value.PlainTime method that
// reads it, or reports ok=false for a name this slice does not host. All six fields are
// clean numbers with no undefined case, so every getter maps to a method.
func plainTimeAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "hour":
		return "Hour", true
	case "minute":
		return "Minute", true
	case "second":
		return "Second", true
	case "millisecond":
		return "Millisecond", true
	case "microsecond":
		return "Microsecond", true
	case "nanosecond":
		return "Nanosecond", true
	}
	return "", false
}

// plainDateTimeAccessor maps a PlainDateTime field getter to the value.PlainDateTime method
// that reads it, or reports ok=false for a name this slice does not host. It is the union of
// the PlainDate getters and the six PlainTime getters, since a date-time carries both, so it
// answers the calendar-dependent getters (era, eraYear, weekOfYear, yearOfWeek) off the date
// half exactly as PlainDate does.
func plainDateTimeAccessor(prop string) (method string, ok bool) {
	if m, ok := plainDateAccessor(prop); ok {
		return m, true
	}
	return plainTimeAccessor(prop)
}

// durationAccessor maps a Duration field getter to the value.Duration method that reads
// it, or reports ok=false for a name this slice does not host. The ten count fields plus
// the derived sign and blank are all clean, so every getter maps to a method. sign reads
// as a number and blank as a boolean, the types the checker gives them.
func durationAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "years":
		return "Years", true
	case "months":
		return "Months", true
	case "weeks":
		return "Weeks", true
	case "days":
		return "Days", true
	case "hours":
		return "Hours", true
	case "minutes":
		return "Minutes", true
	case "seconds":
		return "Seconds", true
	case "milliseconds":
		return "Milliseconds", true
	case "microseconds":
		return "Microseconds", true
	case "nanoseconds":
		return "Nanoseconds", true
	case "sign":
		return "Sign", true
	case "blank":
		return "Blank", true
	}
	return "", false
}

// plainYearMonthAccessor maps a PlainYearMonth field getter to the value.PlainYearMonth method
// that reads it, or reports ok=false for a name this slice does not host. The clean ISO getters
// (year, month, month code, calendar id, and the derived counts and leap flag) map to a method;
// a year-month has no day, and the calendar-dependent getters the checker types number |
// undefined (era, eraYear) are absent so they hand back rather than lower to a getter that
// cannot answer the undefined case.
func plainYearMonthAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "year":
		return "Year", true
	case "month":
		return "Month", true
	case "calendarId":
		return "CalendarId", true
	case "monthCode":
		return "MonthCode", true
	case "daysInMonth":
		return "DaysInMonth", true
	case "daysInYear":
		return "DaysInYear", true
	case "monthsInYear":
		return "MonthsInYear", true
	case "inLeapYear":
		return "InLeapYear", true
	}
	return "", false
}

// plainMonthDayAccessor maps a PlainMonthDay field getter to the value.PlainMonthDay method
// that reads it, or reports ok=false for a name this slice does not host. A month-day exposes
// only its month code, its day, and its calendar id; it has no numeric month or year getter,
// so those names are absent and hand back.
func plainMonthDayAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "monthCode":
		return "MonthCode", true
	case "day":
		return "Day", true
	case "calendarId":
		return "CalendarId", true
	}
	return "", false
}

// instantAccessor maps an Instant field getter to the value.Instant method that reads it,
// or reports ok=false for a name this slice does not host. Both getters are clean: epoch
// milliseconds reads as a number and epoch nanoseconds as a bigint, the types the checker
// gives them, neither with an undefined case, so each maps to a method.
func instantAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "epochMilliseconds":
		return "EpochMilliseconds", true
	case "epochNanoseconds":
		return "EpochNanoseconds", true
	}
	return "", false
}

// instantMethodCall lowers a method call on an Instant receiver, the mirror of
// plainDateMethodCall. equals(other) compares two instants, and toString and toJSON render
// the UTC ISO 8601 string; each takes no options in this slice, so a call with arguments
// beyond the ones handled hands back. The arithmetic and rounding methods (add, subtract,
// until, since, round), which need Duration and options parsing, and the conversions
// (toZonedDateTimeISO, toLocaleString) hand back with a named reason.
func (r *Renderer) instantMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.prototype.equals takes exactly one argument"}
		}
		if !r.isInstant(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.prototype.equals over a non-Instant argument (a string to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Instant.prototype." + method + " is a later slice"}
	}
}

// instantStaticCall lowers a static call on Temporal.Instant. compare lowers to
// value.InstantCompare; fromEpochMilliseconds and fromEpochNanoseconds lower to the
// matching value factory over a number or a bigint; from lowers to value.InstantFrom for
// an Instant argument (the copy the specification makes) and to value.InstantFromString for a
// string literal. An Instant ignores any calendar the string names, so the literal needs no
// calendar gate, like PlainTime; only a dynamic string or a non-string argument hands back.
func (r *Renderer) instantStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.compare takes exactly two arguments"}
		}
		if !r.isInstant(argNodes[0]) || !r.isInstant(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.compare over an argument that is not an Instant (a string to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "InstantCompare"), Args: []ast.Expr{a, b}}, nil
	case "fromEpochMilliseconds":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.fromEpochMilliseconds takes exactly one argument"}
		}
		if !r.isNumber(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.fromEpochMilliseconds over a non-number argument is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "InstantFromEpochMilliseconds"), Args: []ast.Expr{arg}}, nil
	case "fromEpochNanoseconds":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.fromEpochNanoseconds takes exactly one argument"}
		}
		if !r.isBigInt(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.fromEpochNanoseconds over a non-bigint argument is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "InstantFromEpochNanoseconds"), Args: []ast.Expr{arg}}, nil
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.from takes exactly one argument"}
		}
		if r.isInstant(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "InstantFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "InstantFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if r.isString(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "InstantFromString"), Args: []ast.Expr{goStringOf(arg)}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.Instant.from over a property bag or a value not statically typed as a string is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Instant." + method + " is a later slice"}
	}
}

// newInstant lowers new Temporal.Instant over its single bigint argument, the nanoseconds
// since the epoch. The argument must lower as a bigint, so a non-bigint component hands
// back rather than coerce; the runtime runs IsValidEpochNanoseconds, so an out-of-range
// count throws a RangeError at run time the way the specification requires.
func (r *Renderer) newInstant(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: "new Temporal.Instant takes exactly one argument"}
	}
	if !r.isBigInt(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "new Temporal.Instant with a non-bigint argument is a later slice"}
	}
	arg, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewInstant"), Args: []ast.Expr{arg}}, nil
}

// zonedDateTimeAccessor maps a ZonedDateTime field getter to the value.ZonedDateTime method
// that reads it, or reports ok=false for a name this slice does not host. The exact-time
// getters (epochMilliseconds a number, epochNanoseconds a bigint) and the zone getters
// (timeZoneId, calendarId, offset strings, offsetNanoseconds a number) map straight to a
// method, and the wall-clock getters are the union of the PlainDate and PlainTime getters,
// answered off the local reading in the zone, so the calendar-dependent ones (era, eraYear,
// weekOfYear, yearOfWeek) map to a method returning a value.Opt the member read boxes. The
// day-length getter hoursInDay needs the day-boundary offset math this slice does not carry,
// so it is absent and hands back.
func zonedDateTimeAccessor(prop string) (method string, ok bool) {
	switch prop {
	case "epochMilliseconds":
		return "EpochMilliseconds", true
	case "epochNanoseconds":
		return "EpochNanoseconds", true
	case "timeZoneId":
		return "TimeZoneId", true
	case "offset":
		return "Offset", true
	case "offsetNanoseconds":
		return "OffsetNanoseconds", true
	}
	return plainDateTimeAccessor(prop)
}

// zonedDateTimeMethodCall lowers a method call on a ZonedDateTime receiver, the mirror of
// plainDateTimeMethodCall. equals(other) compares two zoned date-times, toString and toJSON
// render the round-trippable string, and the conversions toInstant, toPlainDate, toPlainTime,
// and toPlainDateTime drop the zone or narrow to a plain type; none takes options in this
// slice, so a call with arguments beyond the ones handled hands back. The arithmetic and
// rounding methods, the reshaping (with, withPlainTime, withTimeZone, withCalendar,
// startOfDay), the transition queries, and toLocaleString hand back with a named reason.
func (r *Renderer) zonedDateTimeMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.equals takes exactly one argument"}
		}
		if !r.isZonedDateTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.equals over a non-ZonedDateTime argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON", "toInstant", "toPlainDate", "toPlainTime", "toPlainDateTime":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		names := map[string]string{
			"toString":        "ToString",
			"toJSON":          "ToJSON",
			"toInstant":       "ToInstant",
			"toPlainDate":     "ToPlainDate",
			"toPlainTime":     "ToPlainTime",
			"toPlainDateTime": "ToPlainDateTime",
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(names[method])}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype." + method + " is a later slice"}
	}
}

// zonedDateTimeStaticCall lowers a static call on Temporal.ZonedDateTime. compare lowers to
// value.ZonedDateTimeCompare over two ZonedDateTime arguments, ordering on the exact time;
// from lowers to value.ZonedDateTimeFrom for a ZonedDateTime argument (the copy the
// specification makes) and to value.ZonedDateTimeFromString for a string literal. A
// ZonedDateTime hosts only the iso8601 calendar, so the literal is gated on
// literalISOCalendarOnly, the same stricter gate the two other ISO-only types use: a literal
// naming a non-ISO calendar hands back, as does a dynamic string or a property bag.
func (r *Renderer) zonedDateTimeStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.compare takes exactly two arguments"}
		}
		if !r.isZonedDateTime(argNodes[0]) || !r.isZonedDateTime(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.compare over an argument that is not a ZonedDateTime (a string or bag to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeCompare"), Args: []ast.Expr{a, b}}, nil
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from takes exactly one argument"}
		}
		if r.isZonedDateTime(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if !literalISOCalendarOnly(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a string naming a non-ISO calendar is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a dynamic string or a property bag is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime." + method + " is a later slice"}
	}
}

// nowCall lowers a Temporal.Now function. Now reads the clock, so each function lowers to a
// value.Now* constructor that reads the host wall clock, or, when BENTO_NOW_NS is set, the fixed
// instant the differential harness pins. instant and timeZoneId take no argument. The four ISO
// functions take an optional time-zone identifier: with none the host default zone is used, and
// with a string argument that names a zone; a non-string zone argument, a TimeZoneLike object
// this slice does not carry, hands back rather than coerce.
func (r *Renderer) nowCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	// noArg wraps a Now function that takes no argument.
	noArg := func(fn string) (ast.Expr, error) {
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.Now." + method + " takes no argument"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", fn)}, nil
	}
	// zoned wraps a Now function that takes an optional time-zone identifier, routing to the
	// default-zone constructor with no argument and the named-zone constructor with a string.
	zoned := func(defaultFn, inFn string) (ast.Expr, error) {
		switch len(argNodes) {
		case 0:
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", defaultFn)}, nil
		case 1:
			if !r.isString(argNodes[0]) {
				return nil, &NotYetLowerable{Reason: "Temporal.Now." + method + " over a non-string time-zone argument is a later slice"}
			}
			tz, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", inFn), Args: []ast.Expr{tz}}, nil
		default:
			return nil, &NotYetLowerable{Reason: "Temporal.Now." + method + " takes at most one argument"}
		}
	}
	switch method {
	case "instant":
		return noArg("NowInstant")
	case "timeZoneId":
		return noArg("NowTimeZoneId")
	case "zonedDateTimeISO":
		return zoned("NowZonedDateTimeISO", "NowZonedDateTimeISOIn")
	case "plainDateTimeISO":
		return zoned("NowPlainDateTimeISO", "NowPlainDateTimeISOIn")
	case "plainDateISO":
		return zoned("NowPlainDateISO", "NowPlainDateISOIn")
	case "plainTimeISO":
		return zoned("NowPlainTimeISO", "NowPlainTimeISOIn")
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Now." + method + " is a later slice"}
	}
}

// newZonedDateTime lowers new Temporal.ZonedDateTime over its epoch-nanosecond bigint and its
// time-zone identifier string. A third calendar argument selects a non-ISO calendar, which
// this slice does not carry, so it hands back. The first argument must lower as a bigint and
// the second as a string, so a non-conforming component hands back rather than coerce; the
// runtime constructor runs IsValidEpochNanoseconds and resolves the zone, so an out-of-range
// count or an unknown zone throws a RangeError at run time the way the specification requires.
func (r *Renderer) newZonedDateTime(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "new Temporal.ZonedDateTime with a calendar argument or fewer than two components is a later slice"}
	}
	if !r.isBigInt(argNodes[0]) {
		return nil, &NotYetLowerable{Reason: "new Temporal.ZonedDateTime with a non-bigint epoch argument is a later slice"}
	}
	if !r.isString(argNodes[1]) {
		return nil, &NotYetLowerable{Reason: "new Temporal.ZonedDateTime with a non-string time-zone argument is a later slice"}
	}
	ns, err := r.lowerExpr(argNodes[0])
	if err != nil {
		return nil, err
	}
	tz, err := r.lowerExpr(argNodes[1])
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewZonedDateTime"), Args: []ast.Expr{ns, tz}}, nil
}

// durationMethodCall lowers a method call on a Duration receiver. negated and abs return a
// reshaped Duration, and toString and toJSON render the ISO 8601 duration string; each takes
// no argument in this slice. The methods that balance or round across units (round, total,
// add, subtract, with) need a relativeTo reference and the calendar model, so they hand back
// with a named reason.
func (r *Renderer) durationMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "negated", "abs":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " takes no argument"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "Negated"
		if method == "abs" {
			name = "Abs"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " is a later slice"}
	}
}

// durationStaticCall lowers a static call on Temporal.Duration. from over a Duration lowers
// to value.DurationFrom (the copy the specification makes) and from over a string literal to
// value.DurationFromString. A Duration carries no calendar, so the literal needs no gate; a
// dynamic string or a property bag hands back, and compare, which needs a relativeTo
// reference to balance the calendar units, hands back too.
func (r *Renderer) durationStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.from takes exactly one argument"}
		}
		if r.isDuration(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "DurationFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "DurationFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if r.isString(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "DurationFromString"), Args: []ast.Expr{goStringOf(arg)}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.Duration.from over a property bag or a value not statically typed as a string is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Duration." + method + " is a later slice"}
	}
}

// plainDateMethodCall lowers a method call on a PlainDate receiver. equals(other)
// compares two dates, and toString and toJSON render the ISO 8601 string; each takes
// no options in this slice, so a call with arguments beyond the ones handled hands
// back. The arithmetic and conversion methods, which need Duration, the other Temporal
// types, or options parsing, hand back with a named reason.
func (r *Renderer) plainDateMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.equals takes exactly one argument"}
		}
		if !r.isPlainDate(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.equals over a non-PlainDate argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	case "withCalendar":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.withCalendar takes exactly one argument"}
		}
		cal, ok := r.hostedCalendar(argNodes[0])
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.withCalendar over a calendar other than a literal iso8601 or gregory is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateWithCalendar"), Args: []ast.Expr{recv, calendarLit(cal)}}, nil
	case "add", "subtract":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainDate.prototype." + method
		dur, err := r.durationArg(what, argNodes[0])
		if err != nil {
			return nil, err
		}
		overflow, err := r.temporalOverflowOption(what, argNodes[1:])
		if err != nil {
			return nil, err
		}
		if method == "subtract" {
			dur = &ast.CallExpr{Fun: &ast.SelectorExpr{X: dur, Sel: ident("Negated")}}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AddDate")}, Args: []ast.Expr{dur, stringLit(overflow)}}, nil
	case "until", "since":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainDate.prototype." + method
		if !r.isPlainDate(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainDate is a later slice"}
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		largestUnit, err := r.plainDateDifferenceOptions(what, argNodes[1:])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		fn := "Until"
		if method == "since" {
			fn = "Since"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(fn)}, Args: []ast.Expr{other, stringLit(largestUnit)}}, nil
	case "with":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.with takes at least one argument"}
		}
		what := "Temporal.PlainDate.prototype.with"
		fields, err := r.plainDateBagFields(what, argNodes[0])
		if err != nil {
			return nil, err
		}
		overflow, err := r.temporalOverflowOption(what, argNodes[1:])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithFields")}, Args: append(fields[:], stringLit(overflow))}, nil
	case "toPlainDateTime":
		what := "Temporal.PlainDate.prototype.toPlainDateTime"
		var timeArg ast.Expr = ident("nil")
		if len(argNodes) > 0 {
			if !r.isPlainTime(argNodes[0]) {
				return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainTime is a later slice"}
			}
			t, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			timeArg = t
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToPlainDateTime")}, Args: []ast.Expr{timeArg}}, nil
	case "toPlainYearMonth":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.toPlainYearMonth takes no argument"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToPlainYearMonth")}}, nil
	case "toPlainMonthDay":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype.toPlainMonthDay takes no argument"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToPlainMonthDay")}}, nil
	case "toZonedDateTime":
		what := "Temporal.PlainDate.prototype.toZonedDateTime"
		tz, timeArg, err := r.plainDateToZonedArgs(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToZonedDateTime")}, Args: []ast.Expr{tz, timeArg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype." + method + " is a later slice"}
	}
}

// dateFieldKeys are the numeric calendar-date fields PlainDate.prototype.with reshapes,
// in the order WithFields takes them. monthCode and the era fields are recognized keys
// too, but they resolve a month or year through the calendar, which a later slice carries,
// so the bag reader hands back on them rather than listing them here.
var dateFieldKeys = [3]string{"year", "month", "day"}

// plainDateBagFields reads a PlainDate.prototype.with bag at compile time and returns the
// year, month, and day as present or absent optionals in WithFields order. A present field
// must be a number; an absent one becomes None so WithFields keeps the receiver's value. An
// item that is not an object literal, a spread or shorthand member, a repeated field, a
// non-number value, an empty bag (a TypeError at run time), or a key outside the numeric
// three, including monthCode and the era fields, hands back rather than emitting a wrong or
// partial reshape.
func (r *Renderer) plainDateBagFields(what string, n frontend.Node) ([3]ast.Expr, error) {
	var fields [3]ast.Expr
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return fields, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var seen [3]bool
	present := false
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return fields, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return fields, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		idx := -1
		for i, k := range dateFieldKeys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fields, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if seen[idx] {
			return fields, &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
		}
		if !r.isNumber(kids[1]) {
			return fields, &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
		}
		val, err := r.lowerExpr(kids[1])
		if err != nil {
			return fields, err
		}
		fields[idx] = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("float64")), Args: []ast.Expr{val}}
		seen[idx] = true
		present = true
	}
	if !present {
		return fields, &NotYetLowerable{Reason: what + " over an empty bag (a TypeError at run time) is a later slice"}
	}
	for i := range fields {
		if fields[i] == nil {
			fields[i] = &ast.CallExpr{Fun: index(sel("value", "None"), ident("float64"))}
		}
	}
	r.requireImport(valuePkg)
	return fields, nil
}

// plainDateDifferenceUnits maps the calendar difference units, singular and plural, to the
// singular form the runtime takes. "auto" resolves to "day", the default largestUnit.
var plainDateDifferenceUnits = map[string]string{
	"auto": "day", "year": "year", "years": "year", "month": "month", "months": "month",
	"week": "week", "weeks": "week", "day": "day", "days": "day",
}

// plainDateDifferenceOptions reads the options of PlainDate.prototype.until and since at
// compile time and returns the largestUnit, defaulting to day. It accepts largestUnit as a
// string literal in the calendar-unit set. A smallestUnit, roundingIncrement, or roundingMode
// would round the calendar duration, which needs the round-with-relativeTo machinery a later
// slice carries, so any of those, a non-literal or out-of-set largestUnit, an unknown key, or
// a spread or shorthand member hands back rather than emitting a wrong or partial result.
func (r *Renderer) plainDateDifferenceOptions(what string, argNodes []frontend.Node) (string, error) {
	largestUnit := "day"
	if len(argNodes) == 0 {
		return largestUnit, nil
	}
	if len(argNodes) != 1 {
		return "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := argNodes[0]
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", &NotYetLowerable{Reason: what + " options that are not an object literal are a later slice"}
	}
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "largestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", &NotYetLowerable{Reason: what + " with a non-literal largestUnit is a later slice"}
			}
			unit, ok := plainDateDifferenceUnits[lit]
			if !ok {
				return "", &NotYetLowerable{Reason: what + " with the invalid largestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			largestUnit = unit
		case "smallestUnit", "roundingIncrement", "roundingMode":
			return "", &NotYetLowerable{Reason: what + " with the rounding option " + key + " is a later slice"}
		default:
			return "", &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	return largestUnit, nil
}

// plainTimeMethodCall lowers a method call on a PlainTime receiver, the mirror of
// plainDateMethodCall. equals(other) compares two times, and toString and toJSON render
// the ISO 8601 time string; each takes no options in this slice, so a call with arguments
// beyond the ones handled hands back. The arithmetic, rounding, reshaping, and conversion
// methods, which need Duration, options parsing, or the other Temporal types, hand back
// with a named reason.
func (r *Renderer) plainTimeMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype.equals takes exactly one argument"}
		}
		if !r.isPlainTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype.equals over a non-PlainTime argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	case "with":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype.with takes at least one argument"}
		}
		fields, err := r.plainTimeBagFields("Temporal.PlainTime.prototype.with", argNodes[0])
		if err != nil {
			return nil, err
		}
		overflow, err := r.temporalOverflowOption("Temporal.PlainTime.prototype.with", argNodes[1:])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("With")}, Args: append(fields[:], stringLit(overflow))}, nil
	case "add", "subtract":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainTime.prototype." + method
		dur, err := r.durationArg(what, argNodes[0])
		if err != nil {
			return nil, err
		}
		if _, err := r.temporalOverflowOption(what, argNodes[1:]); err != nil {
			return nil, err
		}
		if method == "subtract" {
			dur = &ast.CallExpr{Fun: &ast.SelectorExpr{X: dur, Sel: ident("Negated")}}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AddDuration")}, Args: []ast.Expr{dur}}, nil
	case "round":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype.round takes at least one argument"}
		}
		unit, increment, mode, err := r.plainTimeRoundOptions("Temporal.PlainTime.prototype.round", argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Round")}, Args: []ast.Expr{stringLit(unit), increment, stringLit(mode)}}, nil
	case "until", "since":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainTime.prototype." + method
		if !r.isPlainTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainTime is a later slice"}
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		largestUnit, smallestUnit, increment, mode, err := r.plainTimeDifferenceOptions(what, argNodes[1:])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		fn := "Until"
		if method == "since" {
			fn = "Since"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(fn)}, Args: []ast.Expr{other, stringLit(largestUnit), stringLit(smallestUnit), increment, stringLit(mode)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.prototype." + method + " is a later slice"}
	}
}

// plainDateTimeMethodCall lowers a method call on a PlainDateTime receiver, the mirror of
// plainDateMethodCall and plainTimeMethodCall. equals(other) compares two date-times, and
// toString and toJSON render the ISO 8601 string; each takes no options in this slice, so a
// call with arguments beyond the ones handled hands back. The arithmetic, rounding,
// reshaping, and conversion methods, which need Duration, options parsing, or the other
// Temporal types, hand back with a named reason.
func (r *Renderer) plainDateTimeMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype.equals takes exactly one argument"}
		}
		if !r.isPlainDateTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype.equals over a non-PlainDateTime argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	case "withCalendar":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype.withCalendar takes exactly one argument"}
		}
		cal, ok := r.hostedCalendar(argNodes[0])
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype.withCalendar over a calendar other than a literal iso8601 or gregory is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateTimeWithCalendar"), Args: []ast.Expr{recv, calendarLit(cal)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " is a later slice"}
	}
}

// plainYearMonthMethodCall lowers a method call on a PlainYearMonth receiver, the mirror of
// plainDateMethodCall. equals(other) compares two year-months, and toString and toJSON render
// the ISO 8601 string; each takes no options in this slice, so a call with arguments beyond the
// ones handled hands back. The arithmetic, reshaping, and conversion methods (add, subtract,
// until, since, with, toPlainDate), which need Duration, options parsing, or the calendar model,
// hand back with a named reason.
func (r *Renderer) plainYearMonthMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype.equals takes exactly one argument"}
		}
		if !r.isPlainYearMonth(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype.equals over a non-PlainYearMonth argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype." + method + " is a later slice"}
	}
}

// plainMonthDayMethodCall lowers a method call on a PlainMonthDay receiver, the mirror of
// plainDateMethodCall. equals(other) compares two month-days, and toString and toJSON render
// the ISO 8601 string; each takes no options in this slice, so a call with arguments beyond the
// ones handled hands back. The reshaping and conversion methods (with, toPlainDate), which need
// options parsing or a year the month-day does not carry, hand back with a named reason.
func (r *Renderer) plainMonthDayMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "equals":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.prototype.equals takes exactly one argument"}
		}
		if !r.isPlainMonthDay(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.prototype.equals over a non-PlainMonthDay argument (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Equals")}, Args: []ast.Expr{other}}, nil
	case "toString", "toJSON":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.prototype." + method + " with options is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToString"
		if method == "toJSON" {
			name = "ToJSON"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.prototype." + method + " is a later slice"}
	}
}

// temporalStaticCall lowers a static call on a Temporal namespace member, routing on the
// type name to the per-type static handler. A Temporal type this file does not host yet
// hands back with a named reason.
func (r *Renderer) temporalStaticCall(typeName, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch typeName {
	case "PlainDate":
		return r.plainDateStaticCall(method, argNodes)
	case "PlainTime":
		return r.plainTimeStaticCall(method, argNodes)
	case "PlainDateTime":
		return r.plainDateTimeStaticCall(method, argNodes)
	case "Duration":
		return r.durationStaticCall(method, argNodes)
	case "PlainYearMonth":
		return r.plainYearMonthStaticCall(method, argNodes)
	case "PlainMonthDay":
		return r.plainMonthDayStaticCall(method, argNodes)
	case "Instant":
		return r.instantStaticCall(method, argNodes)
	case "ZonedDateTime":
		return r.zonedDateTimeStaticCall(method, argNodes)
	case "Now":
		return r.nowCall(method, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "Temporal." + typeName + " is a later slice"}
	}
}

// plainYearMonthStaticCall lowers Temporal.PlainYearMonth.compare(a, b) or
// Temporal.PlainYearMonth.from(x), the mirror of the PlainDate statics. compare lowers to
// value.PlainYearMonthCompare; from lowers to value.PlainYearMonthFrom for a PlainYearMonth
// argument and to value.PlainYearMonthFromString for a string literal. A PlainYearMonth is
// ISO-only, so the literal is gated on literalYearMonthISOOnly, stricter than the PlainDate
// gate: a literal naming any non-ISO calendar hands back, as does a dynamic string or a bag.
func (r *Renderer) plainYearMonthStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.compare takes exactly two arguments"}
		}
		if !r.isPlainYearMonth(argNodes[0]) || !r.isPlainYearMonth(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.compare over an argument that is not a PlainYearMonth (a string or bag to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainYearMonthCompare"), Args: []ast.Expr{a, b}}, nil
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.from with options is a later slice"}
		}
		if r.isPlainYearMonth(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainYearMonthFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if !literalISOCalendarOnly(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.from over a string naming a non-ISO calendar is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainYearMonthFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.from over a dynamic string or a property bag is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth." + method + " is a later slice"}
	}
}

// plainMonthDayStaticCall lowers Temporal.PlainMonthDay.from(x). A month-day has no compare
// static (month-days are not ordered), so only from is handled: it lowers to
// value.PlainMonthDayFrom for a PlainMonthDay argument and value.PlainMonthDayFromString for a
// string literal. A PlainMonthDay is ISO-only, so the literal is gated on
// literalISOCalendarOnly, the same stricter gate PlainYearMonth uses: a literal naming any
// non-ISO calendar hands back, as does a dynamic string or a property bag.
func (r *Renderer) plainMonthDayStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.from with options is a later slice"}
		}
		if r.isPlainMonthDay(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainMonthDayFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if !literalISOCalendarOnly(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.from over a string naming a non-ISO calendar is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainMonthDayFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.from over a dynamic string or a property bag is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay." + method + " is a later slice"}
	}
}

// plainDateStaticCall lowers Temporal.PlainDate.compare(a, b) or Temporal.PlainDate.from(x).
// compare lowers to value.PlainDateCompare over the two dates; from lowers to
// value.PlainDateFrom for a PlainDate argument (the copy the specification makes) and hands
// back for a string or a property bag, which need parsing this slice does not carry.
func (r *Renderer) plainDateStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.compare takes exactly two arguments"}
		}
		if !r.isPlainDate(argNodes[0]) || !r.isPlainDate(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.compare over an argument that is not a PlainDate (a string or bag to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateCompare"), Args: []ast.Expr{a, b}}, nil
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.from with options is a later slice"}
		}
		if r.isPlainDate(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if !literalCalendarHosted(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.from over a string naming a calendar bento does not host is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.from over a dynamic string or a property bag is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate." + method + " is a later slice"}
	}
}

// timeFieldKeys is the ordered set of PlainTime component names a property bag may
// carry, the keys Temporal.PlainTime.from and PlainTime.prototype.with read. The order
// matches the runtime factory's parameter order.
var timeFieldKeys = [6]string{"hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}

// stringLit renders a Go string literal expression, the overflow option the Temporal
// from and with lowerings pass to their runtime factory.
func stringLit(s string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(s)}
}

// plainTimeBagFields reads a PlainTime property bag at compile time into six present-or-
// absent field expressions. The argument must be an object literal whose members are
// plain properties keyed by a time-field name with a number-typed value; a spread, a
// computed or shorthand key, an unknown key (calendar and timeZone among them), a
// non-number value, or a repeated field hands back, since the field would then depend on
// runtime data or a coercion this slice does not carry. At least one field must be
// present, the record Temporal requires. Each present field lowers to
// value.Some[float64](v) and each absent one to value.None[float64]().
func (r *Renderer) plainTimeBagFields(what string, n frontend.Node) ([6]ast.Expr, error) {
	var fields [6]ast.Expr
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return fields, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var seen [6]bool
	present := false
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return fields, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return fields, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		idx := -1
		for i, k := range timeFieldKeys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fields, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if seen[idx] {
			return fields, &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
		}
		if !r.isNumber(kids[1]) {
			return fields, &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
		}
		val, err := r.lowerExpr(kids[1])
		if err != nil {
			return fields, err
		}
		fields[idx] = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("float64")), Args: []ast.Expr{val}}
		seen[idx] = true
		present = true
	}
	if !present {
		return fields, &NotYetLowerable{Reason: what + " over an empty bag (a TypeError at run time) is a later slice"}
	}
	for i := range fields {
		if fields[i] == nil {
			fields[i] = &ast.CallExpr{Fun: index(sel("value", "None"), ident("float64"))}
		}
	}
	r.requireImport(valuePkg)
	return fields, nil
}

// temporalOverflowOption reads the overflow option from a Temporal from or with options
// argument at compile time. With no options argument the default is constrain. An
// options argument must be an object literal carrying only an overflow property whose
// value is the string literal "constrain" or "reject"; any other shape hands back, since
// the mode would then depend on runtime data. The returned string is passed straight to
// the runtime factory.
func (r *Renderer) temporalOverflowOption(what string, argNodes []frontend.Node) (string, error) {
	if len(argNodes) == 0 {
		return "constrain", nil
	}
	if len(argNodes) != 1 {
		return "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := argNodes[0]
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", &NotYetLowerable{Reason: what + " options that are not an object literal are a later slice"}
	}
	overflow := "constrain"
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		if key := r.prog.Text(kids[0]); key != "overflow" {
			return "", &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
		lit, ok := r.stringLiteralValue(kids[1])
		if !ok {
			return "", &NotYetLowerable{Reason: what + " with a non-literal overflow option is a later slice"}
		}
		if lit != "constrain" && lit != "reject" {
			return "", &NotYetLowerable{Reason: what + " with an invalid overflow option (a RangeError at run time) is a later slice"}
		}
		overflow = lit
	}
	return overflow, nil
}

// durationUnitKeys is the ordered set of Duration component names a duration-like bag may
// carry, from the largest unit to the smallest. The order matches value.NewDuration's
// parameter order.
var durationUnitKeys = [10]string{"years", "months", "weeks", "days", "hours", "minutes", "seconds", "milliseconds", "microseconds", "nanoseconds"}

// durationArg lowers a Temporal duration argument, the value the PlainTime arithmetic
// methods take. It is a Temporal.Duration expression, a duration-like bag of numbers, or
// an ISO 8601 duration string literal; anything else hands back, since the duration would
// then depend on runtime data or a coercion this slice does not carry.
func (r *Renderer) durationArg(what string, n frontend.Node) (ast.Expr, error) {
	if r.isDuration(n) {
		return r.lowerExpr(n)
	}
	if lit, ok := r.stringLiteralValue(n); ok {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "DurationFromString"), Args: []ast.Expr{stringLit(lit)}}, nil
	}
	if n.Kind() == frontend.NodeObjectLiteralExpression {
		return r.durationBag(what, n)
	}
	return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Duration, a duration-like bag of numbers, or a string literal is a later slice"}
}

// durationBag reads a duration-like property bag at compile time into a value.NewDuration
// call over its ten unit fields. Each member must be a plain property keyed by a duration
// unit name with a number-typed value; a spread, a computed or shorthand key, an unknown
// key, a non-number value, or a repeated field hands back, since the field would then
// depend on runtime data or a coercion this slice does not carry. At least one field must
// be present, the record Temporal requires. Each present field lowers to its value and
// each absent one to a float64 zero, so the fold sees the whole ten-unit record.
func (r *Renderer) durationBag(what string, n frontend.Node) (ast.Expr, error) {
	var fields [10]ast.Expr
	var seen [10]bool
	present := false
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		idx := -1
		for i, k := range durationUnitKeys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if seen[idx] {
			return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
		}
		if !r.isNumber(kids[1]) {
			return nil, &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
		}
		val, err := r.lowerExpr(kids[1])
		if err != nil {
			return nil, err
		}
		fields[idx] = val
		seen[idx] = true
		present = true
	}
	if !present {
		return nil, &NotYetLowerable{Reason: what + " over an empty bag (a TypeError at run time) is a later slice"}
	}
	for i := range fields {
		if fields[i] == nil {
			fields[i] = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
		}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewDuration"), Args: fields[:]}, nil
}

// plainTimeRoundUnits is the set of smallest units Temporal.PlainTime.prototype.round
// accepts, from the largest to the smallest.
var plainTimeRoundUnits = [6]string{"hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}

// plainTimeRoundModes is the set of rounding modes round accepts. The default when the
// roundingMode option is absent is halfExpand.
var plainTimeRoundModes = [9]string{"ceil", "floor", "expand", "trunc", "halfCeil", "halfFloor", "halfExpand", "halfTrunc", "halfEven"}

// plainTimeRoundOptions reads the options of Temporal.PlainTime.prototype.round at compile
// time. The argument is either a smallestUnit string literal shorthand (t.round("hour")) or
// an object literal carrying a required smallestUnit string literal, an optional
// roundingIncrement number expression (default one), and an optional roundingMode string
// literal (default halfExpand). A missing smallestUnit, an unknown key, a non-literal unit
// or mode, or an out-of-set unit or mode hands back, since the value would then depend on
// runtime data or a coercion this slice does not carry. The increment is emitted as an
// expression so the runtime validates its range against the unit.
func (r *Renderer) plainTimeRoundOptions(what string, argNodes []frontend.Node) (string, ast.Expr, string, error) {
	if len(argNodes) != 1 {
		return "", nil, "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := argNodes[0]
	if lit, ok := r.stringLiteralValue(n); ok {
		if !slices.Contains(plainTimeRoundUnits[:], lit) {
			return "", nil, "", &NotYetLowerable{Reason: what + " with the invalid smallestUnit " + lit + " (a RangeError at run time) is a later slice"}
		}
		return lit, &ast.BasicLit{Kind: token.FLOAT, Value: "1"}, "halfExpand", nil
	}
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", nil, "", &NotYetLowerable{Reason: what + " options that are neither a string nor an object literal are a later slice"}
	}
	unit := ""
	var increment ast.Expr = &ast.BasicLit{Kind: token.FLOAT, Value: "1"}
	mode := "halfExpand"
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", nil, "", &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", nil, "", &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "smallestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", nil, "", &NotYetLowerable{Reason: what + " with a non-literal smallestUnit is a later slice"}
			}
			if !slices.Contains(plainTimeRoundUnits[:], lit) {
				return "", nil, "", &NotYetLowerable{Reason: what + " with the invalid smallestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			unit = lit
		case "roundingIncrement":
			if !r.isNumber(kids[1]) {
				return "", nil, "", &NotYetLowerable{Reason: what + " with a non-number roundingIncrement is a later slice"}
			}
			val, err := r.lowerExpr(kids[1])
			if err != nil {
				return "", nil, "", err
			}
			increment = val
		case "roundingMode":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", nil, "", &NotYetLowerable{Reason: what + " with a non-literal roundingMode is a later slice"}
			}
			if !slices.Contains(plainTimeRoundModes[:], lit) {
				return "", nil, "", &NotYetLowerable{Reason: what + " with the invalid roundingMode " + lit + " (a RangeError at run time) is a later slice"}
			}
			mode = lit
		default:
			return "", nil, "", &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	if unit == "" {
		return "", nil, "", &NotYetLowerable{Reason: what + " without a smallestUnit (a RangeError at run time) is a later slice"}
	}
	return unit, increment, mode, nil
}

// plainTimeDifferenceOptions reads the options of until and since at compile time. The
// argument is absent or an object literal carrying an optional largestUnit and smallestUnit
// string literal, an optional roundingIncrement number expression, and an optional
// roundingMode string literal. The defaults are the specification's: largestUnit hour,
// smallestUnit nanosecond, increment one, and mode trunc. largestUnit also accepts auto,
// which resolves to hour for a wall clock. A dynamic unit or mode, an out-of-set unit or
// mode, or an unknown key hands back; the runtime rejects a largestUnit smaller than the
// smallestUnit and an out-of-range increment, so those ride through as a RangeError.
func (r *Renderer) plainTimeDifferenceOptions(what string, argNodes []frontend.Node) (largestUnit, smallestUnit string, increment ast.Expr, mode string, err error) {
	largestUnit, smallestUnit, mode = "hour", "nanosecond", "trunc"
	increment = &ast.BasicLit{Kind: token.FLOAT, Value: "1"}
	if len(argNodes) == 0 {
		return largestUnit, smallestUnit, increment, mode, nil
	}
	if len(argNodes) != 1 {
		return "", "", nil, "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := argNodes[0]
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", "", nil, "", &NotYetLowerable{Reason: what + " options that are not an object literal are a later slice"}
	}
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", "", nil, "", &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", "", nil, "", &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "largestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with a non-literal largestUnit is a later slice"}
			}
			if lit == "auto" {
				lit = "hour"
			}
			if !slices.Contains(plainTimeRoundUnits[:], lit) {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with the invalid largestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			largestUnit = lit
		case "smallestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with a non-literal smallestUnit is a later slice"}
			}
			if !slices.Contains(plainTimeRoundUnits[:], lit) {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with the invalid smallestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			smallestUnit = lit
		case "roundingIncrement":
			if !r.isNumber(kids[1]) {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with a non-number roundingIncrement is a later slice"}
			}
			val, lowerErr := r.lowerExpr(kids[1])
			if lowerErr != nil {
				return "", "", nil, "", lowerErr
			}
			increment = val
		case "roundingMode":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with a non-literal roundingMode is a later slice"}
			}
			if !slices.Contains(plainTimeRoundModes[:], lit) {
				return "", "", nil, "", &NotYetLowerable{Reason: what + " with the invalid roundingMode " + lit + " (a RangeError at run time) is a later slice"}
			}
			mode = lit
		default:
			return "", "", nil, "", &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	return largestUnit, smallestUnit, increment, mode, nil
}

// plainTimeStaticCall lowers Temporal.PlainTime.compare(a, b) or Temporal.PlainTime.from(x),
// the mirror of the PlainDate statics. compare lowers to value.PlainTimeCompare; from lowers
// to value.PlainTimeFrom for a PlainTime argument, value.PlainTimeFromString for a string,
// and value.PlainTimeFromFields for a property bag with an optional overflow option. A
// PlainTime carries no calendar, so unlike the PlainDate from there is no calendar gate. A
// bag that is not an object literal, or one whose fields are not statically numbers, hands
// back for a later slice.
func (r *Renderer) plainTimeStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.compare takes exactly two arguments"}
		}
		if !r.isPlainTime(argNodes[0]) || !r.isPlainTime(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.compare over an argument that is not a PlainTime (a string or bag to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeCompare"), Args: []ast.Expr{a, b}}, nil
	case "from":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from takes at least one argument"}
		}
		if r.isPlainTime(argNodes[0]) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from over a PlainTime with an options argument is a later slice"}
			}
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainTimeFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from over a string with an options argument is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainTimeFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if r.isString(argNodes[0]) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from over a string with an options argument is a later slice"}
			}
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainTimeFromString"), Args: []ast.Expr{goStringOf(arg)}}, nil
		}
		fields, err := r.plainTimeBagFields("Temporal.PlainTime.from", argNodes[0])
		if err != nil {
			return nil, err
		}
		overflow, err := r.temporalOverflowOption("Temporal.PlainTime.from", argNodes[1:])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFromFields"), Args: append(fields[:], stringLit(overflow))}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainTime." + method + " is a later slice"}
	}
}

// plainDateTimeStaticCall lowers Temporal.PlainDateTime.compare(a, b) or
// Temporal.PlainDateTime.from(x), the mirror of the PlainDate and PlainTime statics. compare
// lowers to value.PlainDateTimeCompare; from lowers to value.PlainDateTimeFrom for a
// PlainDateTime argument and to value.PlainDateTimeFromString for a string literal. Like the
// PlainDate from, a PlainDateTime carries a calendar, so the literal is gated on
// literalCalendarHosted and a string naming a calendar bento does not host hands back rather
// than emit a call the runtime parser would reject. A dynamic string or a property bag hands
// back for a later slice.
func (r *Renderer) plainDateTimeStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "compare":
		if len(argNodes) != 2 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.compare takes exactly two arguments"}
		}
		if !r.isPlainDateTime(argNodes[0]) || !r.isPlainDateTime(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.compare over an argument that is not a PlainDateTime (a string or bag to coerce) is a later slice"}
		}
		a, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		b, err := r.lowerExpr(argNodes[1])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateTimeCompare"), Args: []ast.Expr{a, b}}, nil
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.from with options is a later slice"}
		}
		if r.isPlainDateTime(argNodes[0]) {
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if !literalCalendarHosted(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.from over a string naming a calendar bento does not host is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.from over a dynamic string or a property bag is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime." + method + " is a later slice"}
	}
}

// newTemporal lowers new Temporal.<Type>(...), routing on the type name to the per-type
// constructor handler. A Temporal type this file does not host yet hands back.
func (r *Renderer) newTemporal(typeName string, argNodes []frontend.Node) (ast.Expr, error) {
	switch typeName {
	case "PlainDate":
		return r.newPlainDate(argNodes)
	case "PlainTime":
		return r.newPlainTime(argNodes)
	case "PlainDateTime":
		return r.newPlainDateTime(argNodes)
	case "Duration":
		return r.newDuration(argNodes)
	case "PlainYearMonth":
		return r.newPlainYearMonth(argNodes)
	case "PlainMonthDay":
		return r.newPlainMonthDay(argNodes)
	case "Instant":
		return r.newInstant(argNodes)
	case "ZonedDateTime":
		return r.newZonedDateTime(argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "new Temporal." + typeName + " is a later slice"}
	}
}

// goStringOf reads the Go string behind a lowered bento string expression, the
// receiver.ToGoString() the from-string factories need since they take a Go string but a
// string-typed argument lowers to a value.BStr. It is the dynamic-string counterpart of the
// strconv.Quote a string literal takes: a from over a string value not known until run time
// lowers the argument and reads its Go string through this.
func goStringOf(expr ast.Expr) ast.Expr {
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: expr, Sel: ident("ToGoString")}}
}

// plainDateToZonedArgs reads the arguments of Temporal.PlainDate.prototype.toZonedDateTime at
// compile time into a time-zone Go-string expression and an optional PlainTime expression. The
// argument is either a time-zone string, a literal or a string-typed value, in which case the
// wall clock defaults to midnight, or an options bag carrying a timeZone string and an optional
// plainTime that is a Temporal.PlainTime or a time string literal. A time-zone-like object, a
// dynamic plainTime, a missing timeZone, or any other shape hands back, since the zone or time
// would then depend on runtime data or a coercion this slice does not carry.
func (r *Renderer) plainDateToZonedArgs(what string, argNodes []frontend.Node) (ast.Expr, ast.Expr, error) {
	if len(argNodes) != 1 {
		return nil, nil, &NotYetLowerable{Reason: what + " takes one argument"}
	}
	n := argNodes[0]
	if tz, ok, err := r.timeZoneStringArg(n); err != nil {
		return nil, nil, err
	} else if ok {
		return tz, ident("nil"), nil
	}
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, nil, &NotYetLowerable{Reason: what + " over an argument that is not a time-zone string or an options bag is a later slice"}
	}
	var tz ast.Expr
	var pt ast.Expr = ident("nil")
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, nil, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, nil, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		switch key := r.prog.Text(kids[0]); key {
		case "timeZone":
			z, ok, err := r.timeZoneStringArg(kids[1])
			if err != nil {
				return nil, nil, err
			}
			if !ok {
				return nil, nil, &NotYetLowerable{Reason: what + " over a bag whose timeZone is not a string is a later slice"}
			}
			tz = z
		case "plainTime":
			p, err := r.plainTimeFieldArg(what, kids[1])
			if err != nil {
				return nil, nil, err
			}
			pt = p
		default:
			return nil, nil, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
	}
	if tz == nil {
		return nil, nil, &NotYetLowerable{Reason: what + " over a bag with no timeZone (a TypeError at run time) is a later slice"}
	}
	return tz, pt, nil
}

// timeZoneStringArg lowers a time-zone identifier argument into the Go string the runtime
// resolves. A string literal quotes straight through; a string-typed value reads its Go string
// at run time. It reports false when the argument is neither, so the caller can try another
// argument shape or hand back.
func (r *Renderer) timeZoneStringArg(n frontend.Node) (ast.Expr, bool, error) {
	if lit, ok := r.stringLiteralValue(n); ok {
		return stringLit(lit), true, nil
	}
	if r.isString(n) {
		e, err := r.lowerExpr(n)
		if err != nil {
			return nil, false, err
		}
		return goStringOf(e), true, nil
	}
	return nil, false, nil
}

// plainTimeFieldArg lowers a plainTime option into the *PlainTime the runtime pairs with the
// date. It is a Temporal.PlainTime expression or a time string literal parsed at run time;
// anything else hands back, since the time would then need a coercion this slice does not carry.
func (r *Renderer) plainTimeFieldArg(what string, n frontend.Node) (ast.Expr, error) {
	if r.isPlainTime(n) {
		return r.lowerExpr(n)
	}
	if lit, ok := r.stringLiteralValue(n); ok {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFromString"), Args: []ast.Expr{stringLit(lit)}}, nil
	}
	return nil, &NotYetLowerable{Reason: what + " over a bag whose plainTime is not a Temporal.PlainTime or a time string is a later slice"}
}

// hostedCalendar reads a calendar-identifier argument the runtime hosts, the literal
// "iso8601", "gregory", "roc", or "japanese" a caller names on a constructor or withCalendar. It
// reads the checker's literal type, so a const holding the id resolves the same as a
// bare literal, lowercases it since identifiers are case-insensitive, and returns the
// canonical form. It returns false for a dynamic string, whose value is unknown until
// run time, and for a calendar bento does not host yet, so the caller hands both back
// rather than emit a call the runtime would reject.
func (r *Renderer) hostedCalendar(node frontend.Node) (string, bool) {
	id, ok := r.stringLiteralValue(node)
	if !ok {
		return "", false
	}
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

// literalCalendarHosted reports whether a literal Temporal string names only a calendar
// bento hosts, so the from-string path may route it to the runtime parser. It returns
// false when the string carries a [u-ca=<id>] (or critical [!u-ca=<id>]) annotation whose
// id is not one of the hosted calendars, since the runtime parser throws a RangeError on an
// unhosted id while the specification would succeed with that calendar; such a string hands
// back at lowering instead. A string with no calendar annotation, or one naming a hosted
// calendar, is safe.
func literalCalendarHosted(s string) bool {
	const key = "[u-ca="
	i := strings.Index(s, key)
	if i < 0 {
		i = strings.Index(s, "[!u-ca=")
		if i < 0 {
			return true
		}
		i += len("[!u-ca=")
	} else {
		i += len(key)
	}
	end := strings.IndexByte(s[i:], ']')
	if end < 0 {
		return true // a malformed annotation; the runtime parser rejects it uniformly
	}
	switch strings.ToLower(s[i : i+end]) {
	case "iso8601", "gregory", "roc", "japanese":
		return true
	default:
		return false
	}
}

// literalISOCalendarOnly reports whether a literal Temporal string carries no calendar
// annotation or only the ISO calendar, the sole calendar bento's PlainYearMonth and
// PlainMonthDay host. Unlike literalCalendarHosted it rejects gregory, roc, and japanese too,
// since neither type has a calendar field to carry them: a full-date string naming such a
// calendar would parse to a non-ISO year-month or month-day the specification accepts but the
// runtime cannot represent, so the from path hands it back rather than drop the calendar and
// emit a wrong result.
func literalISOCalendarOnly(s string) bool {
	const key = "[u-ca="
	i := strings.Index(s, key)
	if i < 0 {
		i = strings.Index(s, "[!u-ca=")
		if i < 0 {
			return true
		}
		i += len("[!u-ca=")
	} else {
		i += len(key)
	}
	end := strings.IndexByte(s[i:], ']')
	if end < 0 {
		return true // a malformed annotation; the runtime parser rejects it uniformly
	}
	return strings.EqualFold(s[i:i+end], "iso8601")
}

// calendarLit builds the Go string-literal argument a calendar-aware constructor takes.
func calendarLit(cal string) ast.Expr {
	return &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(cal)}
}

// newPlainDate lowers new Temporal.PlainDate over its three number arguments (isoYear,
// isoMonth, isoDay) and an optional fourth calendar argument. A literal iso8601 or gregory
// calendar routes to NewPlainDateCal; a dynamic or unhosted calendar hands back. Each
// numeric argument must lower as a number, so a non-number component hands back rather than
// coerce; the runtime constructor runs ToIntegerWithTruncation and RejectISODate, so an
// out-of-range date throws a RangeError at run time the way the specification requires.
func (r *Renderer) newPlainDate(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 3 || len(argNodes) > 4 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainDate takes three components and an optional calendar"}
	}
	args := make([]ast.Expr, 3)
	for i := range 3 {
		if !r.isNumber(argNodes[i]) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDate with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(argNodes[i])
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	if len(argNodes) == 4 {
		cal, ok := r.hostedCalendar(argNodes[3])
		if !ok {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDate over a calendar other than a literal iso8601 or gregory is a later slice"}
		}
		return &ast.CallExpr{Fun: sel("value", "NewPlainDateCal"), Args: append(args, calendarLit(cal))}, nil
	}
	return &ast.CallExpr{Fun: sel("value", "NewPlainDate"), Args: args}, nil
}

// newPlainTime lowers new Temporal.PlainTime over its up-to-six number components (hour,
// minute, second, millisecond, microsecond, nanosecond), every one optional and defaulting
// to zero. A missing trailing component is padded with a float64 zero here so the runtime
// constructor always sees six numbers. Each supplied argument must lower as a number, so a
// non-number component hands back; the runtime runs ToIntegerWithTruncation and RejectTime,
// so an out-of-range field throws a RangeError at run time.
func (r *Renderer) newPlainTime(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 6 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainTime takes at most six components"}
	}
	args := make([]ast.Expr, 6)
	for i := range args {
		if i >= len(argNodes) {
			args[i] = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
			continue
		}
		if !r.isNumber(argNodes[i]) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainTime with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(argNodes[i])
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewPlainTime"), Args: args}, nil
}

// newPlainDateTime lowers new Temporal.PlainDateTime over its three required date components
// (isoYear, isoMonth, isoDay), up to six optional time components (hour, minute, second,
// millisecond, microsecond, nanosecond), each defaulting to zero, and an optional tenth
// calendar argument. The calendar is positional after all nine numbers, so it appears only
// when ten arguments are given; a literal iso8601 or gregory calendar routes to
// NewPlainDateTimeCal and a dynamic or unhosted one hands back. A missing trailing time
// component is padded with a float64 zero so the runtime constructor always sees nine
// numbers. Each supplied numeric argument must lower as a number, so a non-number component
// hands back; the runtime runs ToIntegerWithTruncation then RejectISODate and RejectTime, so
// an out-of-range date or time throws a RangeError at run time.
func (r *Renderer) newPlainDateTime(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 3 || len(argNodes) > 10 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainDateTime takes three to nine components and an optional calendar"}
	}
	hasCal := len(argNodes) == 10
	timeArgs := argNodes
	if hasCal {
		timeArgs = argNodes[:9]
	}
	args := make([]ast.Expr, 9)
	for i := range args {
		if i >= len(timeArgs) {
			args[i] = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
			continue
		}
		if !r.isNumber(timeArgs[i]) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDateTime with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(timeArgs[i])
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	if hasCal {
		cal, ok := r.hostedCalendar(argNodes[9])
		if !ok {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDateTime over a calendar other than a literal iso8601 or gregory is a later slice"}
		}
		return &ast.CallExpr{Fun: sel("value", "NewPlainDateTimeCal"), Args: append(args, calendarLit(cal))}, nil
	}
	return &ast.CallExpr{Fun: sel("value", "NewPlainDateTime"), Args: args}, nil
}

// newDuration lowers new Temporal.Duration over its up-to-ten number components (years,
// months, weeks, days, hours, minutes, seconds, milliseconds, microseconds, nanoseconds),
// every one optional and defaulting to zero. A missing trailing component is padded with a
// float64 zero here so the runtime constructor always sees ten numbers. Each supplied
// argument must lower as a number, so a non-number component hands back; the runtime runs
// ToIntegerIfIntegral and RejectDuration, so a fractional, non-finite, mixed-sign, or
// out-of-range component throws a RangeError at run time the way the specification requires.
func (r *Renderer) newDuration(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) > 10 {
		return nil, &NotYetLowerable{Reason: "new Temporal.Duration takes at most ten components"}
	}
	args := make([]ast.Expr, 10)
	for i := range args {
		if i >= len(argNodes) {
			args[i] = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
			continue
		}
		if !r.isNumber(argNodes[i]) {
			return nil, &NotYetLowerable{Reason: "new Temporal.Duration with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(argNodes[i])
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewDuration"), Args: args}, nil
}

// newPlainYearMonth lowers new Temporal.PlainYearMonth over its two number arguments (isoYear,
// isoMonth); a third calendar argument selects a non-ISO calendar and a fourth reference-day
// argument overrides the default, neither of which this slice carries, so more than two
// arguments hand back. Each argument must lower as a number, so a non-number component hands
// back rather than coerce; the runtime runs ToIntegerWithTruncation and RejectISOYearMonth, so
// an out-of-range year-month throws a RangeError at run time the way the specification requires.
func (r *Renderer) newPlainYearMonth(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainYearMonth with a calendar or reference-day argument or fewer than two components is a later slice"}
	}
	args := make([]ast.Expr, 2)
	for i, node := range argNodes {
		if !r.isNumber(node) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainYearMonth with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(node)
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewPlainYearMonth"), Args: args}, nil
}

// newPlainMonthDay lowers new Temporal.PlainMonthDay over its two number arguments (isoMonth,
// isoDay, the month first); a third calendar argument selects a non-ISO calendar and a fourth
// reference-year argument overrides the default leap year, neither of which this slice carries,
// so more than two arguments hand back. Each argument must lower as a number, so a non-number
// component hands back rather than coerce; the runtime runs ToIntegerWithTruncation and
// RejectISOMonthDay against the leap reference year, so an out-of-range month-day throws a
// RangeError at run time the way the specification requires.
func (r *Renderer) newPlainMonthDay(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 2 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainMonthDay with a calendar or reference-year argument or fewer than two components is a later slice"}
	}
	args := make([]ast.Expr, 2)
	for i, node := range argNodes {
		if !r.isNumber(node) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainMonthDay with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(node)
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "NewPlainMonthDay"), Args: args}, nil
}
