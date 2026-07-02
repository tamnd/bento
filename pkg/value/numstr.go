package value

import (
	"math"
	"strconv"
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
//
// The whole conversion writes into a stack buffer and allocates once, for the
// result string. The shortest digits come from strconv.AppendFloat into a stack
// scratch (no intermediate heap string), the digits and exponent are read out of
// those bytes, and the placement rules append straight into the output buffer, so
// a program that stringifies numbers in a loop pays one allocation per value
// rather than the three the earlier split-and-reconcatenate form cost.

// NumberToString returns the JavaScript String(x) of a number, the decimal
// Number::toString. It handles the non-finite and zero cases directly and then
// formats a finite nonzero value from its shortest round-tripping digits.
func NumberToString(x float64) BStr {
	switch {
	case math.IsNaN(x):
		return fromASCII("NaN")
	case x == 0:
		return fromASCII("0") // both +0 and -0 stringify to "0"
	case math.IsInf(x, 1):
		return fromASCII("Infinity")
	case math.IsInf(x, -1):
		return fromASCII("-Infinity")
	}
	// Fast path for an integer value whose magnitude is at most 2^53. In that range
	// every integer is exactly representable as a float64, so it is its own shortest
	// round-tripping decimal, and JavaScript prints it in plain notation with no
	// point and no exponent, which is exactly strconv.FormatInt of the int64 value.
	// That skips the shortest-digits scientific parse and the decimal-point
	// placement the general path runs, the whole cost of stringifying a loop counter
	// or any other whole number. The bound must be 2^53, not the wider int64 range:
	// past 2^53 a float64 integer and its shortest decimal diverge (the stored value
	// 1234567890123456768 prints as 1234567890123456800), so a wider guard would
	// print the exact int64 where JavaScript prints the shorter rounding.
	if x == math.Trunc(x) && x >= -twoPow53 && x <= twoPow53 {
		var ibuf [20]byte
		return fromASCII(string(strconv.AppendInt(ibuf[:0], int64(x), 10)))
	}
	var buf [40]byte
	return fromASCII(string(appendFinite(buf[:0], x)))
}

// fromASCII wraps an all-ASCII string as a BStr with no code-unit scan. Every
// byte a number formats to (a digit, '.', '-', 'e', '+') is one UTF-16 code
// unit, so the length is the byte length and countUTF16Units would only confirm
// it; skipping that walk keeps the formatter's tail cheap. It must not be used
// for a string that could carry a multi-byte rune.
func fromASCII(s string) BStr {
	return BStr{utf8: s, lengthU16: len(s)}
}

// BoolToString returns the JavaScript String(b) of a boolean, "true" or "false",
// the ECMAScript Boolean::toString.
func BoolToString(b bool) BStr {
	if b {
		return fromASCII("true")
	}
	return fromASCII("false")
}

// appendFinite appends the JavaScript Number::toString of a finite nonzero x to
// dst and returns the extended slice. It works for either sign: a negative value
// contributes its '-' and is formatted by magnitude. The shortest round-tripping
// digits come from strconv in scientific form, which yields the digit bytes and
// an exponent, and then the specification's placement rules over n, the position
// of the decimal point relative to the digits (value = digits x 10^(n-k), k =
// len(digits)), decide where the point or the exponent goes.
func appendFinite(dst []byte, x float64) []byte {
	if x < 0 {
		dst = append(dst, '-')
		x = -x
	}

	// strconv 'e' with precision -1 gives the shortest round-tripping form, like
	// "1.23e+04", into a stack scratch so no intermediate string is allocated.
	var scratch [32]byte
	sci := strconv.AppendFloat(scratch[:0], x, 'e', -1, 64)

	// Split the mantissa from the exponent at 'e', then copy the mantissa digits
	// without the decimal point into a stack buffer. A shortest double mantissa is
	// at most 17 digits, so 24 bytes is ample.
	ei := indexByteIn(sci, 'e')
	var digitsBuf [24]byte
	k := 0
	for i := 0; i < ei; i++ {
		if sci[i] != '.' {
			digitsBuf[k] = sci[i]
			k++
		}
	}
	digits := digitsBuf[:k]
	exp := parseExpDigits(sci[ei+1:])
	n := exp + 1 // strconv's scientific exponent is for one leading digit, so n = exp + 1

	switch {
	case k <= n && n <= 21:
		// An integer that fits: all digits, then n-k trailing zeros.
		dst = append(dst, digits...)
		dst = appendZeros(dst, n-k)
	case 0 < n && n <= 21:
		// A decimal point inside the digits.
		dst = append(dst, digits[:n]...)
		dst = append(dst, '.')
		dst = append(dst, digits[n:]...)
	case -6 < n && n <= 0:
		// A small magnitude: "0." then -n leading zeros then the digits.
		dst = append(dst, '0', '.')
		dst = appendZeros(dst, -n)
		dst = append(dst, digits...)
	default:
		// Exponential notation. A single digit has no fraction, and the exponent,
		// shown as n-1, is written with an explicit sign and no zero padding.
		if k == 1 {
			dst = append(dst, digits[0])
		} else {
			dst = append(dst, digits[0], '.')
			dst = append(dst, digits[1:]...)
		}
		dst = append(dst, 'e')
		e := n - 1
		if e < 0 {
			dst = append(dst, '-')
			e = -e
		} else {
			dst = append(dst, '+')
		}
		dst = strconv.AppendInt(dst, int64(e), 10)
	}
	return dst
}

// parseExpDigits reads strconv's exponent field, the bytes after the 'e', which
// carry an explicit sign and at least two digits with a possible leading zero
// ("+05", "-07", "+308"). It returns the signed integer with no allocation.
func parseExpDigits(b []byte) int {
	i := 0
	neg := false
	if i < len(b) && (b[i] == '+' || b[i] == '-') {
		neg = b[i] == '-'
		i++
	}
	e := 0
	for ; i < len(b); i++ {
		e = e*10 + int(b[i]-'0')
	}
	if neg {
		return -e
	}
	return e
}

// indexByteIn returns the index of the first c in b, or len(b) when it is
// absent. strconv's 'e' form always carries the 'e' this looks for, so the
// absent case never arises on the formatter's path; it is defined for totality.
func indexByteIn(b []byte, c byte) int {
	for i := 0; i < len(b); i++ {
		if b[i] == c {
			return i
		}
	}
	return len(b)
}

// appendZeros appends n '0' bytes to dst, or nothing when n is not positive. The
// counts here are small (the trailing zeros of an integer that fits, or the
// leading zeros of a sub-unit magnitude), so a plain loop beats allocating a
// zero run.
func appendZeros(dst []byte, n int) []byte {
	for i := 0; i < n; i++ {
		dst = append(dst, '0')
	}
	return dst
}
