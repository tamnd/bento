package value

import "math"

// This file owns the integer coercions the bitwise operators need. JavaScript
// numbers are float64, but the bitwise operators (&, |, ^, <<, >>, >>>) do not
// work on floats: each operand is first coerced to a 32-bit integer, the
// operation runs on those integers, and the result is turned back into a number.
// ToInt32 and ToUint32 are those coercions, the ECMAScript ToInt32 and ToUint32
// abstract operations (05_type_lowering, the bitwise slice). Doing them wrong is
// easy: a naive int64 cast overflows on a number past 2^63 and truncation alone
// misses the modulo-2^32 wrap, so both go through a float modulo that is exact
// for every finite double.

// twoPow32 is 2^32, the modulus both coercions reduce by. It is exactly
// representable as a float64, so the float modulo below is exact.
const twoPow32 = 4294967296.0

// ToUint32 coerces a number to an unsigned 32-bit integer, the ECMAScript
// ToUint32 operation. NaN and the infinities become 0, every other value is
// truncated toward zero and then reduced modulo 2^32 into [0, 2^32). The
// reduction uses math.Mod rather than an integer cast so it stays correct for a
// number too large to fit in an int64, where a direct conversion would overflow.
func ToUint32(n float64) uint32 {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return 0
	}
	m := math.Mod(math.Trunc(n), twoPow32)
	if m < 0 {
		m += twoPow32
	}
	return uint32(m)
}

// ToInt32 coerces a number to a signed 32-bit integer, the ECMAScript ToInt32
// operation. It shares every step with ToUint32 and differs only in the final
// interpretation: a value at or above 2^31 wraps to its negative two's-complement
// form. Reinterpreting the uint32 bits as int32 is exactly that wrap, so ToInt32
// is ToUint32 reread as signed.
func ToInt32(n float64) int32 {
	return int32(ToUint32(n))
}

// Round rounds a number to the nearest integer, Math.round. It is not math.Round:
// JavaScript breaks a tie by rounding toward +Infinity (Math.round(2.5) is 3 and
// Math.round(-2.5) is -2), where Go's math.Round rounds a tie away from zero
// (math.Round(-2.5) is -3). Rounding down and then bumping when the fraction
// reaches one half gives the +Infinity tie-break directly, and it avoids the
// floor(x+0.5) trap where a value just under one half like 0.49999999999999994
// adds up to 1.0 and rounds the wrong way. NaN and the infinities pass through.
// A result of zero keeps the sign of x, so Math.round(-0.4) stays -0 like the
// specification requires.
func Round(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return x
	}
	f := math.Floor(x)
	r := f
	if x-f >= 0.5 {
		r = f + 1
	}
	if r == 0 && math.Signbit(x) {
		return math.Copysign(0, -1)
	}
	return r
}

// Sign returns the sign of a number, Math.sign: 1 for a positive number, -1 for a
// negative one, and the argument itself for zero or NaN. Go has no math.Sign, and
// returning x for the zero and NaN cases is what keeps the signed zeros and NaN
// flowing through unchanged the way the specification asks.
func Sign(x float64) float64 {
	if math.IsNaN(x) {
		return x
	}
	if x > 0 {
		return 1
	}
	if x < 0 {
		return -1
	}
	return x
}

// maxSafeInteger is 2^53 - 1, Number.MAX_SAFE_INTEGER: the largest integer that
// has no other double sharing its bits, so integers up to it round-trip exactly.
const maxSafeInteger = 9007199254740991.0

// NumberIsNaN reports whether n is the NaN value, Number.isNaN. Unlike the global
// isNaN it does no coercion, but the argument is already a number here, so it is
// just the NaN test. It is not the same as `n != n` written in Go source only in
// that it names the intent; the semantics are identical.
func NumberIsNaN(n float64) bool {
	return math.IsNaN(n)
}

// NumberIsFinite reports whether n is a finite number, Number.isFinite: neither
// an infinity nor NaN. Again no coercion, since the argument is a number.
func NumberIsFinite(n float64) bool {
	return !math.IsInf(n, 0) && !math.IsNaN(n)
}

// NumberIsInteger reports whether n is an integer value, Number.isInteger: finite
// and equal to its own truncation. NaN and the infinities are not integers, which
// the finiteness test rules out before the truncation compare.
func NumberIsInteger(n float64) bool {
	return NumberIsFinite(n) && math.Trunc(n) == n
}

// NumberIsSafeInteger reports whether n is a safe integer, Number.isSafeInteger:
// an integer whose magnitude is at most 2^53 - 1, so it is the only double with
// its value and integer arithmetic on it is exact.
func NumberIsSafeInteger(n float64) bool {
	return NumberIsInteger(n) && math.Abs(n) <= maxSafeInteger
}
