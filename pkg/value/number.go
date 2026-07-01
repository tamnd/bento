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
