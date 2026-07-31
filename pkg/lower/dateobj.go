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
		components, err := r.dateComponentArgs("new Date", valueArgs)
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: sel("value", "NewDateFromComponents"), Args: components}, nil
	}
}

// dateComponentArgs lowers a calendar reading's arguments, the year, month and the fields
// below them that the component constructor, Date.UTC and every setter take. They all have
// to be numbers: a component that needs coercion hands back rather than being guessed at,
// since a component silently read as zero is the wrong instant reported as a right one.
func (r *Renderer) dateComponentArgs(what string, args []frontend.Node) ([]ast.Expr, error) {
	if len(args) > 7 {
		return nil, &NotYetLowerable{Reason: what + " takes at most seven components"}
	}
	out := make([]ast.Expr, 0, len(args))
	for _, a := range args {
		if !r.isNumber(a) {
			return nil, &NotYetLowerable{Reason: what + " with a component that is not a number needs coercion, a later slice"}
		}
		lowered, err := r.lowerExpr(a)
		if err != nil {
			return nil, err
		}
		out = append(out, lowered)
	}
	return out, nil
}

// dateStaticCall lowers a static call on the global Date. Each of the three gives a time
// value as a Number with no Date built at all: now() reads the clock, parse() reads a
// string, and UTC() reads a calendar.
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
	case "UTC":
		components, err := r.dateComponentArgs("Date.UTC", args)
		if err != nil {
			return nil, err
		}
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "DateUTC"), Args: components}, nil
	default:
		return nil, &NotYetLowerable{Reason: "Date." + method + " is a later slice"}
	}
}

// dateArith lowers an operator that reads a date as its time value: the arithmetic
// operators, which ToNumber both sides, and the relational ones, which run the
// Abstract Relational Comparison and get a number from a date because that comparison
// asks ToPrimitive with the number hint. So d2 - d1 is a duration in milliseconds and
// d1 < d2 orders two instants, both on the float64 the runtime's ValueOf answers.
//
// It fires only when a date is on one side and the other side is a date or a
// number-coercible primitive, which keeps it off the pairs the number, string and
// boolean paths already own. + is deliberately not here: a date is the one built-in
// whose default hint is string, so d + x concatenates and goes through
// stringifyOperand instead.
func (r *Renderer) dateArith(opText string, left, right frontend.Node) (ast.Expr, bool, error) {
	_, isRelational := relationalToken(opText)
	if !isToNumberArithOp(opText) && !isRelational {
		return nil, false, nil
	}
	if !r.isDate(left) && !r.isDate(right) {
		return nil, false, nil
	}
	if !r.isDateArithOperand(left) || !r.isDateArithOperand(right) {
		return nil, false, nil
	}
	l, err := r.unaryOperandToNumber(left)
	if err != nil {
		return nil, false, err
	}
	rr, err := r.unaryOperandToNumber(right)
	if err != nil {
		return nil, false, err
	}
	expr, ok := r.numericOpFromFloats(opText, l, rr)
	return expr, ok, nil
}

// isDateArithOperand reports whether n can sit opposite a date in an operator that
// reads both sides as numbers: another date, or one of the primitives ToNumber
// converts at compile-known cost. Anything else, a dynamic value or an object with a
// valueOf of its own, needs the boxing coercion and is left to its own path.
func (r *Renderer) isDateArithOperand(n frontend.Node) bool {
	return r.isDate(n) || r.isNumberCoercible(n)
}

// dateToJSONCall reports whether n is a no-argument date.toJSON() call, the shape
// dateMethodCall lowers to value.DateToJSON. That runtime call answers a value.Value
// because null is one of its two answers, while the checker types the method as
// returning a string, so isDynamic recognizes the call by shape and keeps the box on
// the dynamic path. It must stay in lockstep with the emit guard in dateMethodCall:
// same receiver test, same argument count.
func (r *Renderer) dateToJSONCall(n frontend.Node) bool {
	if n.Kind() != frontend.NodeCallExpression {
		return false
	}
	kids := r.prog.Children(n)
	if len(kids) != 1 || kids[0].Kind() != frontend.NodePropertyAccessExpression {
		return false
	}
	parts := r.prog.Children(kids[0])
	if len(parts) != 2 || r.prog.Text(parts[1]) != "toJSON" {
		return false
	}
	return r.isDate(parts[0])
}

// dateGetters maps each no-argument Date read to the runtime method that answers it.
// The calendar getters come in a local pair and a UTC pair for every component, which is
// most of the surface, and each is a straight rename, so the dispatch is a table rather
// than a switch with sixteen near-identical arms.
var dateGetters = map[string]string{
	"getTime":            "GetTime",
	"valueOf":            "ValueOf",
	"toISOString":        "ToISOString",
	"toString":           "ToString",
	"toDateString":       "ToDateString",
	"toTimeString":       "ToTimeString",
	"toUTCString":        "ToUTCString",
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

// dateSetters maps each calendar write to the runtime method that performs it. Every one
// is variadic in the same way — it takes its own field and, optionally, the fields below
// it — so like the getters they are a table rather than fourteen near-identical arms.
var dateSetters = map[string]string{
	"setFullYear":        "SetFullYear",
	"setMonth":           "SetMonth",
	"setDate":            "SetDate",
	"setHours":           "SetHours",
	"setMinutes":         "SetMinutes",
	"setSeconds":         "SetSeconds",
	"setMilliseconds":    "SetMilliseconds",
	"setUTCFullYear":     "SetUTCFullYear",
	"setUTCMonth":        "SetUTCMonth",
	"setUTCDate":         "SetUTCDate",
	"setUTCHours":        "SetUTCHours",
	"setUTCMinutes":      "SetUTCMinutes",
	"setUTCSeconds":      "SetUTCSeconds",
	"setUTCMilliseconds": "SetUTCMilliseconds",
}

// dateMethodCall lowers a method call on a Date receiver: the reads that give the time
// value, the ISO format, the calendar getters, and the calendar setters. The remaining
// formats have rules of their own, so they hand back naming the slice that brings them.
func (r *Renderer) dateMethodCall(recvNode frontend.Node, method string, argNodes []frontend.Node) (ast.Expr, error) {
	args := r.namedArgs(argNodes)
	_, isGetter := dateGetters[method]
	_, isSetter := dateSetters[method]
	if !isGetter && !isSetter && method != "setTime" && method != "toJSON" {
		// The receiver is left unlowered on this path so that a hand-back does not leave
		// the renderer carrying an import for an expression that was never emitted.
		return nil, &NotYetLowerable{Reason: "the Date method ." + method + " is a later slice"}
	}
	recv, err := r.lowerExpr(recvNode)
	if err != nil {
		return nil, err
	}
	if goName, ok := dateGetters[method]; ok {
		if len(args) != 0 {
			return nil, &NotYetLowerable{Reason: "date." + method + " takes no argument"}
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}}, nil
	}
	if goName, ok := dateSetters[method]; ok {
		components, err := r.dateComponentArgs("date."+method, args)
		if err != nil {
			return nil, err
		}
		if len(components) == 0 {
			return nil, &NotYetLowerable{Reason: "date." + method + " takes at least one component"}
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident(goName)}, Args: components}, nil
	}
	if method == "toJSON" {
		if len(args) != 0 {
			return nil, &NotYetLowerable{Reason: "date.toJSON takes no argument"}
		}
		// toJSON answers null for a date with no representable instant, so the runtime
		// hands back a value.Value rather than a BStr even though the checker types the
		// method as returning a string. dateToJSONCall marks the call node dynamic, which
		// is what keeps the truthful value on the dynamic path: it flows into an any slot
		// and hands the build back in a string slot rather than shipping "" for null.
		r.requireImport(valuePkg)
		return &ast.CallExpr{Fun: sel("value", "DateToJSON"), Args: []ast.Expr{recv}}, nil
	}
	if method == "setTime" {
		if len(args) != 1 || !r.isNumber(args[0]) {
			return nil, &NotYetLowerable{Reason: "date.setTime of a value that is not a number needs coercion, a later slice"}
		}
		ms, err := r.lowerExpr(args[0])
		if err != nil {
			return nil, err
		}
		return &ast.CallExpr{Fun: &ast.SelectorExpr{X: recv, Sel: ident("SetTime")}, Args: []ast.Expr{ms}}, nil
	}
	return nil, &NotYetLowerable{Reason: "the Date method ." + method + " is a later slice"}
}
