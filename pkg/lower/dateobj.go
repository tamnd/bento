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

// newDate lowers the constructor. The bare form reads the clock, a single number is a
// time value clipped the way TimeClip specifies, and a single string is parsed. The
// component form has to build a date from a local calendar reading, which is its own
// slice and says so.
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
	case len(valueArgs) == 1 && r.isString(valueArgs[0]):
		s, err := r.lowerExpr(valueArgs[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "NewDateFromString"), Args: []ast.Expr{s}}, nil
	case len(valueArgs) == 1 && r.isDate(valueArgs[0]):
		d, err := r.lowerExpr(valueArgs[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{
			Fun: sel("value", "NewDateFromMillis"),
			Args: []ast.Expr{&ast.CallExpr{
				Fun: &ast.SelectorExpr{X: d, Sel: ident("GetTime")},
			}},
		}, nil
	case len(valueArgs) == 1:
		return nil, &NotYetLowerable{Reason: "new Date from a value that is not a number, a string, or a date needs coercion, a later slice"}
	default:
		return nil, &NotYetLowerable{Reason: "new Date from year, month, and day components is a later slice"}
	}
}

// dateStaticCall lowers a static call on the global Date. now() reads the clock and
// parse() reads a string, both giving a time value as a Number with no Date built at all.
// Date.UTC builds a time value out of components, which is the same calendar work the
// component constructor needs, so it waits for that slice.
func (r *Renderer) dateStaticCall(method string, argNodes []frontend.Node) (ast.Expr, error) {
	args := r.namedArgs(argNodes)
	switch method {
	case "now":
		if len(args) != 0 {
			return nil, &NotYetLowerable{Reason: "Date.now takes no argument"}
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "DateNow")}, nil
	case "parse":
		if len(args) != 1 || !r.isString(args[0]) {
			return nil, &NotYetLowerable{Reason: "Date.parse of a value that is not a string needs coercion, a later slice"}
		}
		s, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "ParseDate"), Args: []ast.Expr{s}}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Date." + method + " is a later slice"}
	}
}

// dateGetters maps each no-argument Date read to the runtime method that answers it.
// The calendar getters come in a local pair and a UTC pair for every component, which is
// most of the surface, and each is a straight rename, so the dispatch is a table rather
// than a switch with sixteen near-identical arms.
var dateGetters = map[string]string{
	"getTime":            "GetTime",
	"valueOf":            "ValueOf",
	"toISOString":        "ToISOString",
	"getTimezoneOffset":  "GetTimezoneOffset",
	"getFullYear":        "GetFullYear",
	"getMonth":           "GetMonth",
	"getDate":            "GetDate",
	"getDay":             "GetDay",
	"getHours":           "GetHours",
	"getMinutes":         "GetMinutes",
	"getSeconds":         "GetSeconds",
	"getMilliseconds":    "GetMilliseconds",
	"getUTCFullYear":     "GetUTCFullYear",
	"getUTCMonth":        "GetUTCMonth",
	"getUTCDate":         "GetUTCDate",
	"getUTCDay":          "GetUTCDay",
	"getUTCHours":        "GetUTCHours",
	"getUTCMinutes":      "GetUTCMinutes",
	"getUTCSeconds":      "GetUTCSeconds",
	"getUTCMilliseconds": "GetUTCMilliseconds",
}

// dateMethodCall lowers a method call on a Date receiver: the two reads that give the
// time value, the ISO format, and the calendar getters, each of which takes no argument
// and gives a Number. The setters mutate the time value and the other formats have their
// own rules, so both hand back naming the slice that brings them.
func (r *Renderer) dateMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	goName, ok := dateGetters[method]
	if !ok {
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
