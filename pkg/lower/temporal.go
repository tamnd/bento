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
// beyond the ones handled hands back. add and subtract fold a Duration's time part into the
// epoch count, rejecting the calendar units at run time. until and since report the exact-time
// difference balanced from largestUnit down and rounded at smallestUnit. round rounds the epoch
// count to a time unit against the day length. toZonedDateTimeISO pairs the instant with a time
// zone under the ISO calendar. toLocaleString hands back with a named reason.
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
	case "add", "subtract":
		what := "Temporal.Instant.prototype." + method
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: what + " takes exactly one argument"}
		}
		dur, err := r.durationArg(what, argNodes[0])
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
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AddDuration")}, Args: []ast.Expr{dur}}, nil
	case "until", "since":
		what := "Temporal.Instant.prototype." + method
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: what + " takes at least one argument"}
		}
		if !r.isInstant(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.Instant is a later slice"}
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		largestUnit, smallestUnit, increment, mode, err := r.instantDifferenceOptions(what, argNodes[1:])
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
	case "round":
		what := "Temporal.Instant.prototype.round"
		unit, increment, mode, err := r.plainTimeRoundOptions(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Round")}, Args: []ast.Expr{stringLit(unit), increment, stringLit(mode)}}, nil
	case "toZonedDateTimeISO":
		what := "Temporal.Instant.prototype.toZonedDateTimeISO"
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: what + " takes exactly one argument"}
		}
		tz, ok, err := r.timeZoneStringArg(argNodes[0])
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a time-zone string is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToZonedDateTimeISO")}, Args: []ast.Expr{tz}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Instant.prototype." + method + " is a later slice"}
	}
}

// instantDifferenceOptions reads the options bag for Temporal.Instant.prototype.until and since.
// It mirrors plainTimeDifferenceOptions over the same time units and modes, with one difference:
// an Instant has no wall clock or day, so the default and the auto largestUnit resolve to the
// coarser of second and the smallestUnit, not to hour. A dynamic unit or mode, an out-of-set
// unit or mode, or an unknown key hands back; the runtime rejects a largestUnit smaller than the
// smallestUnit and an out-of-range increment, so those ride through as a RangeError. A calendar
// unit such as day is not in the accepted set, so it hands back rather than lowering to a throw.
func (r *Renderer) instantDifferenceOptions(what string, argNodes []frontend.Node) (largestUnit, smallestUnit string, increment ast.Expr, mode string, err error) {
	largestUnit, smallestUnit, mode = "auto", "nanosecond", "trunc"
	increment = &ast.BasicLit{Kind: token.FLOAT, Value: "1"}
	if len(argNodes) > 1 {
		return "", "", nil, "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	if len(argNodes) == 1 {
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
				if lit != "auto" && !slices.Contains(plainTimeRoundUnits[:], lit) {
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
	}
	if largestUnit == "auto" {
		// An Instant defaults to second, but a coarser smallestUnit raises the floor.
		secondRank := slices.Index(plainTimeRoundUnits[:], "second")
		smallRank := slices.Index(plainTimeRoundUnits[:], smallestUnit)
		largestUnit = plainTimeRoundUnits[min(secondRank, smallRank)]
	}
	return largestUnit, smallestUnit, increment, mode, nil
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
// day-length getter hoursInDay reads the length of the local day, twenty-four hours except across a
// daylight-saving transition, off the two adjacent midnights.
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
	case "hoursInDay":
		return "HoursInDay", true
	}
	return plainDateTimeAccessor(prop)
}

// zonedDateTimeMethodCall lowers a method call on a ZonedDateTime receiver, the mirror of
// plainDateTimeMethodCall. equals(other) compares two zoned date-times, toString and toJSON
// render the round-trippable string, and the conversions toInstant, toPlainDate, toPlainTime,
// and toPlainDateTime drop the zone or narrow to a plain type; none takes options in this
// slice, so a call with arguments beyond the ones handled hands back. add and subtract move the
// value by a duration, the calendar part added in the calendar and the exact-time part folded on
// as nanoseconds after the offset re-resolves, reading the overflow option the same way the plain
// date-time movers do. until and since return the difference under a largestUnit that spans the
// calendar and exact-time units and defaults to hour, and round rounds the wall clock and re-resolves
// the offset over the shared day-and-time round-options reader. The remaining reshaping (with,
// withPlainTime, withTimeZone, withCalendar) and startOfDay lower, while the transition queries and
// toLocaleString hand back with a named reason.
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
	case "add", "subtract":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.ZonedDateTime.prototype." + method
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
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AddDuration")}, Args: []ast.Expr{dur, stringLit(overflow)}}, nil
	case "until", "since":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.ZonedDateTime.prototype." + method
		if !r.isZonedDateTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over a non-ZonedDateTime argument (a string or bag to coerce) is a later slice"}
		}
		largestUnit, err := r.zonedDateTimeDifferenceOptions(what, argNodes[1:])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		name := "Until"
		if method == "since" {
			name = "Since"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}, Args: []ast.Expr{other, stringLit(largestUnit)}}, nil
	case "round":
		what := "Temporal.ZonedDateTime.prototype.round"
		unit, increment, mode, err := r.plainDateTimeRoundOptions(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Round")}, Args: []ast.Expr{stringLit(unit), increment, stringLit(mode)}}, nil
	case "with":
		what := "Temporal.ZonedDateTime.prototype.with"
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: what + " takes at least one argument"}
		}
		fields, err := r.plainDateTimeBagFields(what, argNodes[0])
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
		args := append(fields[:], stringLit(overflow))
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithFields")}, Args: args}, nil
	case "withPlainTime":
		what := "Temporal.ZonedDateTime.prototype.withPlainTime"
		time, err := r.plainDateTimeWithPlainTimeArg(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithPlainTime")}, Args: []ast.Expr{time}}, nil
	case "withTimeZone":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.withTimeZone takes exactly one argument"}
		}
		tz, ok, err := r.timeZoneStringArg(argNodes[0])
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.withTimeZone over a time zone that is not a string is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithTimeZone")}, Args: []ast.Expr{tz}}, nil
	case "withCalendar":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.withCalendar takes exactly one argument"}
		}
		lit, ok := r.stringLiteralValue(argNodes[0])
		if !ok || (!strings.EqualFold(lit, "iso8601") && !strings.EqualFold(lit, "iso")) {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.withCalendar over a calendar other than a literal iso8601 is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithCalendar")}}, nil
	case "startOfDay":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.startOfDay takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("StartOfDay")}}, nil
	case "getTimeZoneTransition":
		// The next or previous offset transition needs the zone's transition list. Go's time
		// package resolves an offset at an instant but exposes no way to enumerate the transitions,
		// and the reference polyfill only approximates the list with a wall-clock-dependent bounded
		// probe, which would make the compiled output nondeterministic. So this hands back honestly
		// rather than emit a probe that could read the wrong answer past its horizon.
		return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.prototype.getTimeZoneTransition needs a zone transition list the host time package does not expose"}
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
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from takes at least one argument"}
		}
		if r.isZonedDateTime(argNodes[0]) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a ZonedDateTime with options is a later slice"}
			}
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a string with options is a later slice"}
			}
			if !literalISOCalendarOnly(lit) {
				return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a string naming a non-ISO calendar is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if argNodes[0].Kind() == frontend.NodeObjectLiteralExpression {
			return r.zonedDateTimeFromBag("Temporal.ZonedDateTime.from", argNodes[0], argNodes[1:])
		}
		return nil, &NotYetLowerable{Reason: "Temporal.ZonedDateTime.from over a dynamic string is a later slice"}
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
// reshaped Duration, with overlays a partial-duration bag onto the receiver (a pure reshape,
// no balancing, so it needs no relativeTo), toString and toJSON render the ISO 8601 duration
// string, and add and subtract fold the receiver and a Duration operand over a fixed 24-hour
// day (the reduced profile takes no relativeTo, so both throw a RangeError at run time when an
// operand carries years, months, or weeks). total converts the duration to one unit against an
// optional relativeTo PlainDate. round rounds at a smallestUnit and re-balances to a largestUnit
// against an optional relativeTo, throwing at run time without a reference for a calendar unit.
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
	case "with":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype.with takes exactly one argument"}
		}
		fields, err := r.durationPartialBag("Temporal.Duration.prototype.with", argNodes[0])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("With")}, Args: fields[:]}, nil
	case "add", "subtract":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " takes exactly one argument"}
		}
		if !r.isDuration(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " over an argument that is not a Temporal.Duration (a string or bag to coerce) is a later slice"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		name := "Add"
		if method == "subtract" {
			name = "Subtract"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}, Args: []ast.Expr{other}}, nil
	case "total":
		unit, rel, err := r.durationTotalOptions("Temporal.Duration.prototype.total", argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Total")}, Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(unit)}, rel}}, nil
	case "round":
		sm, lg, inc, mode, rel, err := r.durationRoundOptions("Temporal.Duration.prototype.round", argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Round")}, Args: []ast.Expr{
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(sm)},
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lg)},
			inc,
			&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(mode)},
			rel,
		}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Duration.prototype." + method + " is a later slice"}
	}
}

// durationStaticCall lowers a static call on Temporal.Duration. from over a Duration lowers
// to value.DurationFrom (the copy the specification makes), from over a string literal to
// value.DurationFromString, and from over an object literal to value.DurationFromFields over
// its ten present-or-absent fields. A Duration carries no calendar, so the literal and bag
// need no gate; a value not statically typed as a string hands back. compare orders two
// durations over an optional relativeTo PlainDate, folding day-and-time durations on a fixed
// 24-hour day and resolving calendar units against the reference.
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
		if argNodes[0].Kind() == frontend.NodeObjectLiteralExpression {
			fields, err := r.durationPartialBag("Temporal.Duration.from", argNodes[0])
			if err != nil {
				return nil, err
			}
			return &ast.CallExpr{Fun: sel("value", "DurationFromFields"), Args: fields[:]}, nil
		}
		return nil, &NotYetLowerable{Reason: "Temporal.Duration.from over a value not statically typed as a string is a later slice"}
	case "compare":
		if len(argNodes) < 2 || len(argNodes) > 3 {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.compare takes two durations and an optional options argument"}
		}
		if !r.isDuration(argNodes[0]) || !r.isDuration(argNodes[1]) {
			return nil, &NotYetLowerable{Reason: "Temporal.Duration.compare over an argument that is not a Temporal.Duration (a string or bag to coerce) is a later slice"}
		}
		var rel ast.Expr = ident("nil")
		if len(argNodes) == 3 {
			expr, err := r.durationRelativeToOption("Temporal.Duration.compare", argNodes[2])
			if err != nil {
				return nil, err
			}
			rel = expr
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
		return &ast.CallExpr{Fun: sel("value", "DurationCompare"), Args: []ast.Expr{a, b, rel}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.Duration." + method + " is a later slice"}
	}
}

// durationTotalUnits maps the units Temporal.Duration.prototype.total accepts, singular and
// plural, to the singular form the runtime takes. total has no "auto", so every unit names a
// concrete scale from year down to nanosecond.
var durationTotalUnits = map[string]string{
	"year": "year", "years": "year", "month": "month", "months": "month",
	"week": "week", "weeks": "week", "day": "day", "days": "day",
	"hour": "hour", "hours": "hour", "minute": "minute", "minutes": "minute",
	"second": "second", "seconds": "second", "millisecond": "millisecond", "milliseconds": "millisecond",
	"microsecond": "microsecond", "microseconds": "microsecond", "nanosecond": "nanosecond", "nanoseconds": "nanosecond",
}

// durationTotalOptions reads the options of Temporal.Duration.prototype.total at compile time.
// total takes a required unit, given either as a bare string or as the unit field of an options
// object, and an optional relativeTo the calendar units resolve against. It returns the unit in
// its singular form and the lowered relativeTo, or the nil identifier when none was given, which
// the runtime reads as no reference. A relativeTo that is not a Temporal.PlainDate (a
// ZonedDateTime, a PlainDateTime, or a string or bag to coerce) hands back, as do a missing or
// invalid unit, a non-literal unit, an unknown key, or a spread or shorthand member.
func (r *Renderer) durationTotalOptions(what string, argNodes []frontend.Node) (string, ast.Expr, error) {
	if len(argNodes) != 1 {
		return "", nil, &NotYetLowerable{Reason: what + " takes exactly one argument"}
	}
	n := argNodes[0]
	if lit, ok := r.stringLiteralValue(n); ok {
		unit, ok := durationTotalUnits[lit]
		if !ok {
			return "", nil, &NotYetLowerable{Reason: what + " with the invalid unit " + lit + " (a RangeError at run time) is a later slice"}
		}
		return unit, ident("nil"), nil
	}
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", nil, &NotYetLowerable{Reason: what + " options that are neither a string nor an object literal are a later slice"}
	}
	unit := ""
	var rel ast.Expr = ident("nil")
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", nil, &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", nil, &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "unit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", nil, &NotYetLowerable{Reason: what + " with a non-literal unit is a later slice"}
			}
			u, ok := durationTotalUnits[lit]
			if !ok {
				return "", nil, &NotYetLowerable{Reason: what + " with the invalid unit " + lit + " (a RangeError at run time) is a later slice"}
			}
			unit = u
		case "relativeTo":
			if !r.isPlainDate(kids[1]) {
				return "", nil, &NotYetLowerable{Reason: what + " with a relativeTo that is not a Temporal.PlainDate is a later slice"}
			}
			expr, err := r.lowerExpr(kids[1])
			if err != nil {
				return "", nil, err
			}
			rel = expr
		default:
			return "", nil, &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	if unit == "" {
		return "", nil, &NotYetLowerable{Reason: what + " without a unit (a RangeError at run time) is a later slice"}
	}
	return unit, rel, nil
}

// durationRelativeToOption reads a relativeTo from an options object literal, the only option
// Temporal.Duration.compare takes. It returns the lowered PlainDate or the nil identifier when
// the object has no relativeTo. A relativeTo that is not a Temporal.PlainDate, an options value
// that is not an object literal, an unknown key, or a spread or shorthand member hands back.
func (r *Renderer) durationRelativeToOption(what string, n frontend.Node) (ast.Expr, error) {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, &NotYetLowerable{Reason: what + " options that are not an object literal are a later slice"}
	}
	var rel ast.Expr = ident("nil")
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "relativeTo":
			if !r.isPlainDate(kids[1]) {
				return nil, &NotYetLowerable{Reason: what + " with a relativeTo that is not a Temporal.PlainDate is a later slice"}
			}
			expr, err := r.lowerExpr(kids[1])
			if err != nil {
				return nil, err
			}
			rel = expr
		default:
			return nil, &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	return rel, nil
}

// durationRoundOptions reads the options of Temporal.Duration.prototype.round at compile time.
// round takes a required rounding target, given either as a bare string smallestUnit or as an
// options object carrying smallestUnit, largestUnit, roundingIncrement, roundingMode, and
// relativeTo. It returns the smallest and largest units in singular form, each empty when the
// option was absent so the runtime supplies the default, the lowered increment (default 1), the
// mode (default halfExpand), and the lowered relativeTo or the nil identifier. A relativeTo that
// is not a Temporal.PlainDate, a non-literal or invalid unit or mode, a non-number increment, a
// bag with neither smallestUnit nor largestUnit, an unknown key, or a spread or shorthand member
// hands back.
func (r *Renderer) durationRoundOptions(what string, argNodes []frontend.Node) (smallestUnit, largestUnit string, increment ast.Expr, mode string, rel ast.Expr, err error) {
	increment = &ast.BasicLit{Kind: token.FLOAT, Value: "1"}
	mode = "halfExpand"
	rel = ident("nil")
	if len(argNodes) != 1 {
		return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " takes exactly one argument"}
	}
	n := argNodes[0]
	if lit, ok := r.stringLiteralValue(n); ok {
		u, ok := durationTotalUnits[lit]
		if !ok {
			return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with the invalid smallestUnit " + lit + " (a RangeError at run time) is a later slice"}
		}
		return u, "", increment, mode, rel, nil
	}
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " options that are neither a string nor an object literal are a later slice"}
	}
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		switch key {
		case "smallestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with a non-literal smallestUnit is a later slice"}
			}
			u, ok := durationTotalUnits[lit]
			if !ok {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with the invalid smallestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			smallestUnit = u
		case "largestUnit":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with a non-literal largestUnit is a later slice"}
			}
			if lit == "auto" {
				largestUnit = "auto"
				continue
			}
			u, ok := durationTotalUnits[lit]
			if !ok {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with the invalid largestUnit " + lit + " (a RangeError at run time) is a later slice"}
			}
			largestUnit = u
		case "roundingIncrement":
			if !r.isNumber(kids[1]) {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with a non-number roundingIncrement is a later slice"}
			}
			val, lowerErr := r.lowerExpr(kids[1])
			if lowerErr != nil {
				return "", "", nil, "", nil, lowerErr
			}
			increment = val
		case "roundingMode":
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with a non-literal roundingMode is a later slice"}
			}
			if !slices.Contains(plainTimeRoundModes[:], lit) {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with the invalid roundingMode " + lit + " (a RangeError at run time) is a later slice"}
			}
			mode = lit
		case "relativeTo":
			if !r.isPlainDate(kids[1]) {
				return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with a relativeTo that is not a Temporal.PlainDate is a later slice"}
			}
			expr, lowerErr := r.lowerExpr(kids[1])
			if lowerErr != nil {
				return "", "", nil, "", nil, lowerErr
			}
			rel = expr
		default:
			return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	if smallestUnit == "" && (largestUnit == "" || largestUnit == "auto") {
		return "", "", nil, "", nil, &NotYetLowerable{Reason: what + " without a smallestUnit or largestUnit (a RangeError at run time) is a later slice"}
	}
	return smallestUnit, largestUnit, increment, mode, rel, nil
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

// yearMonthFieldKeys is the ordered set of numeric PlainYearMonth field names a property bag may
// carry: the calendar year and the month, the two fields WithFields takes. monthCode is a
// recognized key too, but it names the month through a code the reader resolves to a number, and
// the era fields resolve a year through the calendar a later slice carries, so the bag reader
// handles a literal monthCode itself and hands back on the era fields.
var yearMonthFieldKeys = [2]string{"year", "month"}

// plainYearMonthBagFields reads a PlainYearMonth.prototype.with bag at compile time and returns the
// year and month as present or absent optionals in WithFields order. A present year or month must
// be a number, and a monthCode must be a string literal of the form "MNN" that resolves to its
// month. A bag carrying both month and monthCode, an era field, a day, or any other key, a spread
// or shorthand member, a repeated field, a computed key, a non-number value, an empty bag (a
// TypeError at run time), or a dynamic or malformed monthCode hands back rather than emitting a
// wrong or partial reshape.
func (r *Renderer) plainYearMonthBagFields(what string, n frontend.Node) ([2]ast.Expr, error) {
	var fields [2]ast.Expr
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return fields, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var seen [2]bool
	monthCodeSeen := false
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
		if key == "monthCode" {
			if seen[1] {
				return fields, &NotYetLowerable{Reason: what + " over a bag carrying both month and monthCode is a later slice"}
			}
			lit, ok := r.stringLiteralValue(kids[1])
			if !ok {
				return fields, &NotYetLowerable{Reason: what + " over a bag whose monthCode is dynamic is a later slice"}
			}
			month, ok := monthFromCode(lit)
			if !ok {
				return fields, &NotYetLowerable{Reason: what + " over a bag whose monthCode " + lit + " is not a plain ISO month code is a later slice"}
			}
			fields[1] = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("float64")), Args: []ast.Expr{&ast.BasicLit{Kind: token.INT, Value: strconv.Itoa(month)}}}
			seen[1] = true
			monthCodeSeen = true
			present = true
			continue
		}
		idx := -1
		for i, k := range yearMonthFieldKeys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return fields, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if seen[idx] {
			if idx == 1 && monthCodeSeen {
				return fields, &NotYetLowerable{Reason: what + " over a bag carrying both month and monthCode is a later slice"}
			}
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

// yearMonthToPlainDateDay reads the day from a PlainYearMonth.prototype.toPlainDate argument bag at
// compile time. The argument must be an object literal carrying a numeric day; a spread or
// shorthand member, a computed key, a non-number day, a missing day (a TypeError at run time), a
// repeated day, or any other key hands back. toPlainDate takes no overflow option, so the runtime
// always constrains.
func (r *Renderer) yearMonthToPlainDateDay(what string, n frontend.Node) (ast.Expr, error) {
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return nil, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var day ast.Expr
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		if key != "day" {
			return nil, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if day != nil {
			return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field day is a later slice"}
		}
		if !r.isNumber(kids[1]) {
			return nil, &NotYetLowerable{Reason: what + " over a bag whose day is not a number is a later slice"}
		}
		val, err := r.lowerExpr(kids[1])
		if err != nil {
			return nil, err
		}
		day = val
	}
	if day == nil {
		return nil, &NotYetLowerable{Reason: what + " over a bag missing the field day (a TypeError at run time) is a later slice"}
	}
	return day, nil
}

// monthFromCode resolves a plain ISO month code of the form "MNN" (M01 through M12) to its numeric
// month. It rejects a leap-month code (a trailing L) and any malformed string, since the ISO
// calendar has no leap months and a later slice carries the non-ISO calendars.
func monthFromCode(code string) (int, bool) {
	if len(code) != 3 || code[0] != 'M' {
		return 0, false
	}
	m, err := strconv.Atoi(code[1:])
	if err != nil || m < 1 || m > 12 {
		return 0, false
	}
	return m, true
}

var dateTimeFieldKeys = [9]string{"year", "month", "day", "hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}

// plainDateTimeBagFields reads a PlainDateTime.prototype.with bag at compile time and returns the
// year, month, day, and the six time fields as present or absent optionals in WithFields order. A
// present field must be a number; an absent one becomes None so WithFields keeps the receiver's
// value. An item that is not an object literal, a spread or shorthand member, a repeated field, a
// non-number value, an empty bag (a TypeError at run time), or a key outside the numeric nine,
// including monthCode and the era fields, hands back rather than emitting a wrong or partial
// reshape.
func (r *Renderer) plainDateTimeBagFields(what string, n frontend.Node) ([9]ast.Expr, error) {
	var fields [9]ast.Expr
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return fields, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var seen [9]bool
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
		for i, k := range dateTimeFieldKeys {
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

// plainYearMonthDifferenceUnits maps the year-month difference units, singular and plural, to
// the singular form the runtime takes. "auto" resolves to "year", the default largestUnit. A
// year-month has no day, so only year and month are valid; a week or day largestUnit is a
// RangeError at run time and hands back here.
var plainYearMonthDifferenceUnits = map[string]string{
	"auto": "year", "year": "year", "years": "year", "month": "month", "months": "month",
}

// plainYearMonthDifferenceOptions reads the options of PlainYearMonth.prototype.until and since
// at compile time and returns the largestUnit, defaulting to year. It accepts largestUnit as a
// string literal in the year-month unit set; a smallestUnit, roundingIncrement, or roundingMode
// would round the duration, a later slice, so any of those, a non-literal or out-of-set
// largestUnit, an unknown key, or a spread or shorthand member hands back.
func (r *Renderer) plainYearMonthDifferenceOptions(what string, argNodes []frontend.Node) (string, error) {
	largestUnit := "year"
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
			unit, ok := plainYearMonthDifferenceUnits[lit]
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
	case "add", "subtract":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainDateTime.prototype." + method
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
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("AddDateTime")}, Args: []ast.Expr{dur, stringLit(overflow)}}, nil
	case "until", "since":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainDateTime.prototype." + method
		if !r.isPlainDateTime(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainDateTime is a later slice"}
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		largestUnit, err := r.plainDateTimeDifferenceOptions(what, argNodes[1:])
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
	case "round":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype.round takes at least one argument"}
		}
		unit, increment, mode, err := r.plainDateTimeRoundOptions("Temporal.PlainDateTime.prototype.round", argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("Round")}, Args: []ast.Expr{stringLit(unit), increment, stringLit(mode)}}, nil
	case "with":
		what := "Temporal.PlainDateTime.prototype.with"
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: what + " takes at least one argument"}
		}
		fields, err := r.plainDateTimeBagFields(what, argNodes[0])
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
		args := append(fields[:], stringLit(overflow))
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithFields")}, Args: args}, nil
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
	case "toPlainDate", "toPlainTime":
		if len(argNodes) != 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " takes no arguments"}
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		name := "ToPlainDate"
		if method == "toPlainTime" {
			name = "ToPlainTime"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(name)}}, nil
	case "withPlainTime":
		what := "Temporal.PlainDateTime.prototype.withPlainTime"
		time, err := r.plainDateTimeWithPlainTimeArg(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("WithPlainTime")}, Args: []ast.Expr{time}}, nil
	case "toZonedDateTime":
		what := "Temporal.PlainDateTime.prototype.toZonedDateTime"
		tz, disambiguation, err := r.plainDateTimeToZonedArgs(what, argNodes)
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToZonedDateTime")}, Args: []ast.Expr{tz, stringLit(disambiguation)}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime.prototype." + method + " is a later slice"}
	}
}

// plainDateTimeWithPlainTimeArg reads the argument of Temporal.PlainDateTime.prototype.withPlainTime
// into the *PlainTime the runtime pairs with the date. No argument, or undefined, defaults the wall
// clock to midnight (a nil *PlainTime). Otherwise the argument is a Temporal.PlainTime, a time
// string (a literal or a string-typed value parsed at run time), or a plain-time-like bag of number
// fields regulated under constrain. Any other shape hands back, since the time would then depend on
// a coercion this slice does not carry.
func (r *Renderer) plainDateTimeWithPlainTimeArg(what string, argNodes []frontend.Node) (ast.Expr, error) {
	if len(argNodes) == 0 {
		return ident("nil"), nil
	}
	if len(argNodes) != 1 {
		return nil, &NotYetLowerable{Reason: what + " takes at most one argument"}
	}
	n := argNodes[0]
	if r.isPlainTime(n) {
		return r.lowerExpr(n)
	}
	if lit, ok := r.stringLiteralValue(n); ok {
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFromString"), Args: []ast.Expr{stringLit(lit)}}, nil
	}
	if r.isString(n) {
		e, err := r.lowerExpr(n)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFromString"), Args: []ast.Expr{goStringOf(e)}}, nil
	}
	if n.Kind() == frontend.NodeObjectLiteralExpression {
		fields, err := r.plainTimeBagFields(what, n)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		args := append(fields[:], stringLit("constrain"))
		return &ast.CallExpr{Fun: sel("value", "PlainTimeFromFields"), Args: args}, nil
	}
	return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainTime, a time string, or a time-like bag is a later slice"}
}

// plainDateTimeToZonedArgs reads the arguments of Temporal.PlainDateTime.prototype.toZonedDateTime
// at compile time into a time-zone Go-string expression and a disambiguation string. The first
// argument is a time-zone string, a literal or a string-typed value; the optional second is an
// options bag carrying a disambiguation that is one of compatible, earlier, later, or reject,
// defaulting to compatible. A time-zone-like object, a missing time zone, a dynamic or out-of-set
// disambiguation, or any other shape hands back, since the zone or the resolution would then
// depend on runtime data this slice does not carry.
func (r *Renderer) plainDateTimeToZonedArgs(what string, argNodes []frontend.Node) (ast.Expr, string, error) {
	if len(argNodes) == 0 {
		return nil, "", &NotYetLowerable{Reason: what + " takes at least one argument"}
	}
	tz, ok, err := r.timeZoneStringArg(argNodes[0])
	if err != nil {
		return nil, "", err
	}
	if !ok {
		return nil, "", &NotYetLowerable{Reason: what + " over a time zone that is not a string is a later slice"}
	}
	disambiguation := "compatible"
	if len(argNodes) > 1 {
		n := argNodes[1]
		if n.Kind() != frontend.NodeObjectLiteralExpression {
			return nil, "", &NotYetLowerable{Reason: what + " with options that are not an object literal is a later slice"}
		}
		for _, member := range r.prog.Children(n) {
			if member.Kind() != frontend.NodeUnknown {
				return nil, "", &NotYetLowerable{Reason: what + " over options with a spread or non-property member is a later slice"}
			}
			kids := r.prog.Children(member)
			if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
				return nil, "", &NotYetLowerable{Reason: what + " over options with a computed or shorthand key is a later slice"}
			}
			switch key := r.prog.Text(kids[0]); key {
			case "disambiguation":
				v, ok := r.stringLiteralValue(kids[1])
				if !ok || !slices.Contains(plainDateTimeDisambiguations[:], v) {
					return nil, "", &NotYetLowerable{Reason: what + " over a dynamic or out-of-set disambiguation is a later slice"}
				}
				disambiguation = v
			default:
				return nil, "", &NotYetLowerable{Reason: what + " over options with the field " + key + " is a later slice"}
			}
		}
	}
	return tz, disambiguation, nil
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
	case "add", "subtract":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainYearMonth.prototype." + method
		dur, err := r.durationArg(what, argNodes[0])
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
		fn := "AddDuration"
		if method == "subtract" {
			fn = "SubtractDuration"
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(fn)}, Args: []ast.Expr{dur, stringLit(overflow)}}, nil
	case "until", "since":
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype." + method + " takes at least one argument"}
		}
		what := "Temporal.PlainYearMonth.prototype." + method
		if !r.isPlainYearMonth(argNodes[0]) {
			return nil, &NotYetLowerable{Reason: what + " over an argument that is not a Temporal.PlainYearMonth (a string or bag to coerce) is a later slice"}
		}
		other, err := r.lowerExpr(argNodes[0])
		if err != nil {
			return nil, err
		}
		largestUnit, err := r.plainYearMonthDifferenceOptions(what, argNodes[1:])
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
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype.with takes at least one argument"}
		}
		what := "Temporal.PlainYearMonth.prototype.with"
		fields, err := r.plainYearMonthBagFields(what, argNodes[0])
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
	case "toPlainDate":
		if len(argNodes) != 1 {
			return nil, &NotYetLowerable{Reason: "Temporal.PlainYearMonth.prototype.toPlainDate takes exactly one argument"}
		}
		what := "Temporal.PlainYearMonth.prototype.toPlainDate"
		day, err := r.yearMonthToPlainDateDay(what, argNodes[0])
		if err != nil {
			return nil, err
		}
		recv, err := r.lowerExpr(recvNode)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("ToPlainDate")}, Args: []ast.Expr{day}}, nil
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
		what := "Temporal.PlainDate.from"
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: what + " requires an argument"}
		}
		if r.isPlainDate(argNodes[0]) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: what + " over a PlainDate with an options argument is a later slice"}
			}
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: what + " over a string with an options argument is a later slice"}
			}
			if !literalCalendarHosted(lit) {
				return nil, &NotYetLowerable{Reason: what + " over a string naming a calendar bento does not host is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if argNodes[0].Kind() == frontend.NodeObjectLiteralExpression {
			return r.plainDateFromBag(what, argNodes[0], argNodes[1:])
		}
		return nil, &NotYetLowerable{Reason: what + " over a dynamic string is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDate." + method + " is a later slice"}
	}
}

// plainDateFromBag lowers Temporal.PlainDate.from over a property bag to a
// value.PlainDateFromFields call: it reads the required year, month, and day fields and the
// optional calendar from the bag, then the overflow option from the second argument. The
// calendar is gated on a hosted id the same way the from-string path is, so an unhosted or
// dynamic calendar hands back rather than dropping to a wrong result.
func (r *Renderer) plainDateFromBag(what string, bag frontend.Node, optionNodes []frontend.Node) (ast.Expr, error) {
	year, month, day, cal, err := r.plainDateFromFields(what, bag)
	if err != nil {
		return nil, err
	}
	overflow, err := r.temporalOverflowOption(what, optionNodes)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "PlainDateFromFields"), Args: []ast.Expr{year, month, day, stringLit(cal), stringLit(overflow)}}, nil
}

// plainDateFromFields reads a PlainDate property bag at compile time into the three required
// numeric field expressions and the calendar id. Unlike the with bag, from requires year,
// month, and day, the record a fresh date needs, so a missing one hands back rather than
// defaulting. The calendar is an optional hosted string literal defaulting to iso8601. A
// spread, a computed or shorthand key, an unknown key (monthCode and the era fields among
// them), a non-number year, month, or day, a dynamic or unhosted calendar, or a repeated
// field hands back, since the field or the calendar would then depend on runtime data or a
// calendar bento cannot represent.
func (r *Renderer) plainDateFromFields(what string, n frontend.Node) (year, month, day ast.Expr, cal string, err error) {
	cal = "iso8601"
	var fields [3]ast.Expr
	var seen [3]bool
	calSeen := false
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		if key == "calendar" {
			if calSeen {
				return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag repeating the field calendar is a later slice"}
			}
			id, ok := r.hostedCalendar(kids[1])
			if !ok {
				return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag whose calendar is dynamic or names a calendar bento does not host is a later slice"}
			}
			cal = id
			calSeen = true
			continue
		}
		idx := -1
		for i, k := range dateFieldKeys {
			if k == key {
				idx = i
				break
			}
		}
		if idx < 0 {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
		}
		if seen[idx] {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
		}
		if !r.isNumber(kids[1]) {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
		}
		val, verr := r.lowerExpr(kids[1])
		if verr != nil {
			return nil, nil, nil, "", verr
		}
		fields[idx] = val
		seen[idx] = true
	}
	for i, k := range dateFieldKeys {
		if !seen[i] {
			return nil, nil, nil, "", &NotYetLowerable{Reason: what + " over a bag missing the field " + k + " (a TypeError at run time) is a later slice"}
		}
	}
	return fields[0], fields[1], fields[2], cal, nil
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

// durationPartialBag reads a Temporal.Duration.prototype.with or Temporal.Duration.from bag at
// compile time into ten present-or-absent field expressions, present as value.Some[float64](v)
// and absent as value.None[float64](), so the runtime knows which fields the bag named. It
// mirrors durationBag but keeps the present-or-absent distinction with and from need: with
// overlays the present fields onto the receiver and from defaults the absent ones to zero. An
// empty bag is NOT handed back, so the runtime raises the TypeError the specification mandates.
// A spread, a computed or shorthand key, an unknown key, a non-number value, or a repeated
// field still hands back, since the field would then depend on runtime data or a coercion this
// slice does not carry.
func (r *Renderer) durationPartialBag(what string, n frontend.Node) ([10]ast.Expr, error) {
	var fields [10]ast.Expr
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return fields, &NotYetLowerable{Reason: what + " over an item that is not an object literal is a later slice"}
	}
	var seen [10]bool
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
		for i, k := range durationUnitKeys {
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
	}
	for i := range fields {
		if fields[i] == nil {
			fields[i] = &ast.CallExpr{Fun: index(sel("value", "None"), ident("float64"))}
		}
	}
	r.requireImport(valuePkg)
	return fields, nil
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

// plainDateTimeRoundUnits is the set of smallest units Temporal.PlainDateTime.prototype.round
// accepts, from the largest to the smallest. It is the time units the wall clock rounds to plus
// day, which rounds the whole date-time to midnight and can carry into the next day.
var plainDateTimeRoundUnits = [7]string{"day", "hour", "minute", "second", "millisecond", "microsecond", "nanosecond"}

// plainDateTimeDisambiguations is the set of disambiguation options
// Temporal.PlainDateTime.prototype.toZonedDateTime accepts when a wall clock is ambiguous or
// falls in a gap: compatible (the default), earlier, later, and reject.
var plainDateTimeDisambiguations = [4]string{"compatible", "earlier", "later", "reject"}

// plainDateTimeRoundOptions reads the options of Temporal.PlainDateTime.prototype.round at compile
// time, the mirror of plainTimeRoundOptions over the day-and-time unit set. The argument is either a
// smallestUnit string literal shorthand or an object literal carrying a required smallestUnit, an
// optional roundingIncrement number expression (default one), and an optional roundingMode string
// literal (default halfExpand). A missing smallestUnit, an unknown key, a non-literal unit or mode,
// or an out-of-set unit or mode hands back. The increment rides through as an expression so the
// runtime validates its range against the unit, including the rule that day accepts only one.
func (r *Renderer) plainDateTimeRoundOptions(what string, argNodes []frontend.Node) (string, ast.Expr, string, error) {
	if len(argNodes) != 1 {
		return "", nil, "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := argNodes[0]
	if lit, ok := r.stringLiteralValue(n); ok {
		if !slices.Contains(plainDateTimeRoundUnits[:], lit) {
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
			if !slices.Contains(plainDateTimeRoundUnits[:], lit) {
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

// plainDateTimeDifferenceUnits maps the largestUnit forms until and since accept, singular and
// plural, to the singular form the runtime takes. It spans the calendar units and the time units,
// since a date-time difference can be counted in either. "auto" resolves to "day", the default.
var plainDateTimeDifferenceUnits = map[string]string{
	"auto": "day", "year": "year", "years": "year", "month": "month", "months": "month",
	"week": "week", "weeks": "week", "day": "day", "days": "day",
	"hour": "hour", "hours": "hour", "minute": "minute", "minutes": "minute",
	"second": "second", "seconds": "second", "millisecond": "millisecond", "milliseconds": "millisecond",
	"microsecond": "microsecond", "microseconds": "microsecond", "nanosecond": "nanosecond", "nanoseconds": "nanosecond",
}

// plainDateTimeDifferenceOptions reads the options of PlainDateTime.prototype.until and since at
// compile time and returns the largestUnit, defaulting to day. It accepts largestUnit as a string
// literal in the calendar-or-time unit set. A smallestUnit, roundingIncrement, or roundingMode would
// round the duration, which needs the round-with-relativeTo machinery a later slice carries, so any
// of those, a non-literal or out-of-set largestUnit, an unknown key, or a spread or shorthand member
// hands back rather than emitting a wrong or partial result.
func (r *Renderer) plainDateTimeDifferenceOptions(what string, argNodes []frontend.Node) (string, error) {
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
			unit, ok := plainDateTimeDifferenceUnits[lit]
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

// zonedDateTimeDifferenceOptions reads the options of ZonedDateTime.prototype.until and since at
// compile time and returns the largestUnit. It shares the unit vocabulary of the plain date-time
// difference but defaults to hour, not day: a ZonedDateTime difference counts in exact-time units by
// default and only spans days when a calendar largestUnit is asked for, so "auto" and the absent
// option both resolve to hour here. A smallestUnit, roundingIncrement, or roundingMode would round
// the duration, which needs the round-with-relativeTo machinery a later slice carries, so any of
// those, a non-literal or out-of-set largestUnit, an unknown key, or a spread or shorthand member
// hands back rather than emitting a wrong or partial result.
func (r *Renderer) zonedDateTimeDifferenceOptions(what string, argNodes []frontend.Node) (string, error) {
	largestUnit := "hour"
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
			if lit == "auto" {
				largestUnit = "hour"
				continue
			}
			unit, ok := plainDateTimeDifferenceUnits[lit]
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
		what := "Temporal.PlainDateTime.from"
		if len(argNodes) == 0 {
			return nil, &NotYetLowerable{Reason: what + " requires an argument"}
		}
		if r.isPlainDateTime(argNodes[0]) {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: what + " over a PlainDateTime with an options argument is a later slice"}
			}
			arg, err := r.lowerExpr(argNodes[0])
			if err != nil {
				return nil, err
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFrom"), Args: []ast.Expr{arg}}, nil
		}
		if lit, ok := r.stringLiteralValue(argNodes[0]); ok {
			if len(argNodes) != 1 {
				return nil, &NotYetLowerable{Reason: what + " over a string with an options argument is a later slice"}
			}
			if !literalCalendarHosted(lit) {
				return nil, &NotYetLowerable{Reason: what + " over a string naming a calendar bento does not host is a later slice"}
			}
			r.requireImport(valuePkg)
			return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFromString"), Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(lit)}}}, nil
		}
		if argNodes[0].Kind() == frontend.NodeObjectLiteralExpression {
			return r.plainDateTimeFromBag(what, argNodes[0], argNodes[1:])
		}
		return nil, &NotYetLowerable{Reason: what + " over a dynamic string is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "Temporal.PlainDateTime." + method + " is a later slice"}
	}
}

// plainDateTimeFromBag lowers Temporal.PlainDateTime.from over a property bag to a
// value.PlainDateTimeFromFields call: it reads the required year, month, and day fields, the
// optional time fields, and the optional calendar from the bag, then the overflow option from the
// second argument. The calendar is gated on a hosted id the same way the from-string path is, so an
// unhosted or dynamic calendar hands back rather than dropping to a wrong result.
func (r *Renderer) plainDateTimeFromBag(what string, bag frontend.Node, optionNodes []frontend.Node) (ast.Expr, error) {
	year, month, day, timeFields, cal, err := r.plainDateTimeFromFields(what, bag)
	if err != nil {
		return nil, err
	}
	overflow, err := r.temporalOverflowOption(what, optionNodes)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	args := []ast.Expr{year, month, day}
	args = append(args, timeFields[:]...)
	args = append(args, stringLit(cal), stringLit(overflow))
	return &ast.CallExpr{Fun: sel("value", "PlainDateTimeFromFields"), Args: args}, nil
}

// plainDateTimeFromFields reads a PlainDateTime property bag at compile time into the three
// required date field expressions, the six present-or-absent time field expressions, and the
// calendar id. Like the PlainDate from bag, the year, month, and day are required, the record a
// fresh date needs, so a missing one hands back; the time fields default to absent, so an omitted
// one falls to the midnight the runtime carries. The calendar is an optional hosted string literal
// defaulting to iso8601. A spread, a computed or shorthand key, an unknown key (monthCode and the
// era fields among them), a non-number value, a dynamic or unhosted calendar, or a repeated field
// hands back, since the field or the calendar would then depend on runtime data or a calendar bento
// cannot represent.
func (r *Renderer) plainDateTimeFromFields(what string, n frontend.Node) (year, month, day ast.Expr, timeFields [6]ast.Expr, cal string, err error) {
	cal = "iso8601"
	var date [3]ast.Expr
	var dateSeen [3]bool
	var timeSeen [6]bool
	calSeen := false
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		if key == "calendar" {
			if calSeen {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag repeating the field calendar is a later slice"}
			}
			id, ok := r.hostedCalendar(kids[1])
			if !ok {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag whose calendar is dynamic or names a calendar bento does not host is a later slice"}
			}
			cal = id
			calSeen = true
			continue
		}
		if di := slices.Index(dateFieldKeys[:], key); di >= 0 {
			if dateSeen[di] {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
			}
			if !r.isNumber(kids[1]) {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
			}
			val, verr := r.lowerExpr(kids[1])
			if verr != nil {
				return nil, nil, nil, timeFields, "", verr
			}
			date[di] = val
			dateSeen[di] = true
			continue
		}
		if ti := slices.Index(timeFieldKeys[:], key); ti >= 0 {
			if timeSeen[ti] {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
			}
			if !r.isNumber(kids[1]) {
				return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
			}
			val, verr := r.lowerExpr(kids[1])
			if verr != nil {
				return nil, nil, nil, timeFields, "", verr
			}
			timeFields[ti] = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("float64")), Args: []ast.Expr{val}}
			timeSeen[ti] = true
			continue
		}
		return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
	}
	for i, k := range dateFieldKeys {
		if !dateSeen[i] {
			return nil, nil, nil, timeFields, "", &NotYetLowerable{Reason: what + " over a bag missing the field " + k + " (a TypeError at run time) is a later slice"}
		}
	}
	for i := range timeFields {
		if timeFields[i] == nil {
			timeFields[i] = &ast.CallExpr{Fun: index(sel("value", "None"), ident("float64"))}
		}
	}
	return date[0], date[1], date[2], timeFields, cal, nil
}

// zonedDateTimeOffsetOptions is the set of offset options a ZonedDateTime.from bag weighs a
// supplied offset field under.
var zonedDateTimeOffsetOptions = [4]string{"use", "ignore", "prefer", "reject"}

// zonedDateTimeFromBag lowers Temporal.ZonedDateTime.from over a property bag. The bag carries the
// required year, month, and day, the optional time fields defaulting to midnight, a required
// timeZone naming the zone, and an optional offset, a UTC offset string weighed against the zone
// under the offset option. The options bag supplies overflow (constrain or reject, default
// constrain), disambiguation (compatible, earlier, later, or reject, default compatible), and
// offset (use, ignore, prefer, or reject, default reject), each an optional hosted literal. The bag
// is ISO-calendar gated: a calendar field naming another calendar hands back, since bento's
// ZonedDateTime hosts only the ISO calendar. A spread, a computed or shorthand key, an unknown key,
// a non-number date or time field, a non-string timeZone or offset, a dynamic or out-of-set option,
// a missing timeZone or date field, or a repeated field hands back, since the value would then
// depend on runtime data or a shape this slice does not carry.
func (r *Renderer) zonedDateTimeFromBag(what string, bag frontend.Node, optionNodes []frontend.Node) (ast.Expr, error) {
	var date [3]ast.Expr
	var dateSeen [3]bool
	var timeFields [6]ast.Expr
	var timeSeen [6]bool
	var tz ast.Expr
	offsetSeen := false
	offset := ast.Expr(&ast.CallExpr{Fun: index(sel("value", "None"), ident("string"))})
	for _, member := range r.prog.Children(bag) {
		if member.Kind() != frontend.NodeUnknown {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a spread or non-property member is a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return nil, &NotYetLowerable{Reason: what + " over a bag with a computed or shorthand key is a later slice"}
		}
		key := r.prog.Text(kids[0])
		if key == "calendar" {
			id, ok := r.hostedCalendar(kids[1])
			if !ok {
				return nil, &NotYetLowerable{Reason: what + " over a bag whose calendar is dynamic or names a calendar bento does not host is a later slice"}
			}
			if !strings.EqualFold(id, "iso8601") {
				return nil, &NotYetLowerable{Reason: what + " over a bag naming the non-ISO calendar " + id + " is a later slice"}
			}
			continue
		}
		if key == "timeZone" {
			if tz != nil {
				return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field timeZone is a later slice"}
			}
			z, ok, err := r.timeZoneStringArg(kids[1])
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, &NotYetLowerable{Reason: what + " over a bag whose timeZone is not a string is a later slice"}
			}
			tz = z
			continue
		}
		if key == "offset" {
			if offsetSeen {
				return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field offset is a later slice"}
			}
			o, ok, err := r.timeZoneStringArg(kids[1])
			if err != nil {
				return nil, err
			}
			if !ok {
				return nil, &NotYetLowerable{Reason: what + " over a bag whose offset is not a string is a later slice"}
			}
			offset = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("string")), Args: []ast.Expr{o}}
			offsetSeen = true
			continue
		}
		if di := slices.Index(dateFieldKeys[:], key); di >= 0 {
			if dateSeen[di] {
				return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
			}
			if !r.isNumber(kids[1]) {
				return nil, &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
			}
			val, err := r.lowerExpr(kids[1])
			if err != nil {
				return nil, err
			}
			date[di] = val
			dateSeen[di] = true
			continue
		}
		if ti := slices.Index(timeFieldKeys[:], key); ti >= 0 {
			if timeSeen[ti] {
				return nil, &NotYetLowerable{Reason: what + " over a bag repeating the field " + key + " is a later slice"}
			}
			if !r.isNumber(kids[1]) {
				return nil, &NotYetLowerable{Reason: what + " over a bag whose " + key + " is not a number is a later slice"}
			}
			val, err := r.lowerExpr(kids[1])
			if err != nil {
				return nil, err
			}
			timeFields[ti] = &ast.CallExpr{Fun: index(sel("value", "Some"), ident("float64")), Args: []ast.Expr{val}}
			timeSeen[ti] = true
			continue
		}
		return nil, &NotYetLowerable{Reason: what + " over a bag with the field " + key + " is a later slice"}
	}
	for i, k := range dateFieldKeys {
		if !dateSeen[i] {
			return nil, &NotYetLowerable{Reason: what + " over a bag missing the field " + k + " (a TypeError at run time) is a later slice"}
		}
	}
	if tz == nil {
		return nil, &NotYetLowerable{Reason: what + " over a bag with no timeZone (a TypeError at run time) is a later slice"}
	}
	for i := range timeFields {
		if timeFields[i] == nil {
			timeFields[i] = &ast.CallExpr{Fun: index(sel("value", "None"), ident("float64"))}
		}
	}
	overflow, disambiguation, offsetOption, err := r.zonedDateTimeFromOptions(what, optionNodes)
	if err != nil {
		return nil, err
	}
	r.requireImport(valuePkg)
	args := []ast.Expr{date[0], date[1], date[2]}
	args = append(args, timeFields[:]...)
	args = append(args, tz, offset, stringLit(overflow), stringLit(disambiguation), stringLit(offsetOption))
	return &ast.CallExpr{Fun: sel("value", "ZonedDateTimeFromFields"), Args: args}, nil
}

// zonedDateTimeFromOptions reads the options bag of Temporal.ZonedDateTime.from into the overflow,
// disambiguation, and offset options. Each is an optional hosted string literal from its own set,
// defaulting to constrain, compatible, and reject the way the specification's ToTemporalZonedDateTime
// does. A missing bag takes the three defaults; more than one options argument, options that are not
// an object literal, a spread or non-property member, a computed or shorthand key, an unknown option,
// or a dynamic or out-of-set value hands back, since the option would then depend on runtime data.
func (r *Renderer) zonedDateTimeFromOptions(what string, optionNodes []frontend.Node) (overflow, disambiguation, offsetOption string, err error) {
	overflow, disambiguation, offsetOption = "constrain", "compatible", "reject"
	if len(optionNodes) == 0 {
		return overflow, disambiguation, offsetOption, nil
	}
	if len(optionNodes) != 1 {
		return "", "", "", &NotYetLowerable{Reason: what + " with more than an options argument is a later slice"}
	}
	n := optionNodes[0]
	if n.Kind() != frontend.NodeObjectLiteralExpression {
		return "", "", "", &NotYetLowerable{Reason: what + " options that are not an object literal are a later slice"}
	}
	for _, member := range r.prog.Children(n) {
		if member.Kind() != frontend.NodeUnknown {
			return "", "", "", &NotYetLowerable{Reason: what + " options with a spread or non-property member are a later slice"}
		}
		kids := r.prog.Children(member)
		if len(kids) != 2 || kids[0].Kind() != frontend.NodeIdentifier {
			return "", "", "", &NotYetLowerable{Reason: what + " options with a computed or shorthand key are a later slice"}
		}
		key := r.prog.Text(kids[0])
		lit, ok := r.stringLiteralValue(kids[1])
		switch key {
		case "overflow":
			if !ok || (lit != "constrain" && lit != "reject") {
				return "", "", "", &NotYetLowerable{Reason: what + " with a dynamic or out-of-set overflow option is a later slice"}
			}
			overflow = lit
		case "disambiguation":
			if !ok || !slices.Contains(plainDateTimeDisambiguations[:], lit) {
				return "", "", "", &NotYetLowerable{Reason: what + " with a dynamic or out-of-set disambiguation option is a later slice"}
			}
			disambiguation = lit
		case "offset":
			if !ok || !slices.Contains(zonedDateTimeOffsetOptions[:], lit) {
				return "", "", "", &NotYetLowerable{Reason: what + " with a dynamic or out-of-set offset option is a later slice"}
			}
			offsetOption = lit
		default:
			return "", "", "", &NotYetLowerable{Reason: what + " with the option " + key + " is a later slice"}
		}
	}
	return overflow, disambiguation, offsetOption, nil
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
