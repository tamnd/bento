package lower

// This file lowers Date, the clock built-in. Every spelling of it handed the unit back
// before this, including Date.now(), because the standard library types the constructor
// with several call and construct signatures at once and the lowerer stopped at the type
// rather than at the construction. Claiming the name ahead of that is what unblocks it.
//
// This slice covers the time value: construction from now or from a number, the two
// reads that give the number back, and the ISO serialization. The calendar getters, the
// string constructor, and the component constructor are each their own slice, and each
// says so with its own reason.

import (
	"go/ast"

	"github.com/tamnd/bento/pkg/frontend"
)

// isDateType reports whether an object type is a Date. getTime and getTimezoneOffset
// together are the fingerprint: getTime alone would also match anything carrying a
// timer-shaped method, while getTimezoneOffset belongs to Date and nothing else in the
// standard library.
func (r *Renderer) isDateType(t frontend.Type) bool {
	var hasGetTime, hasOffset bool
	for _, p := range r.prog.Properties(t) {
		switch p.Name {
		case "getTime":
			hasGetTime = true
		case "getTimezoneOffset":
			hasOffset = true
		}
	}
	return hasGetTime && hasOffset
}

// isDate reports whether the checker types a node as a Date, the receiver test the date
// lowerings share.
func (r *Renderer) isDate(n frontend.Node) bool {
	t := r.prog.TypeAt(n)
	if t.Flags&frontend.TypeObject == 0 {
		return false
	}
	return r.isDateType(t)
}

// newDate lowers new Date() and new Date(ms). The bare form reads the clock; a single
// number is a time value, clipped by the runtime the way TimeClip specifies. The string
// form has to parse a date, and the component form has to build one from year, month,
// and day, so each is its own slice and says so.
func (r *Renderer) newDate(args []frontend.Node) (ast.Expr, error) {
	valueArgs := r.namedArgs(args)
	r.requireImport(valuePkg)
	switch {
	case len(valueArgs) == 0:
		return &ast.CallExpr{Fun: sel("value", "NewDate")}, nil
	case len(valueArgs) == 1 && r.isNumber(valueArgs[0]):
		ms, err := r.lowerExpr(valueArgs[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "NewDateFromMillis"), Args: []ast.Expr{ms}}, nil
	case len(valueArgs) == 1:
		return nil, &NotYetLowerable{Reason: "new Date from a string or another date is a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "new Date from year, month, and day components is a later slice"}
	}
}

// dateStaticCall lowers a static call on the global Date. Date.now() is the whole of the
// covered surface: it gives the current time value as a Number, with no Date built at
// all. Date.parse and Date.UTC each need the parsing and component work a later slice
// brings.
func (r *Renderer) dateStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	if method != "now" {
		return nil, &NotYetLowerable{Reason: "Date." + method + " is a later slice"}
	}
	if len(r.namedArgs(argNodes)) != 0 {
		return nil, &NotYetLowerable{Reason: "Date.now takes no argument"}
	}
	r.requireImport(valuePkg)
	return &ast.CallExpr{Fun: sel("value", "DateNow")}, nil
}

// dateMethodCall lowers a method call on a Date receiver. getTime and valueOf both give
// the time value, and toISOString serializes it; the calendar getters read components
// this slice does not derive, so they hand back naming the slice that brings them.
func (r *Renderer) dateMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	var goName string
	switch method {
	case "getTime":
		goName = "GetTime"
	case "valueOf":
		goName = "ValueOf"
	case "toISOString":
		goName = "ToISOString"
	default:
		return nil, &NotYetLowerable{Reason: "the Date method ." + method + " is a later slice"}
	}
	if len(r.namedArgs(argNodes)) != 0 {
		return nil, &NotYetLowerable{Reason: "date." + method + " takes no argument"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}}, nil
}
