package value

import "math"

// This file implements the primitive cases of the ToBoolean conversion behind
// Boolean(x), the companion to the String and Number coercions. The number and
// string rules are the ones a Go truthiness test does not express directly: a
// number is falsy only at zero or NaN (and Go's own zero test would call NaN
// truthy), and a string is falsy only when it is empty, regardless of content, so
// the string "0" and "false" are both truthy.

// NumberToBool returns the JavaScript Boolean(x) of a number, the ECMAScript
// ToBoolean applied to a Number: false for +0, -0, and NaN, and true for every
// other value. The NaN guard is what a bare x != 0 would miss, since NaN compares
// unequal to zero.
func NumberToBool(x float64) bool {
	return x != 0 && !math.IsNaN(x)
}

// StringToBool returns the JavaScript Boolean(s) of a string, the ECMAScript
// ToBoolean applied to a String: true for any non-empty string and false only for
// the empty one. The content does not matter, so "0" and "false" are both true.
func StringToBool(s BStr) bool {
	return s.Length() > 0
}
