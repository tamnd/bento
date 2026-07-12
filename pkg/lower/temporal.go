package lower

import (
	"go/ast"
	"go/token"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers the Temporal area (10_advanced group 6), one type per cut. Two are
// hosted so far: PlainDate, a calendar date with no time and no zone over the ISO 8601
// calendar, and PlainTime, a wall-clock time with no date and no zone. For each,
// construction, the static from over the same type and compare, the clean field getters,
// and the equals, toString, and toJSON methods lower to the matching value runtime type.
// Everything else, the arithmetic, the reshaping, the cross-type conversions, from over
// a string or a property bag, and the getters the checker types number | undefined,
// hands back with a named reason so the compiler reports the exact ceiling.
//
// Each Temporal type follows the host-type model RegExp and the collections use: it is a
// bare pointer in the generated Go (*value.PlainDate, *value.PlainTime), recognized by
// its declaring symbol name rather than a dedicated type flag. The Temporal namespace is
// a two-level access (Temporal.PlainDate.compare), which no other built-in uses, so the
// call and new paths carry a small amount of namespace-chain recognition this file drives.

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

// plainDateAccessor maps a PlainDate field getter to the value.PlainDate method that
// reads it, or reports ok=false for a name this slice does not host. The clean ISO
// getters (year, month, day, and the derived weekday, day-of-year, leap flag, fixed
// counts, month code, and calendar id) map to a method; the calendar-dependent getters
// the checker types number | undefined (era, eraYear, weekOfYear, yearOfWeek) are absent
// so they hand back rather than lower to a getter that cannot answer the undefined case.
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

// temporalStaticCall lowers a static call on a Temporal namespace member, routing on the
// type name to the per-type static handler. A Temporal type this file does not host yet
// hands back with a named reason.
func (r *Renderer) temporalStaticCall(typeName, method string, argNodes []frontend.Node) (ast.Expr, error) {
	switch typeName {
	case "PlainDate":
		return r.plainDateStaticCall(method, argNodes)
	case "PlainTime":
		return r.plainTimeStaticCall(method, argNodes)
	default:
		return nil, &NotYetLowerable{Reason: "Temporal." + typeName + " is a later slice"}
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

// newTemporal lowers new Temporal.<Type>(...), routing on the type name to the per-type
// constructor handler. A Temporal type this file does not host yet hands back.
func (r *Renderer) newTemporal(typeName string, argNodes []frontend.Node) (ast.Expr, error) {
	switch typeName {
	case "PlainDate":
		return r.newPlainDate(argNodes)
	case "PlainTime":
		return r.newPlainTime(argNodes)
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
