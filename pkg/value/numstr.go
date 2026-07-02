package value

import (
	"math"
	"strconv"
	"strings"
)

// This file implements the decimal Number-to-String conversion behind String(x)
// on a number and, later, template literals and other number stringifications
// (the ECMAScript Number::toString with radix 10). It cannot be strconv's 'g'
// format: JavaScript switches to exponential notation at different thresholds
// (a value whose decimal point sits past position 21, or before position -5) and
// writes the exponent without the two-digit zero padding Go uses, so (1e-7)
// stringifies to "1e-7" in JavaScript but "1e-07" through strconv. The algorithm
// below follows the specification directly: it takes the shortest round-tripping
// decimal digits from strconv in scientific form, then places the decimal point
// or the exponent per the specification's exact rules.

// NumberToString returns the JavaScript String(x) of a number, the decimal
// Number::toString. It handles the non-finite and zero cases directly and then
// formats a finite nonzero value from its shortest round-tripping digits.
func NumberToString(x float64) BStr {
	switch {
	case math.IsNaN(x):
		return FromGoString("NaN")
	case x == 0:
		return FromGoString("0") // both +0 and -0 stringify to "0"
	case math.IsInf(x, 1):
		return FromGoString("Infinity")
	case math.IsInf(x, -1):
		return FromGoString("-Infinity")
	}
	if x < 0 {
		return FromGoString("-" + formatFinite(-x))
	}
	return FromGoString(formatFinite(x))
}

// BoolToString returns the JavaScript String(b) of a boolean, "true" or "false",
// the ECMAScript Boolean::toString.
func BoolToString(b bool) BStr {
	if b {
		return FromGoString("true")
	}
	return FromGoString("false")
}

// formatFinite formats a positive, finite, nonzero number the way
// Number::toString does. It gets the shortest decimal that round-trips from
// strconv in scientific form, which yields the digit string s and an exponent, and
// then applies the specification's placement rules over n, the position of the
// decimal point relative to the digits (value = s x 10^(n-k), k = len(s)).
func formatFinite(x float64) string {
	digits, exp := shortestDigits(x)
	k := len(digits)
	n := exp + 1 // strconv's scientific exponent is for one leading digit, so n = exp + 1

	switch {
	case k <= n && n <= 21:
		// An integer that fits: all digits, then n-k trailing zeros.
		return digits + zeros(n-k)
	case 0 < n && n <= 21:
		// A decimal point inside the digits.
		return digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		// A small magnitude: "0." then -n leading zeros then the digits.
		return "0." + zeros(-n) + digits
	default:
		// Exponential notation. The exponent shown is n-1.
		return exponential(digits, n-1)
	}
}

// exponential writes the digits in exponential form with the given exponent, the
// last two branches of Number::toString: a single digit has no fraction, and the
// exponent is written with an explicit sign and no zero padding.
func exponential(digits string, e int) string {
	mant := digits
	if len(digits) > 1 {
		mant = digits[:1] + "." + digits[1:]
	}
	sign := "+"
	if e < 0 {
		sign = "-"
		e = -e
	}
	return mant + "e" + sign + strconv.Itoa(e)
}

// shortestDigits returns the shortest decimal digit string that round-trips to x
// and the base-ten exponent of its leading digit, so value = digits x
// 10^(exp-len+1). It reads strconv's scientific form, whose mantissa carries
// exactly those shortest digits, and strips the decimal point.
func shortestDigits(x float64) (digits string, exp int) {
	// strconv 'e' with precision -1 gives the shortest form, like "1.23e+04".
	s := strconv.FormatFloat(x, 'e', -1, 64)
	// Split off the exponent.
	mant, expStr, _ := strings.Cut(s, "e")
	exp, _ = strconv.Atoi(expStr)
	// Drop the decimal point to leave the bare digit string.
	if before, after, found := strings.Cut(mant, "."); found {
		mant = before + after
	}
	return mant, exp
}

// zeros returns a string of n '0' characters, or empty when n is not positive.
func zeros(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat("0", n)
}
