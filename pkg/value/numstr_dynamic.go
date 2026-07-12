package value

// This file formats the number-to-string methods whose digit count is only known
// at runtime: a non-literal or out-of-range argument to toFixed, toExponential,
// and toPrecision. Each applies ToInteger to the count, throws the RangeError
// JavaScript raises when the result falls outside the method's valid range, and
// otherwise delegates to the exact formatter. The AOT lowerer routes a literal
// count in range straight to the exact formatter and only reaches these when the
// range cannot be checked at compile time, so the runtime carries the check.

// formatDigits applies ToInteger to a runtime digit count and returns it as an int
// when it lands in [lo, hi], throwing the RangeError JavaScript raises otherwise.
// A non-finite count (NaN truncates to 0, an infinity stays outside any range)
// throws the same way a finite out-of-range count does.
func formatDigits(digits float64, lo, hi int, msg string) int {
	i := toInteger(digits)
	if i < float64(lo) || i > float64(hi) {
		Throw(NewRangeError(FromGoString(msg)))
	}
	return int(i)
}

// NumberToFixedDynamic is n.toFixed(digits) with a runtime digit count: it
// range-checks the count against 0..100 and formats through NumberToFixed.
func NumberToFixedDynamic(x, digits float64) BStr {
	return NumberToFixed(x, formatDigits(digits, 0, 100, "toFixed() digits argument must be between 0 and 100"))
}

// NumberToExponentialDynamic is n.toExponential(digits) with a runtime digit
// count: it range-checks the count against 0..100 and formats through
// NumberToExponential.
func NumberToExponentialDynamic(x, digits float64) BStr {
	return NumberToExponential(x, formatDigits(digits, 0, 100, "toExponential() argument must be between 0 and 100"))
}

// NumberToPrecisionDynamic is n.toPrecision(precision) with a runtime precision:
// it range-checks the precision against 1..100 (zero significant digits is not a
// valid precision) and formats through NumberToPrecision.
func NumberToPrecisionDynamic(x, precision float64) BStr {
	return NumberToPrecision(x, formatDigits(precision, 1, 100, "toPrecision() argument must be between 1 and 100"))
}
