package value

import (
	"math"
	"math/big"
)

// This file implements Number.prototype.toFixed(fractionDigits), the fixed-point
// number-to-string conversion. The specification rounds the exact value of the
// double, not a decimal approximation of it, and breaks a tie by picking the
// larger result, so (1.005).toFixed(2) is "1.00" because the double nearest 1.005
// is just below it, and (0.5).toFixed(0) is "1" because an exact tie rounds up.
// strconv's 'f' format rounds ties to even, which would diverge on both, so the
// rounding runs in exact rational arithmetic through math/big to match V8 and
// JavaScriptCore byte for byte.

// NumberToFixed returns the JavaScript n.toFixed(digits) of a number, formatting
// it with exactly digits fraction digits. digits must be in 0..100, which the
// caller guarantees by only lowering a literal in range; a value at or past 1e21
// falls back to Number::toString the way the specification requires, and the
// non-finite cases match it too.
func NumberToFixed(x float64, digits int) BStr {
	switch {
	case math.IsNaN(x):
		return FromGoString("NaN")
	case math.IsInf(x, 1):
		return FromGoString("Infinity")
	case math.IsInf(x, -1):
		return FromGoString("-Infinity")
	}
	// At or past 1e21 the specification returns ToString(x) directly, since the
	// integer part alone outruns the fixed-point form.
	if math.Abs(x) >= 1e21 {
		return NumberToString(x)
	}
	negative := math.Signbit(x) && x != 0
	if negative {
		x = -x
	}
	body := fixedDigits(x, digits)
	if negative {
		return FromGoString("-" + body)
	}
	return FromGoString(body)
}

// fixedDigits formats a non-negative finite double with exactly f fraction digits,
// rounding the exact value with ties going up. It scales the exact rational value
// by 10^f, adds one half, and takes the floor, which selects the integer n that
// minimizes the distance to the scaled value and breaks a tie toward the larger n,
// then places the decimal point f digits from the right.
func fixedDigits(x float64, f int) string {
	r := new(big.Rat).SetFloat64(x) // exact: a float64 is a dyadic rational
	pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(f)), nil)
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(pow))
	scaled.Add(scaled, big.NewRat(1, 2))
	// floor of a non-negative rational is the truncating quotient of its terms.
	n := new(big.Int).Quo(scaled.Num(), scaled.Denom())

	digits := n.String()
	if f == 0 {
		return digits
	}
	// Pad so there are at least f fraction digits plus one integer digit, then cut.
	for len(digits) <= f {
		digits = "0" + digits
	}
	return digits[:len(digits)-f] + "." + digits[len(digits)-f:]
}
