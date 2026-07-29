package value

import "strings"

// This file is the argument-checking half of Node's built-in modules, the part
// that decides what a built-in does with an argument it cannot use. Node does not
// coerce: path.dirname(5) does not answer the dirname of "5", it throws
// ERR_INVALID_ARG_TYPE. That is a deliberate choice on Node's side and programs
// depend on it, so a built-in bento implements has to make the same choice or it
// will answer where Node refuses.
//
// The message format is Node's own, down to how each kind of value is described,
// because it is the part a developer reads when the throw reaches them. The code
// matters more: a program branches on err.code, and every one of these errors is
// ERR_INVALID_ARG_TYPE.
//
// These live here rather than in the path module because the check is the same for
// every built-in. fs, url, and the rest validate their arguments the same way and
// against the same message, so they share this rather than each growing a copy that
// drifts.

// requireString returns the Go string an argument holds, or throws Node's
// ERR_INVALID_ARG_TYPE naming the parameter. Only an actual string passes: a String
// object, a number, and null are all refused, which is what Node's validateString
// does.
func requireString(v Value, param string) string {
	if v.Kind() != KindString {
		Throw(invalidArgType(param, "string", v))
	}
	return v.AsString().ToGoString()
}

// requireObject returns the argument when it is one Node would accept as an object
// and throws otherwise. Node's validateObject refuses null and refuses a function,
// because neither has the shape the caller is going to read fields off, and it
// accepts an array only where the caller opted in, which none of these do.
func requireObject(v Value, param string) Value {
	if v.Kind() != KindObject && v.Kind() != KindArray {
		Throw(invalidArgType(param, "object", v))
	}
	return v
}

// invalidArgType builds the ERR_INVALID_ARG_TYPE a built-in throws for an argument
// of the wrong type. The message names the parameter, the type that was wanted, and
// what arrived, in Node's wording.
func invalidArgType(param, expected string, got Value) *Error {
	msg := `The "` + param + `" argument must be of type ` + expected + ". " + receivedDescription(got)
	return NewNodeError("TypeError", "ERR_INVALID_ARG_TYPE", FromGoString(msg))
}

// receivedDescription renders the "Received ..." tail of an ERR_INVALID_ARG_TYPE.
// Node spells it three different ways depending on what arrived, and the difference
// carries information: a primitive is shown with its value, since the value is short
// and is usually the mistake, while an object is described by its constructor,
// since printing it would bury the message. null and undefined are named outright
// rather than described as a type, because "type object (null)" would read as a
// class of values when it is one particular value.
func receivedDescription(v Value) string {
	switch v.Kind() {
	case KindNull:
		return "Received null"
	case KindUndefined:
		return "Received undefined"
	case KindFunc:
		if name := functionNameFor(v); name != "" {
			return "Received function " + name
		}
		return "Received function (anonymous)"
	case KindArray:
		return "Received an instance of Array"
	case KindObject:
		return "Received an instance of Object"
	default:
		return "Received type " + v.TypeOf().ToGoString() + " (" + receivedLiteral(v) + ")"
	}
}

// receivedLiteral renders a primitive the way Node shows it inside the parentheses:
// a string in single quotes, a bigint with its n suffix, everything else as itself.
// A long string is cut, because the point is to identify the argument rather than to
// reproduce it, and Node cuts at the same width.
func receivedLiteral(v Value) string {
	switch v.Kind() {
	case KindString:
		s := v.AsString().ToGoString()
		if len(s) > 25 {
			s = s[:25] + "..."
		}
		return "'" + strings.ReplaceAll(s, "'", `\'`) + "'"
	case KindBigInt:
		return ToString(v).ToGoString() + "n"
	default:
		// StringCoerce rather than ToString, because a symbol reaching here is exactly
		// the case the abstract ToString throws on, and throwing while building the
		// message for another throw would lose the original error.
		return StringCoerce(v).ToGoString()
	}
}

// functionNameFor reads a function value's own name property, the name Node prints
// when a function arrives where a string was wanted. An anonymous function has none
// and is described as anonymous instead.
func functionNameFor(v Value) string {
	n := v.Get(FromGoString("name"))
	if n.Kind() != KindString {
		return ""
	}
	return n.AsString().ToGoString()
}
