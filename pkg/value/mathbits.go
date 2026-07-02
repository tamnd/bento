package value

import "math/bits"

// This file owns the three Math methods that are integer or single-precision
// operations rather than transcendental ones. They belong in the value package,
// next to ToInt32 and ToUint32, because they share those coercions and, unlike
// sin or log, they are bit-exact: every finite double maps to one answer with no
// dependence on a libm, so the emitted Go agrees with the engine to the last bit.

// Fround rounds a number to the nearest single-precision float and back, the
// ECMAScript Math.fround. A round trip through float32 is exactly that: the value
// is rounded to the nearest representable float32 with ties to even, then widened
// back to a double. NaN stays NaN, the infinities and the signed zeros pass
// through unchanged, and a magnitude past the float32 range becomes the matching
// infinity, all of which the float32 conversion already does.
func Fround(x float64) float64 {
	return float64(float32(x))
}

// Clz32 counts the leading zero bits of a number read as a 32-bit unsigned
// integer, the ECMAScript Math.clz32. The argument is coerced with ToUint32
// first, so NaN, the infinities, and a fraction reduce the same way the bitwise
// operators reduce them, and then the count runs on the 32-bit value: zero has no
// set bit, so it counts the full 32, which bits.LeadingZeros32 returns directly.
func Clz32(x float64) float64 {
	return float64(bits.LeadingZeros32(ToUint32(x)))
}

// Clz32U counts the leading zero bits of a value already held as a 32-bit
// unsigned integer, the integer-typed core of Clz32. Clz32 exists for the float64
// value model, where its argument is a number that must run through ToUint32
// first; Clz32U is what the lowerer emits once a local is specialized to int32 and
// the coercion is already done, so the leading-zero count is a single machine
// instruction with no float round trip. The two agree by construction: Clz32(x) is
// Clz32U(ToUint32(x)).
func Clz32U(u uint32) int32 {
	return int32(bits.LeadingZeros32(u))
}

// Imul multiplies two numbers as 32-bit signed integers, the ECMAScript Math.imul,
// the one multiply that keeps only the low 32 bits rather than the full double
// product. Each operand is coerced with ToInt32, the product of two int32 values
// wraps modulo 2^32 in Go exactly as two's-complement multiplication requires, and
// the wrapped int32 widens back to a number.
func Imul(a, b float64) float64 {
	return float64(ToInt32(a) * ToInt32(b))
}
