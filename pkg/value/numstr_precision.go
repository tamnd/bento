package value

import (
	"math"
	"strings"
)

// This file implements Number.prototype.toPrecision(precision), the
// significant-digits number-to-string conversion. It shares the exact-rounding
// significand core with toExponential (numstr_exp.go); the two differ only in how
// the digits and the exponent are laid out. toPrecision picks a fixed form when
// the exponent is in range and an exponential form when it is not, exactly the
// choice the specification makes on the rounded exponent, so a large or tiny
// value prints in scientific notation while a mid-range one keeps its point.

// NumberToPrecision returns the JavaScript n.toPrecision(precision) of a number,
// formatting it with exactly precision significant digits. precision must be in
// 1..100, which the caller guarantees by only lowering a literal in range; the
// non-finite cases match Number::toString.
func NumberToPrecision(x float64, precision int) BStr {
	switch {
	case math.IsNaN(x):
		return FromGoString("NaN")
	case math.IsInf(x, 1):
		return FromGoString("Infinity")
	case math.IsInf(x, -1):
		return FromGoString("-Infinity")
	}
	negative := math.Signbit(x) && x != 0
	if negative {
		x = -x
	}
	var body string
	if x == 0 {
		body = precisionZero(precision)
	} else {
		body = precisionDigits(x, precision)
	}
	if negative {
		return FromGoString("-" + body)
	}
	return FromGoString(body)
}

// precisionZero formats zero with p significant digits: a single zero integer
// digit and, when p > 1, a decimal point followed by p-1 zero digits, so
// (0).toPrecision(3) is "0.00".
func precisionZero(p int) string {
	if p == 1 {
		return "0"
	}
	return "0." + strings.Repeat("0", p-1)
}

// precisionDigits formats a positive finite double with p significant digits. It
// rounds the exact value to p digits through the shared significand core, then
// chooses the layout the specification prescribes on the rounded exponent e: an
// exponential form when e is below -6 or at least p, and a fixed form otherwise,
// where the decimal point falls e+1 digits into the significand and a value below
// one gets a leading "0." with the right number of zeros.
func precisionDigits(x float64, p int) string {
	s, e := significand(x, p)
	// Out of the fixed-form window the specification uses exponential notation,
	// with p-1 digits after the point, the same shape toExponential(p-1) makes.
	if e < -6 || e >= p {
		var m string
		if p == 1 {
			m = s
		} else {
			m = s[:1] + "." + s[1:]
		}
		return m + "e" + expPart(e)
	}
	// A non-negative exponent puts the point e+1 digits in; when it lands past the
	// last digit (e == p-1) there is no fraction and the digits stand alone.
	if e >= 0 {
		if e == p-1 {
			return s
		}
		return s[:e+1] + "." + s[e+1:]
	}
	// A negative exponent is a value below one: "0.", then -e-1 leading zeros, then
	// all p significant digits.
	return "0." + strings.Repeat("0", -e-1) + s
}
