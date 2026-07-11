// This file owns the generic-receiver Array.prototype methods (08 group 1): the
// forms test262 borrows with Array.prototype.<m>.call(arrayLike, ...), where the
// receiver is a plain object carrying a length property and integer-keyed values
// rather than a real array. Each method reads length off the receiver, coerces it
// to a length the spec's way, reads each element as the property named by its index,
// and runs the same algorithm the array method runs on a dense slice. A real array
// receiver flows through the same reads, since an array answers a length and an
// integer key too, so a borrowed call works whether the receiver is array-like or
// a real array. The receiver arrives boxed so the reads dispatch by its kind.

package value

import "math"

// maxArrayLength is 2^53 - 1, the largest integer a length can hold, the cap
// ToLength applies so a fractional or over-large length property still yields a
// valid array length.
const maxArrayLength = 1<<53 - 1

// toLength coerces a value to an array length the way the spec's ToLength does:
// ToNumber then truncate toward zero, a NaN or non-positive result clamping to 0
// and an over-large result capping at 2^53 - 1. It is what a generic-receiver
// method applies to the length property before it walks the indices, so a
// fractional, negative, or huge length behaves the way the array methods assert.
func toLength(v Value) int {
	n := ToNumber(v)
	if math.IsNaN(n) || n <= 0 {
		return 0
	}
	if n >= maxArrayLength {
		return maxArrayLength
	}
	return int(n) // truncate toward zero, matching ToIntegerOrInfinity
}

// toIntegerValue coerces a value to an integer the way ToIntegerOrInfinity does,
// ToNumber then truncate toward zero with NaN becoming 0. It is what a method
// applies to a fromIndex argument, which unlike a length may be negative to count
// from the end.
func toIntegerValue(v Value) float64 {
	return toInteger(ToNumber(v))
}

// arrayLikeLen reads the receiver's length property and coerces it to a length, the
// first step every generic-receiver method takes.
func arrayLikeLen(recv Value) int {
	return toLength(recv.Get(FromGoString("length")))
}

// arrayLikeGet reads element k off the receiver as the property named by the integer
// k, the read a generic-receiver method makes at each index. It dispatches through
// Get, so a real array answers from its dense storage and an array-like object from
// the property named by the index string.
func arrayLikeGet(recv Value, k int) Value {
	return recv.GetIndex(float64(k))
}

// sameValueZero compares two values the way SameValueZero does, strict equality
// except that NaN equals NaN, the equality Array.prototype.includes uses so a hole
// or a stored NaN is found.
func sameValueZero(a, b Value) bool {
	if a.kind == KindNumber && b.kind == KindNumber {
		x, y := a.AsNumber(), b.AsNumber()
		if math.IsNaN(x) && math.IsNaN(y) {
			return true
		}
	}
	return StrictEquals(a, b)
}

// GenericIndexOf runs Array.prototype.indexOf on a generic receiver, returning the
// index of the first element strictly equal to target at or after fromIndex, or -1.
// A negative fromIndex counts from the end, the way the array method does. The
// result boxes to a number so a borrowed call yields a value whatever the receiver.
func GenericIndexOf(recv, target Value, from ...Value) Value {
	n := arrayLikeLen(recv)
	start := 0
	if len(from) > 0 {
		f := toIntegerValue(from[0])
		switch {
		case f >= float64(n):
			return Number(-1)
		case f >= 0:
			start = int(f)
		default:
			start = n + int(f)
			if start < 0 {
				start = 0
			}
		}
	}
	for k := start; k < n; k++ {
		if StrictEquals(arrayLikeGet(recv, k), target) {
			return Number(float64(k))
		}
	}
	return Number(-1)
}

// GenericLastIndexOf runs Array.prototype.lastIndexOf on a generic receiver,
// returning the index of the last element strictly equal to target at or before
// fromIndex, or -1. fromIndex defaults to the last index and a negative value counts
// from the end.
func GenericLastIndexOf(recv, target Value, from ...Value) Value {
	n := arrayLikeLen(recv)
	start := n - 1
	if len(from) > 0 {
		f := toIntegerValue(from[0])
		switch {
		case f >= 0:
			if int(f) < start {
				start = int(f)
			}
		default:
			start = n + int(f)
		}
	}
	for k := start; k >= 0; k-- {
		if StrictEquals(arrayLikeGet(recv, k), target) {
			return Number(float64(k))
		}
	}
	return Number(-1)
}

// arrayLikeSet writes element k onto the receiver as the property named by the
// integer k, the write a mutating generic-receiver method makes at each index. It
// dispatches through SetIndex, so a real array writes its dense storage and an
// array-like object writes the property named by the index string, not a slice
// slot.
func arrayLikeSet(recv Value, k int, val Value) {
	recv.SetIndex(float64(k), val)
}

// GenericFill runs Array.prototype.fill on a generic receiver, writing value into
// each index in the half-open range [start, end) and returning the receiver. start
// and end are relative indices: a negative bound counts from the end and clamps at
// 0, a positive bound clamps at the length, and an omitted start is 0 and an omitted
// end is the length. The receiver is returned so the borrowed call reads as the
// in-place fill the array method evaluates to.
func GenericFill(recv, value Value, bounds ...Value) Value {
	n := arrayLikeLen(recv)
	start, end := 0, n
	if len(bounds) > 0 {
		start = relIndex(toIntegerValue(bounds[0]), n)
	}
	if len(bounds) > 1 {
		end = relIndex(toIntegerValue(bounds[1]), n)
	}
	for k := start; k < end; k++ {
		arrayLikeSet(recv, k, value)
	}
	return recv
}

// GenericReverse runs Array.prototype.reverse on a generic receiver, swapping the
// element at each index with its mirror across the middle and returning the
// receiver. Each swap reads both elements as properties named by their indices and
// writes them back, so a real array and an array-like object both reverse in place.
func GenericReverse(recv Value) Value {
	n := arrayLikeLen(recv)
	for lower := 0; lower < n/2; lower++ {
		upper := n - 1 - lower
		lo := arrayLikeGet(recv, lower)
		hi := arrayLikeGet(recv, upper)
		arrayLikeSet(recv, lower, hi)
		arrayLikeSet(recv, upper, lo)
	}
	return recv
}

// GenericIncludes runs Array.prototype.includes on a generic receiver, reporting
// whether any element at or after fromIndex is SameValueZero-equal to target, so a
// stored NaN is found where indexOf would miss it. The result boxes to a boolean.
func GenericIncludes(recv, target Value, from ...Value) Value {
	n := arrayLikeLen(recv)
	start := 0
	if len(from) > 0 {
		f := toIntegerValue(from[0])
		switch {
		case f >= float64(n):
			return Bool(false)
		case f >= 0:
			start = int(f)
		default:
			start = n + int(f)
			if start < 0 {
				start = 0
			}
		}
	}
	for k := start; k < n; k++ {
		if sameValueZero(arrayLikeGet(recv, k), target) {
			return Bool(true)
		}
	}
	return Bool(false)
}
