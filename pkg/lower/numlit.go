package lower

import (
	"go/token"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// This file decodes a JavaScript numeric literal's source text into the Go literal
// that denotes the same float64 value (05_type_lowering, the numeric-literal
// slice). A JavaScript number is always a float64, but its source can be written
// several ways the earlier plain-decimal path did not accept: hexadecimal, binary,
// and octal integers, underscore digit separators, and exponents. The two
// languages' numeric grammars overlap enough that a well-formed JavaScript literal
// with its separators stripped is also a well-formed Go literal for the same
// value (strict-mode modules reject the one ambiguous case, a leading-zero octal
// like 010, before it ever reaches here), so the decoder validates the value is a
// finite number and hands the cleaned text straight to Go rather than reformatting
// it, which keeps the emitted literal readable.

// decodeNumericLiteral decodes a JavaScript numeric literal into the Go literal
// value and token kind that denote the same number. It returns false for a literal
// this slice does not lower: a BigInt (the trailing n is a different type, not a
// number) and a literal whose value is not finite (an exponent like 1e400 that
// overflows to Infinity), which Go would reject as an overflowing constant.
func decodeNumericLiteral(text string) (value string, kind token.Token, ok bool) {
	if text == "" || strings.HasSuffix(text, "n") {
		return "", 0, false // empty or a BigInt literal
	}
	clean := strings.ReplaceAll(text, "_", "")
	if clean == "" {
		return "", 0, false
	}
	// A radix-prefixed integer: hex, binary, or octal. Its digits can name a value
	// far past uint64, so it is validated through a big.Int and only rejected if
	// the value does not fit a finite float64.
	if len(clean) >= 2 && clean[0] == '0' {
		switch clean[1] {
		case 'x', 'X':
			return clean, token.INT, radixIsFinite(clean[2:], 16)
		case 'b', 'B':
			return clean, token.INT, radixIsFinite(clean[2:], 2)
		case 'o', 'O':
			return clean, token.INT, radixIsFinite(clean[2:], 8)
		}
	}
	// A decimal integer, fraction, or exponent. ParseFloat both validates it and
	// reports the range error that flags an overflow to Infinity.
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return "", 0, false
	}
	kind = token.INT
	if strings.ContainsAny(clean, ".eE") {
		kind = token.FLOAT
	}
	return clean, kind, true
}

// numericLiteralValue decodes a JavaScript numeric literal's source text to the
// float64 it denotes, for a caller that needs the value itself rather than the Go
// literal, such as reading a radix argument at compile time. It mirrors
// decodeNumericLiteral's grammar (the radix-prefixed integers, the underscore
// separators, and the decimal forms) and rejects the same non-finite and BigInt
// cases, so the two stay in step on what counts as a lowerable number.
func numericLiteralValue(text string) (float64, bool) {
	if text == "" || strings.HasSuffix(text, "n") {
		return 0, false
	}
	clean := strings.ReplaceAll(text, "_", "")
	if clean == "" {
		return 0, false
	}
	if len(clean) >= 2 && clean[0] == '0' {
		base := 0
		switch clean[1] {
		case 'x', 'X':
			base = 16
		case 'b', 'B':
			base = 2
		case 'o', 'O':
			base = 8
		}
		if base != 0 {
			i, ok := new(big.Int).SetString(clean[2:], base)
			if !ok {
				return 0, false
			}
			f, _ := new(big.Float).SetInt(i).Float64()
			if math.IsInf(f, 0) {
				return 0, false
			}
			return f, true
		}
	}
	v, err := strconv.ParseFloat(clean, 64)
	if err != nil || math.IsInf(v, 0) || math.IsNaN(v) {
		return 0, false
	}
	return v, true
}

// bigIntLiteralValue decodes a bigint literal's source text (a numeric literal
// with a trailing n, like 123n or 0xffn or 1_000_000_000_000_000_000_000n) to the
// arbitrary-precision integer it denotes. It reads the same radix prefixes and
// underscore separators a numeric literal accepts, minus the fraction and exponent
// forms a bigint cannot take. The caller picks the emitted shape by the value's
// width: within int64 it is a big.NewInt call, wider it is an interned package var
// parsed once at init.
func bigIntLiteralValue(text string) (*big.Int, bool) {
	if !strings.HasSuffix(text, "n") {
		return nil, false
	}
	clean := strings.ReplaceAll(text[:len(text)-1], "_", "")
	if clean == "" {
		return nil, false
	}
	base := 10
	digits := clean
	if len(clean) >= 2 && clean[0] == '0' {
		switch clean[1] {
		case 'x', 'X':
			base, digits = 16, clean[2:]
		case 'b', 'B':
			base, digits = 2, clean[2:]
		case 'o', 'O':
			base, digits = 8, clean[2:]
		}
	}
	v, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return nil, false
	}
	return v, true
}

// radixIsFinite reports whether the base-n integer written as digits names a value
// that fits a finite float64. It parses through a big.Int so a very long run of
// digits does not overflow an intermediate integer, then checks the float64 the
// value rounds to is not an infinity.
func radixIsFinite(digits string, base int) bool {
	if digits == "" {
		return false
	}
	i, ok := new(big.Int).SetString(digits, base)
	if !ok {
		return false
	}
	f, _ := new(big.Float).SetInt(i).Float64()
	return !math.IsInf(f, 0)
}
