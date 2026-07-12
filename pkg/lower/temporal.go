package lower

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// This file lowers Temporal.PlainDate, the first cut of the Temporal area (10_advanced
// group 6). A PlainDate is a calendar date with no time and no zone; this slice hosts
// only the ISO 8601 calendar, the default every PlainDate carries. Construction
// (new Temporal.PlainDate(y, m, d)), the static Temporal.PlainDate.from over a PlainDate
// and Temporal.PlainDate.compare, the clean ISO field getters, and the equals, toString,
// and toJSON methods lower to the value.PlainDate runtime type. Everything else, the
// arithmetic (add, subtract, until, since), the with and withCalendar reshaping, the
// cross-type conversions, from over a string or a property bag, and the calendar-dependent
// getters the checker types number | undefined (era, eraYear, weekOfYear, yearOfWeek),
// hands back with a named reason so the compiler reports the exact ceiling.
//
// A PlainDate follows the host-type model RegExp and the collections use: it is a bare
// *value.PlainDate in the generated Go, recognized by its declaring symbol name rather
// than a dedicated type flag. The Temporal namespace is a two-level access
// (Temporal.PlainDate.compare), which no other built-in uses, so the call and new paths
// carry a small amount of namespace-chain recognition this file drives.

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

// temporalStaticCall lowers a static call on a Temporal namespace member,
// Temporal.PlainDate.compare(a, b) or Temporal.PlainDate.from(x). compare lowers to
// value.PlainDateCompare over the two dates; from lowers to value.PlainDateFrom for a
// PlainDate argument (the copy the specification makes) and hands back for a string or
// a property bag, which need parsing this slice does not carry. A Temporal type other
// than PlainDate, or a static this slice does not host, hands back with a named reason.
func (r *Renderer) temporalStaticCall(typeName, method string, argNodes []frontend.Node) (ast.Expr, error) {
	if typeName != "PlainDate" {
		return nil, &NotYetLowerable{Reason: "Temporal." + typeName + " is a later slice"}
	}
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

// newTemporal lowers new Temporal.<Type>(...). Only Temporal.PlainDate is hosted, over
// its three number arguments (isoYear, isoMonth, isoDay); a fourth calendar argument
// selects a non-ISO calendar, which this slice does not carry, so it hands back. Each
// argument must lower as a number, so a non-number component hands back rather than
// coerce; the runtime constructor runs ToIntegerWithTruncation and RejectISODate, so an
// out-of-range date throws a RangeError at run time the way the specification requires.
func (r *Renderer) newTemporal(typeName string, argNodes []frontend.Node) (ast.Expr, error) {
	if typeName != "PlainDate" {
		return nil, &NotYetLowerable{Reason: "new Temporal." + typeName + " is a later slice"}
	}
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
