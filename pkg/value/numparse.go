package value

import (
	"math"
	"math/big"
	"strconv"
	"strings"
)

// This file implements the String-to-Number conversion behind Number(x) on a
// string (the ECMAScript ToNumber applied to a String, StringToNumber). It cannot
// delegate to strconv.ParseFloat: that grammar and JavaScript's disagree in both
// directions. ParseFloat accepts forms JavaScript rejects to NaN, "Inf", a
// hexadecimal float like "0x1p-2", "NaN", and underscore digit separators, and the
// two treat an empty string differently (JavaScript's Number("") is +0, not an
// error). So the string is validated against JavaScript's StrNumericLiteral grammar
// first, and strconv is used only to convert a string already known to conform.

// StringToNumber returns the JavaScript Number(s) of a string. It trims the
// ECMAScript whitespace, maps the empty result to +0, recognizes the
// radix-prefixed integer forms and the signed decimal (including Infinity), and
// returns NaN for anything that does not match the grammar.
func StringToNumber(s BStr) float64 {
	// Trim the ECMAScript whitespace on the Go-string view rather than through
	// BStr.Trim, which would materialize the whole string into a UTF-16 code-unit
	// slice first. ToGoString keeps the UTF-8 fast path (the shape every numeric
	// string that reaches here has), and strings.TrimFunc returns a sub-slice with no
	// allocation, so the common no-whitespace case does no work. A lone surrogate
	// cannot be numeric, so decoding it to U+FFFD on this path is harmless: the result
	// is NaN either way.
	str := strings.TrimFunc(s.ToGoString(), isStringWhiteSpaceRune)
	if str == "" {
		return 0
	}
	// The non-decimal integer forms: 0x, 0b, 0o, each with no sign. The leading '0'
	// means a sign cannot precede them, which matches the grammar.
	if len(str) >= 2 && str[0] == '0' {
		switch str[1] {
		case 'x', 'X':
			return parseRadix(str[2:], 16)
		case 'b', 'B':
			return parseRadix(str[2:], 2)
		case 'o', 'O':
			return parseRadix(str[2:], 8)
		}
	}
	if !isJSDecimal(str) {
		return math.NaN()
	}
	// The validated decimal is also a valid Go float literal, so ParseFloat converts
	// it. A range error still returns the right value: an overflow yields the signed
	// infinity and an underflow yields zero, both of which are what Number produces.
	f, _ := strconv.ParseFloat(str, 64)
	return f
}

// parseRadix converts the digits of a non-decimal integer to a float64, or NaN
// when they are empty or hold a character that is not a digit of the base. It
// validates the digits itself rather than trusting big.Int.SetString, which would
// also accept a leading sign, so that Number("0x-1") is NaN. A big.Int carries a
// digit run too long for a fixed integer before it rounds to float64.
func parseRadix(digits string, base int) float64 {
	if !validRadixDigits(digits, base) {
		return math.NaN()
	}
	// Fast path: a digit run that fits in a uint64 needs no big.Int. strconv.ParseUint
	// rejects a sign and any non-base digit (validRadixDigits already screened those
	// out), so a success here is an exact integer, and uint64-to-float64 rounds to
	// nearest even, the same rounding big.Float.Float64 would apply, so the result is
	// bit-identical to the general path. Only a run too long for uint64 (ErrRange)
	// falls through to big.Int, where the arbitrary-precision value carries the full
	// digit string before it rounds to a float64.
	if u, err := strconv.ParseUint(digits, base, 64); err == nil {
		return float64(u)
	}
	i, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return math.NaN()
	}
	f, _ := new(big.Float).SetInt(i).Float64()
	return f
}

// validRadixDigits reports whether every character of s is a digit of the base,
// with at least one digit. It admits no sign, underscore, or point, so only a bare
// run of base-n digits passes.
func validRadixDigits(s string, base int) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		var d int
		switch {
		case c >= '0' && c <= '9':
			d = int(c - '0')
		case c >= 'a' && c <= 'f':
			d = int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			d = int(c-'A') + 10
		default:
			return false
		}
		if d >= base {
			return false
		}
	}
	return true
}

// isJSDecimal reports whether s is a JavaScript StrDecimalLiteral: an optional
// sign, then either the word Infinity or a decimal mantissa (digits with an
// optional fraction, or a bare fraction) followed by an optional exponent. It
// rejects the forms strconv would otherwise accept, "Inf", a hexadecimal float,
// and a bare "NaN", by requiring the whole string to be consumed by this grammar.
func isJSDecimal(s string) bool {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if s[i:] == "Infinity" {
		return true
	}
	digitsBefore := 0
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
		digitsBefore++
	}
	digitsAfter := 0
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
			digitsAfter++
		}
	}
	if digitsBefore == 0 && digitsAfter == 0 {
		return false // a mantissa needs at least one digit
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		expDigits := 0
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
			expDigits++
		}
		if expDigits == 0 {
			return false // an exponent marker needs at least one digit
		}
	}
	return i == len(s)
}

func isASCIIDigit(c byte) bool { return c >= '0' && c <= '9' }

// BoolToNumber returns the JavaScript Number(b) of a boolean, 1 for true and 0 for
// false, the ECMAScript ToNumber applied to a Boolean.
func BoolToNumber(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
