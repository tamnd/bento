package value

import (
	"math"
	"strconv"
)

// This file implements Number.prototype.toString(radix) for a radix other than
// 10, the non-decimal number-to-string conversion. Radix 10 is Number::toString,
// which NumberToString already produces, so the caller routes a radix-10 call
// there and only a radix in 2..9 or 11..36 reaches here. The specification leaves
// the exact digits of the fractional part to the implementation but every engine
// (V8 behind node and deno, JavaScriptCore behind bun) uses the same algorithm,
// David Gay's dtoa generalized to an arbitrary base, so this is a faithful port
// of that shared algorithm and the output matches those runtimes byte for byte.

// radixDigits indexes a digit value 0..35 to its character, the lowercase form
// JavaScript uses for the letter digits above 9.
const radixDigits = "0123456789abcdefghijklmnopqrstuvwxyz"

// NumberToStringRadix returns the JavaScript n.toString(radix) of a number in the
// given radix. The radix must be in 2..36, which the caller guarantees by only
// lowering a literal radix in range; a radix of 10 is delegated to NumberToString
// so the decimal path stays the single source of the base-ten form. The
// non-finite and zero cases match Number::toString, and a finite nonzero value is
// converted through the shared dtoa-in-base algorithm.
func NumberToStringRadix(x float64, radix int) BStr {
	if radix == 10 {
		return NumberToString(x)
	}
	switch {
	case math.IsNaN(x):
		return FromGoString("NaN")
	case x == 0:
		return FromGoString("0")
	case math.IsInf(x, 1):
		return FromGoString("Infinity")
	case math.IsInf(x, -1):
		return FromGoString("-Infinity")
	}
	// Fast path for a whole value of magnitude at most 2^53. Such a value has no
	// fractional part, so the general dtoa-in-base path skips its fraction loop and
	// only runs the integer digits, which for a positional radix are exactly what
	// strconv.AppendInt writes: the same digit alphabet (radixDigits is lowercase,
	// matching AppendInt for base 2..36) and the same leading '-'. Taking this path
	// avoids the 2200-byte stack buffer doubleToRadix zeroes on every call, the cost
	// that dominated stringifying a loop counter or a byte value in a radix. The
	// bound is 2^53 for the same reason NumberToString uses it: past that an integer
	// float and its exact int64 diverge, and doubleToRadix fills the unrepresented
	// low digits with zero, which AppendInt of the int64 would not reproduce.
	if x == math.Trunc(x) && x >= -twoPow53 && x <= twoPow53 {
		var ibuf [24]byte
		return fromASCII(string(strconv.AppendInt(ibuf[:0], int64(x), radix)))
	}
	return FromGoString(doubleToRadix(x, radix))
}

// doubleToRadix converts a finite nonzero double to its representation in the
// given radix, the port of V8's DoubleToRadixCString. It writes into the middle
// of a buffer, growing the integer part to the left and the fractional part to
// the right, so the two cursors bracket the finished string with no reversal. The
// fractional loop emits digits only to the precision the double carries, tracked
// by delta (half the gap to the next representable double, scaled by the radix
// each step), and rounds to even with a carry that can propagate back through the
// written fraction digits and into the integer part.
func doubleToRadix(value float64, radix int) string {
	const bufSize = 2200
	// The buffer is a stack array, not a make'd slice: the only value that escapes
	// this function is the small result string, which string(buf[lo:hi]) copies out,
	// so the array itself never leaves the frame and needs no heap allocation. A
	// heap buffer here cost one 2200-byte allocation on every call, which dominated
	// the radix path when a program stringifies many small integers in a loop.
	var buf [bufSize]byte
	intCursor := bufSize / 2
	fracCursor := intCursor

	negative := value < 0
	if negative {
		value = -value
	}

	integer := math.Floor(value)
	fraction := value - integer
	delta := 0.5 * (math.Nextafter(value, math.Inf(1)) - value)
	if smallest := math.Nextafter(0, math.Inf(1)); smallest > delta {
		delta = smallest
	}

	if fraction >= delta {
		buf[fracCursor] = '.'
		fracCursor++
		for {
			// Shift up by one digit, then write it.
			fraction *= float64(radix)
			delta *= float64(radix)
			digit := int(fraction)
			buf[fracCursor] = radixDigits[digit]
			fracCursor++
			fraction -= float64(digit)
			// Round to even, carrying back through the written digits when the
			// rounded-up fraction plus its uncertainty crosses one.
			if fraction > 0.5 || (fraction == 0.5 && digit&1 == 1) {
				if fraction+delta > 1 {
					for {
						fracCursor--
						if fracCursor == bufSize/2 {
							// The carry ran off the front of the fraction, past
							// the decimal point, so it lands on the integer part.
							integer += 1
							break
						}
						c := buf[fracCursor]
						var d int
						if c > '9' {
							d = int(c-'a') + 10
						} else {
							d = int(c - '0')
						}
						if d+1 < radix {
							buf[fracCursor] = radixDigits[d+1]
							fracCursor++
							break
						}
					}
					break
				}
			}
			if fraction < delta {
				break
			}
		}
	}

	// Integer digits. Once the integer part exceeds the double's 53-bit precision,
	// its low digits are not represented, so they are filled with zero.
	for radixExponent(integer/float64(radix)) > 0 {
		integer /= float64(radix)
		intCursor--
		buf[intCursor] = '0'
	}
	for {
		remainder := math.Mod(integer, float64(radix))
		intCursor--
		buf[intCursor] = radixDigits[int(remainder)]
		integer = (integer - remainder) / float64(radix)
		if integer <= 0 {
			break
		}
	}

	if negative {
		intCursor--
		buf[intCursor] = '-'
	}
	return string(buf[intCursor:fracCursor])
}

// radixExponent returns the binary exponent of d when its significand is
// normalized to the 53-bit integer range [2^52, 2^53), matching the value V8's
// Double::Exponent reports. It is positive exactly when d is at least 2^53, the
// point past which consecutive integers are no longer all representable, which is
// the test doubleToRadix uses to fill unrepresented integer digits with zero.
func radixExponent(d float64) int {
	if d == 0 {
		return -1 << 30 // no exponent; keep the fill loop from running
	}
	_, exp := math.Frexp(d) // d = frac * 2^exp, frac in [0.5, 1)
	return exp - 53
}
