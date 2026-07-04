package value

import (
	"math"
	"math/big"
	"strconv"
)

// This file implements Number.prototype.toExponential(fractionDigits), the
// exponential-notation number-to-string conversion. Like toFixed, the
// specification rounds the exact value of the double and breaks a tie by picking
// the larger significand, so the rounding runs in exact rational arithmetic
// through math/big rather than strconv's round-to-even 'e' format, which would
// diverge on a tie, and the exponent is printed with an explicit sign and the
// fewest digits (e+3, e-4) the way V8 and JavaScriptCore print it, not the
// two-digit zero-padded exponent Go's strconv emits.

// NumberToExponential returns the JavaScript n.toExponential(digits) of a number,
// formatting it with one integer digit, exactly digits fraction digits, and a
// signed decimal exponent. digits must be in 0..100, which the caller guarantees
// by only lowering a literal in range; the non-finite cases match Number::toString.
func NumberToExponential(x float64, digits int) BStr {
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
		body = expZero(digits)
	} else {
		body = expDigits(x, digits)
	}
	if negative {
		return FromGoString("-" + body)
	}
	return FromGoString(body)
}

// expZero formats zero with f fraction digits: a single zero integer digit, f
// zero fraction digits, and the exponent zero, so (0).toExponential(2) is
// "0.00e+0".
func expZero(f int) string {
	if f == 0 {
		return "0e+0"
	}
	m := make([]byte, f+2)
	m[0] = '0'
	m[1] = '.'
	for i := 2; i < len(m); i++ {
		m[i] = '0'
	}
	return string(m) + "e+0"
}

// expDigits formats a positive finite double with f fraction digits in
// exponential notation, one integer digit before the point and f after it, so
// its significand is f+1 significant digits.
func expDigits(x float64, f int) string {
	s, e := significand(x, f+1)
	var m string
	if f == 0 {
		m = s
	} else {
		m = s[:1] + "." + s[1:]
	}
	return m + "e" + expPart(e)
}

// significand returns the sig most significant decimal digits of a positive
// finite double and its decimal exponent e, where the value is s × 10^(e-sig+1)
// and s has exactly sig digits. It rounds the exact value with ties toward the
// larger significand, and it pins e even when rounding carries a nine up into a
// new place (9.99 to two digits is "10", exponent 1, which normalizes to "10"
// having three digits so e is bumped and the significand becomes "10" of the
// right length). The exponent estimate comes from log10 and is corrected by the
// digit count of the rounded significand, so a float rounding error in the
// estimate cannot leave the significand a digit too long or too short. It is the
// shared core of toExponential and toPrecision, which differ only in how they lay
// the digits and the exponent out.
func significand(x float64, sig int) (string, int) {
	r := new(big.Rat).SetFloat64(x) // exact: a float64 is a dyadic rational
	e := int(math.Floor(math.Log10(x)))
	var s string
	for i := 0; i < 4; i++ {
		// sig significant digits with the point after the first means rounding to
		// (sig-1) - e places past the decimal point.
		s = scaleRound(r, (sig-1)-e).String()
		if len(s) == sig {
			break
		}
		if len(s) > sig {
			e++
		} else {
			e--
		}
	}
	return s, e
}

// scaleRound returns floor(r * 10^k + 1/2) for a non-negative rational r, the
// integer that minimizes the distance to the scaled value and breaks a tie toward
// the larger integer, matching the specification's round-half-up on the exact
// double. k may be negative, which divides rather than multiplies.
func scaleRound(r *big.Rat, k int) *big.Int {
	mag := k
	if mag < 0 {
		mag = -mag
	}
	pow := new(big.Rat).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(mag)), nil))
	scaled := new(big.Rat).Set(r)
	if k >= 0 {
		scaled.Mul(scaled, pow)
	} else {
		scaled.Quo(scaled, pow)
	}
	scaled.Add(scaled, big.NewRat(1, 2))
	// floor of a non-negative rational is the truncating quotient of its terms.
	return new(big.Int).Quo(scaled.Num(), scaled.Denom())
}

// expPart formats the exponent with an explicit sign and no leading zeros, the
// form JavaScript prints: a non-negative exponent as +N and a negative one as -N.
func expPart(e int) string {
	if e >= 0 {
		return "+" + strconv.Itoa(e)
	}
	return "-" + strconv.Itoa(-e)
}
