package value

import (
	"math"
	"math/big"
)

// This file implements parseInt, the radix-aware integer parse. It shares the
// lenient shape of parseFloat (trim only the leading whitespace, read a prefix and
// ignore the rest) but adds the ECMAScript radix rules: a radix of 0 or omitted
// means base 10 with automatic 0x detection, a radix of 16 also detects a 0x
// prefix, any other radix in 2..36 is used verbatim, and a radix outside that
// range is NaN. The passed radix is ToInt32-coerced first, so a fractional or NaN
// radix behaves as the specification says (NaN becomes 0, then 10).

// ParseInt returns the JavaScript parseInt(s, radix) of a string. A radix of 0
// stands for an omitted argument, which the specification treats identically to a
// radix of 0, so the compiler passes 0 when parseInt is called with one argument.
func ParseInt(s BStr, radix float64) float64 {
	str := s.TrimStart().ToGoString()
	i := 0
	sign := 1.0
	if i < len(str) && (str[i] == '+' || str[i] == '-') {
		if str[i] == '-' {
			sign = -1
		}
		i++
	}

	r := int(ToInt32(radix))
	stripPrefix := true
	switch {
	case r != 0:
		if r < 2 || r > 36 {
			return math.NaN()
		}
		if r != 16 {
			stripPrefix = false // only base 16 (and the default) reads a 0x prefix
		}
	default:
		r = 10
	}

	if stripPrefix && i+1 < len(str) && str[i] == '0' && (str[i+1] == 'x' || str[i+1] == 'X') {
		i += 2
		r = 16
	}

	// Read the longest run of digits valid in the radix. A digit is 0-9 or a letter
	// whose value is below the radix.
	start := i
	for i < len(str) && digitValue(str[i]) < r {
		i++
	}
	if i == start {
		return math.NaN() // no valid digit after the prefix
	}

	// Convert the digit run in the radix through a big.Int so a long run keeps its
	// value up to the float64 rounding the specification also applies.
	n, ok := new(big.Int).SetString(str[start:i], r)
	if !ok {
		return math.NaN()
	}
	f, _ := new(big.Float).SetInt(n).Float64()
	return sign * f
}

// digitValue returns the base-36 value of a digit character (0-9, then a-z or A-Z
// as 10-35), or 36 for any other byte so it never counts as a valid digit for a
// radix of 36 or below.
func digitValue(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'a' && c <= 'z':
		return int(c-'a') + 10
	case c >= 'A' && c <= 'Z':
		return int(c-'A') + 10
	default:
		return 36
	}
}
