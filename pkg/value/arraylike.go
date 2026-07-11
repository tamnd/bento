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

import (
	"math"
	"strings"
)

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

// arrayLikeHas reports whether the receiver carries an own or inherited property at
// index k, the HasProperty probe the hole-skipping methods make before visiting an
// index. It dispatches through HasProperty, so a real array answers false for a hole
// and an array-like object answers by whether it carries the integer key, each the
// presence the spec's kPresent step tests.
func arrayLikeHas(recv Value, k int) bool {
	return recv.HasProperty(NumberToString(float64(k)))
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
		if arrayLikeHas(recv, k) && StrictEquals(arrayLikeGet(recv, k), target) {
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
		if arrayLikeHas(recv, k) && StrictEquals(arrayLikeGet(recv, k), target) {
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

// GenericJoin runs Array.prototype.join on a generic receiver, concatenating the
// string form of each element with a separator between them. The separator defaults
// to a comma and an explicit undefined takes that default too, any other separator
// coercing through ToString. A hole reads as undefined and undefined and null each
// contribute the empty string, so join treats a hole as undefined the way the spec
// does. The result boxes to a string.
func GenericJoin(recv Value, sep ...Value) Value {
	n := arrayLikeLen(recv)
	separator := ","
	if len(sep) > 0 && sep[0].kind != KindUndefined {
		separator = ToString(sep[0]).ToGoString()
	}
	var b strings.Builder
	for k := 0; k < n; k++ {
		if k > 0 {
			b.WriteString(separator)
		}
		elem := arrayLikeGet(recv, k)
		if elem.kind == KindUndefined || elem.kind == KindNull {
			continue // undefined and null, and so a hole, join as the empty string
		}
		b.WriteString(ToString(elem).ToGoString())
	}
	return StringValue(FromGoString(b.String()))
}

// GenericCopyWithin runs Array.prototype.copyWithin on a generic receiver, copying
// the block of elements starting at from into the positions starting at to, both
// relative indices that count from the end when negative, and returning the receiver.
// The copy runs backward when the ranges overlap so a source is read before it is
// overwritten. A hole in the source stays a hole: rather than writing undefined, the
// target index is deleted, matching the spec's DeletePropertyOrThrow on a missing
// source.
func GenericCopyWithin(recv Value, bounds ...Value) Value {
	n := arrayLikeLen(recv)
	to, from, final := 0, 0, n
	if len(bounds) > 0 {
		to = relIndex(toIntegerValue(bounds[0]), n)
	}
	if len(bounds) > 1 {
		from = relIndex(toIntegerValue(bounds[1]), n)
	}
	if len(bounds) > 2 {
		final = relIndex(toIntegerValue(bounds[2]), n)
	}
	count := final - from
	if n-to < count {
		count = n - to
	}
	dir := 1
	if from < to && to < from+count {
		dir = -1
		from += count - 1
		to += count - 1
	}
	for ; count > 0; count-- {
		if arrayLikeHas(recv, from) {
			arrayLikeSet(recv, to, arrayLikeGet(recv, from))
		} else {
			recv.DeleteIndex(float64(to))
		}
		from += dir
		to += dir
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

// callBack invokes a boxed callback with the three arguments the array iteration
// methods pass, the element, its index as a number, and the receiver object, the
// (value, index, object) signature the spec calls the callback with. A thisArg, if
// the borrow supplied one, is dropped: bento's functions take no this, so a callback
// that reads this hands back when it is lowered, and one that does not is unaffected.
func callBack(cb, recv, elem Value, k int) Value {
	return cb.Call(elem, Number(float64(k)), recv)
}

// GenericForEach runs Array.prototype.forEach on a generic receiver, calling the
// callback with each element, its index, and the receiver, and returning undefined.
func GenericForEach(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue // a hole is skipped, the callback never sees it
		}
		callBack(cb, recv, arrayLikeGet(recv, k), k)
	}
	return Undefined
}

// GenericSlice runs Array.prototype.slice on a generic receiver, returning a new
// array of the elements in the half-open range [start, end), read as the properties
// named by their indices. start and end are relative indices: a negative bound
// counts from the end and clamps at 0, a positive bound clamps at the length, an
// omitted start is 0 and an omitted end is the length. The result is a real array
// whatever the receiver's kind, so a borrowed slice on an array-like still yields an
// array. A hole in the range stays a hole in the result rather than materializing as
// a stored undefined, matching the spec's HasProperty guard on each copied index.
func GenericSlice(recv Value, bounds ...Value) Value {
	n := arrayLikeLen(recv)
	start, end := 0, n
	if len(bounds) > 0 {
		start = relIndex(toIntegerValue(bounds[0]), n)
	}
	if len(bounds) > 1 {
		end = relIndex(toIntegerValue(bounds[1]), n)
	}
	out := []Value{}
	for k := start; k < end; k++ {
		if !arrayLikeHas(recv, k) {
			out = append(out, hole)
			continue
		}
		out = append(out, arrayLikeGet(recv, k))
	}
	return NewArrayValue(out)
}

// GenericConcat runs Array.prototype.concat on a generic receiver, returning a new
// array of the receiver's elements followed by each argument's. A spreadable
// argument, an array, contributes its elements one by one; any other argument is
// appended whole, the way concat folds a non-array into a single slot. A hole in the
// receiver or in a spreadable argument stays a hole in the result, so concat carries
// holes across rather than filling them with undefined.
func GenericConcat(recv Value, items ...Value) Value {
	out := []Value{}
	appendFrom := func(src Value) {
		if src.kind == KindArray {
			n := arrayLikeLen(src)
			for k := 0; k < n; k++ {
				if !arrayLikeHas(src, k) {
					out = append(out, hole)
					continue
				}
				out = append(out, arrayLikeGet(src, k))
			}
			return
		}
		out = append(out, src)
	}
	appendFrom(recv)
	for _, it := range items {
		appendFrom(it)
	}
	return NewArrayValue(out)
}

// GenericMap runs Array.prototype.map on a generic receiver, returning a new array
// whose element k is the callback's result on element k. The result is a real array
// whatever the receiver's kind, so a borrowed map on an array-like still yields an
// array.
func GenericMap(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	out := make([]Value, n)
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			out[k] = hole // a hole is skipped and left a hole in the result
			continue
		}
		out[k] = callBack(cb, recv, arrayLikeGet(recv, k), k)
	}
	return NewArrayValue(out)
}

// GenericFilter runs Array.prototype.filter on a generic receiver, returning a new
// array of the elements for which the callback's result is truthy, in order.
func GenericFilter(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	out := []Value{}
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue // a hole is skipped, never tested and never kept
		}
		elem := arrayLikeGet(recv, k)
		if ToBoolean(callBack(cb, recv, elem, k)) {
			out = append(out, elem)
		}
	}
	return NewArrayValue(out)
}

// GenericSome runs Array.prototype.some on a generic receiver, reporting whether the
// callback's result is truthy for any element, stopping at the first that is.
func GenericSome(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue // a hole is skipped, never tested
		}
		if ToBoolean(callBack(cb, recv, arrayLikeGet(recv, k), k)) {
			return Bool(true)
		}
	}
	return Bool(false)
}

// GenericEvery runs Array.prototype.every on a generic receiver, reporting whether
// the callback's result is truthy for every element, stopping at the first that is
// not.
func GenericEvery(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue // a hole is skipped, never tested
		}
		if !ToBoolean(callBack(cb, recv, arrayLikeGet(recv, k), k)) {
			return Bool(false)
		}
	}
	return Bool(true)
}

// GenericFind runs Array.prototype.find on a generic receiver, returning the first
// element for which the callback's result is truthy, or undefined when none is.
// Unlike the hole-skipping methods, find visits a hole as undefined, so the callback
// is called for every index in range.
func GenericFind(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		elem := arrayLikeGet(recv, k)
		if ToBoolean(callBack(cb, recv, elem, k)) {
			return elem
		}
	}
	return Undefined
}

// GenericFindIndex runs Array.prototype.findIndex on a generic receiver, returning
// the index of the first element for which the callback's result is truthy, or -1.
func GenericFindIndex(recv, cb Value, thisArg ...Value) Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		if ToBoolean(callBack(cb, recv, arrayLikeGet(recv, k), k)) {
			return Number(float64(k))
		}
	}
	return Number(-1)
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

// GenericReduce runs Array.prototype.reduce on a generic receiver, folding the
// present elements left to right into a single accumulator. With an initial value
// the fold seeds from it and runs over every present index; with no initial value
// it seeds from the first present element and runs from the next, so an all-hole or
// empty receiver with no initial value throws a TypeError the way the array method
// does. A hole is skipped, never visited, matching the spec's kPresent guard. The
// callback takes the four arguments the spec passes, the accumulator, the element,
// its index as a number, and the receiver.
func GenericReduce(recv, cb Value, initial ...Value) Value {
	n := arrayLikeLen(recv)
	k := 0
	var acc Value
	if len(initial) > 0 {
		acc = initial[0]
	} else {
		for k < n && !arrayLikeHas(recv, k) {
			k++
		}
		if k >= n {
			Throw(NewTypeError(FromGoString("Reduce of empty array with no initial value")))
		}
		acc = arrayLikeGet(recv, k)
		k++
	}
	for ; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue
		}
		acc = cb.Call(acc, arrayLikeGet(recv, k), Number(float64(k)), recv)
	}
	return acc
}

// GenericReduceRight runs Array.prototype.reduceRight on a generic receiver, folding
// the present elements right to left into a single accumulator. It mirrors
// GenericReduce: with an initial value the fold seeds from it, otherwise it seeds
// from the last present element and an all-hole or empty receiver with no initial
// value throws a TypeError. A hole is skipped, and the callback takes the
// accumulator, the element, its index, and the receiver.
func GenericReduceRight(recv, cb Value, initial ...Value) Value {
	n := arrayLikeLen(recv)
	k := n - 1
	var acc Value
	if len(initial) > 0 {
		acc = initial[0]
	} else {
		for k >= 0 && !arrayLikeHas(recv, k) {
			k--
		}
		if k < 0 {
			Throw(NewTypeError(FromGoString("Reduce of empty array with no initial value")))
		}
		acc = arrayLikeGet(recv, k)
		k--
	}
	for ; k >= 0; k-- {
		if !arrayLikeHas(recv, k) {
			continue
		}
		acc = cb.Call(acc, arrayLikeGet(recv, k), Number(float64(k)), recv)
	}
	return acc
}
