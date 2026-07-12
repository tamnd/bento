package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the Temporal area (10_advanced group 6), one type per cut. Seven are
// hosted so far: PlainDate, a calendar date with no time and no zone over the ISO 8601
// calendar; PlainTime, a wall-clock time with no date and no zone; PlainDateTime, a date
// paired with a wall-clock time; Duration, a span of time as ten signed component counts;
// PlainYearMonth, a calendar year and month with no day; PlainMonthDay, a calendar month and
// day with no year; and Instant, an exact point on the UTC time line as a nanosecond count.
// For the plain types, construction, the static from over the same type (and, for the ordered
// types, compare), the clean field getters, and the equals, toString, and toJSON methods lower
// to the matching value runtime type. Duration hosts construction, the field getters plus sign
// and blank, negated and abs, toString and toJSON, and from over a Duration. Instant hosts
// construction and the two epoch factories, the epoch-milliseconds and epoch-nanoseconds
// getters, compare, equals, toString and toJSON, and from over an Instant. Everything else, the
// arithmetic, the balancing and rounding, the cross-type conversions, from over a string or a
// property bag, and the getters the checker types number | undefined, hands back with a named
// reason so the compiler reports the exact ceiling.
//
// Each Temporal type follows the host-type model RegExp and the collections use: it is a bare
// pointer in the generated Go (*value.PlainDate, *value.PlainTime, *value.PlainDateTime,
// *value.Duration, *value.PlainYearMonth, *value.PlainMonthDay, *value.Instant), recognized by
// its declaring symbol name rather than a dedicated type flag.
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
// an Instant argument (the copy the specification makes) and hands back for a string,
// which needs the ISO parser this slice does not carry.
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
		if !r.isInstant(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Instant.from over a string is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "InstantFrom"), Args: []ast.Expr{arg}}, nil
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
// to value.DurationFrom (the copy the specification makes); from over a string or a property
// bag needs parsing this slice does not carry, and compare needs a relativeTo reference to
// balance the calendar units, so both hand back.
func (r *Renderer) durationStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.from takes exactly one argument"}
		}
		if !r.isDuration(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "DurationFrom"), Args: []ast.Expr{arg}}, nil
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
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.prototype." + method + " is a later slice"}
	}
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
	default:
		return nil, &NotYetLowerable{Reason: "Temporal." + typeName + " is a later slice"}
	}
}

// plainYearMonthStaticCall lowers Temporal.PlainYearMonth.compare(a, b) or
// Temporal.PlainYearMonth.from(x), the mirror of the PlainDate statics. compare lowers to
// value.PlainYearMonthCompare; from lowers to value.PlainYearMonthFrom for a PlainYearMonth
// argument and hands back for a string or a bag.
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
		if !r.isPlainYearMonth(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainYearMonthFrom"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth." + method + " is a later slice"}
	}
}

// plainMonthDayStaticCall lowers Temporal.PlainMonthDay.from(x). A month-day has no compare
// static (month-days are not ordered), so only from is handled: it lowers to
// value.PlainMonthDayFrom for a PlainMonthDay argument and hands back for a string or a bag.
func (r *Renderer) plainMonthDayStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch method {
	case "from":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.from with options is a later slice"}
		}
		if !r.isPlainMonthDay(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainMonthDay.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainMonthDayFrom"), Args: []ast.Expr{arg}}, nil
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
		if !r.isPlainDate(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDate.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateFrom"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate." + method + " is a later slice"}
	}
}

// plainTimeStaticCall lowers Temporal.PlainTime.compare(a, b) or Temporal.PlainTime.from(x),
// the mirror of the PlainDate statics. compare lowers to value.PlainTimeCompare; from lowers
// to value.PlainTimeFrom for a PlainTime argument and hands back for a string or a bag.
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
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from with options is a later slice"}
		}
		if !r.isPlainTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainTime.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFrom"), Args: []ast.Expr{arg}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainTime." + method + " is a later slice"}
	}
}

// plainDateTimeStaticCall lowers Temporal.PlainDateTime.compare(a, b) or
// Temporal.PlainDateTime.from(x), the mirror of the PlainDate and PlainTime statics. compare
// lowers to value.PlainDateTimeCompare; from lowers to value.PlainDateTimeFrom for a
// PlainDateTime argument and hands back for a string or a bag.
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
		if !r.isPlainDateTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.from over a string or a property bag is a later slice"}
		}
		arg, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFrom"), Args: []ast.Expr{arg}}, nil
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
	default:
		return nil, &NotYetLowerable{Reason: "new Temporal." + typeName + " is a later slice"}
	}
}

// newPlainDate lowers new Temporal.PlainDate over its three number arguments (isoYear,
// isoMonth, isoDay); a fourth calendar argument selects a non-ISO calendar, which this
// slice does not carry, so it hands back. Each argument must lower as a number, so a
// non-number component hands back rather than coerce; the runtime constructor runs
// ToIntegerWithTruncation and RejectISODate, so an out-of-range date throws a RangeError
// at run time the way the specification requires.
func (r *Renderer) newPlainDate(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) != 3 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainDate with a calendar argument or fewer than three components is a later slice"}
	}
	args := make([]ast.Expr, 3)
	for i, node := range argNodes {
		if !r.isNumber(node) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDate with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(node)
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
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
// (isoYear, isoMonth, isoDay) and up to six optional time components (hour, minute, second,
// millisecond, microsecond, nanosecond), each time field defaulting to zero. Fewer than three
// arguments, or a tenth calendar argument, hands back: this slice carries only the ISO
// calendar. A missing trailing time component is padded with a float64 zero so the runtime
// constructor always sees nine numbers. Each supplied argument must lower as a number, so a
// non-number component hands back; the runtime runs ToIntegerWithTruncation then RejectISODate
// and RejectTime, so an out-of-range date or time throws a RangeError at run time.
func (r *Renderer) newPlainDateTime(argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) < 3 || len(argNodes) > 9 {
		return nil, &NotYetLowerable{Reason: "new Temporal.PlainDateTime with a calendar argument or fewer than three components is a later slice"}
	}
	args := make([]ast.Expr, 9)
	for i := range args {
		if i >= len(argNodes) {
			args[i] = &ast.BasicLit{Kind: token.FLOAT, Value: "0"}
			continue
		}
		if !r.isNumber(argNodes[i]) {
			return nil, &NotYetLowerable{Reason: "new Temporal.PlainDateTime with a non-number component is a later slice"}
		}
		lowered, err := r.lowerExpr(argNodes[i])
		if err != nil {
			return nil, err
		}
		args[i] = lowered
	}
	r.requireImport(valuePkg)
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
