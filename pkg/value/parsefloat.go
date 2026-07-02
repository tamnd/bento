package value

import (
	"math"
	"strconv"
	"strings"
)

// This file implements parseFloat, the lenient sibling of Number(x) on a string.
// It differs from StringToNumber in three ways that keep it from sharing that code
// path: it trims only leading whitespace (not trailing), it reads the longest
// prefix that forms a valid decimal rather than requiring the whole string to
// match, and it does not accept the 0x/0b/0o radix forms, so parseFloat("0x1F") is
// 0 (it reads the "0" and stops at the "x"). An exponent marker with no digits is
// not part of the prefix either, so parseFloat("1e") is 1, not NaN.

// ParseFloat returns the JavaScript parseFloat(s) of a string: the Number value of
// the longest leading substring that is a decimal literal, or NaN when no such
// prefix exists.
func ParseFloat(s BStr) float64 {
	// parseFloat trims only leading whitespace, so a value with a trailing tail
	// still parses. TrimStart reuses the exact StrWhiteSpace set.
	str := s.TrimStart().ToGoString()
	n := decimalPrefixLen(str)
	if n == 0 {
		return math.NaN()
	}
	// The prefix is a valid decimal literal, so it is also a valid Go float literal.
	// A range error still returns the right value (signed infinity or zero).
	f, _ := strconv.ParseFloat(str[:n], 64)
	return f
}

// decimalPrefixLen returns the length of the longest prefix of s that is a
// JavaScript StrDecimalLiteral: an optional sign, then either the word Infinity or
// a mantissa (digits with an optional fraction, or a bare fraction) followed by an
// optional exponent. It returns 0 when no such prefix exists. The exponent is
// included only when it carries at least one digit, so a dangling "e" is left out
// of the prefix rather than failing the whole parse.
func decimalPrefixLen(s string) int {
	i := 0
	if i < len(s) && (s[i] == '+' || s[i] == '-') {
		i++
	}
	if strings.HasPrefix(s[i:], "Infinity") {
		return i + len("Infinity")
	}
	intDigits := 0
	for i < len(s) && isASCIIDigit(s[i]) {
		i++
		intDigits++
	}
	fracDigits := 0
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && isASCIIDigit(s[i]) {
			i++
			fracDigits++
		}
	}
	if intDigits == 0 && fracDigits == 0 {
		return 0 // no mantissa means no valid prefix, even if a sign was seen
	}
	mantEnd := i
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		j := i + 1
		if j < len(s) && (s[j] == '+' || s[j] == '-') {
			j++
		}
		expDigits := 0
		for j < len(s) && isASCIIDigit(s[j]) {
			j++
			expDigits++
		}
		if expDigits > 0 {
			i = j
		} else {
			i = mantEnd // a dangling exponent marker is not part of the prefix
		}
	}
	return i
}
