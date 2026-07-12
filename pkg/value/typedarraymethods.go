package value

import "strings"

// The copy and search methods over a typed array run against the view's element
// slice and clamp to its length, the same shape the Array methods take but with
// every element widened to the Number a typed-array read hands out. A method that
// builds a new array allocates a fresh buffer and owns it (slice), while a method
// that makes a new view shares the receiver's buffer (subarray); the in-place
// methods (fill, copyWithin, set) write straight through the view so the change
// shows through every other view of the same buffer. The bounds go through
// relativeIndex, the same step Array.prototype.slice uses, so a negative bound
// counts from the end and an out-of-range or crossed pair yields an empty range
// rather than a panic.

// Fill overwrites a range of the view with a single coerced value in place and
// returns the receiver, the lowering of TypedArray.prototype.fill. The value is
// run through the element kind's store coercion exactly as an indexed write would
// be, so a value outside the element's range wraps or clamps. The optional start
// and end bounds go through relativeIndex, so fill(v) fills the whole view,
// fill(v, start) runs to the end, and fill(v, start, end) is the half-open range.
func (a *TypedArray[T]) Fill(v float64, bounds ...float64) *TypedArray[T] {
	n := len(a.data)
	start := 0
	end := n
	if len(bounds) >= 1 {
		start = relativeIndex(bounds[0], n)
	}
	if len(bounds) >= 2 {
		end = relativeIndex(bounds[1], n)
	}
	cv := a.coerce(v)
	for i := start; i < end; i++ {
		a.data[i] = cv
	}
	return a
}

// Slice returns a fresh typed array holding a copy of a range of the view, the
// lowering of TypedArray.prototype.slice. Unlike subarray, which makes a new view
// over the same buffer, slice allocates a new buffer and copies the elements into
// it, so the result owns its storage and a later write to either array does not
// show through the other. The bounds go through relativeIndex, so slice() copies
// the whole view, slice(start) runs to the end, and a crossed pair yields an empty
// array. The copy is a plain slice copy, since both arrays hold the same element
// type.
func (a *TypedArray[T]) Slice(bounds ...float64) *TypedArray[T] {
	n := len(a.data)
	start := 0
	end := n
	if len(bounds) >= 1 {
		start = relativeIndex(bounds[0], n)
	}
	if len(bounds) >= 2 {
		end = relativeIndex(bounds[1], n)
	}
	if end < start {
		end = start
	}
	out := newTypedArray(float64(end-start), a.coerce)
	copy(out.data, a.data[start:end])
	return out
}

// Subarray returns a new typed array that views the same buffer over a range of
// the receiver, the lowering of TypedArray.prototype.subarray. It shares the
// bytes: a write through the subarray shows through the receiver and every other
// view of the buffer, which is the difference from slice, whose result owns a
// fresh copy. The byte offset of the new view is the receiver's offset advanced by
// the start element, so the two views alias the same run of bytes. The bounds go
// through relativeIndex, so subarray() views the whole range, subarray(start) runs
// to the end, and a crossed pair yields an empty view.
func (a *TypedArray[T]) Subarray(bounds ...float64) *TypedArray[T] {
	n := len(a.data)
	start := 0
	end := n
	if len(bounds) >= 1 {
		start = relativeIndex(bounds[0], n)
	}
	if len(bounds) >= 2 {
		end = relativeIndex(bounds[1], n)
	}
	if end < start {
		end = start
	}
	byteOffset := a.byteOffset + start*elemBytes[T]()
	return newTypedArrayView(a.buffer, byteOffset, end-start, a.coerce)
}

// CopyWithin copies a block of the view to another position within the same view
// in place and returns the receiver, the lowering of
// TypedArray.prototype.copyWithin. The target and the optional start and end
// bounds go through relativeIndex, and the count is capped so the copy neither
// runs past the source range nor writes past the end of the view, matching
// copyWithin, which never changes the length. The copy uses Go's builtin copy,
// whose memmove semantics reproduce copyWithin's overlap behavior of reading the
// source range as if to a temporary before writing, so an overlapping copy is
// correct. The elements are already stored, so no coercion runs, unlike fill.
func (a *TypedArray[T]) CopyWithin(bounds ...float64) *TypedArray[T] {
	n := len(a.data)
	target := 0
	start := 0
	end := n
	if len(bounds) >= 1 {
		target = relativeIndex(bounds[0], n)
	}
	if len(bounds) >= 2 {
		start = relativeIndex(bounds[1], n)
	}
	if len(bounds) >= 3 {
		end = relativeIndex(bounds[2], n)
	}
	count := end - start
	if rem := n - target; rem < count {
		count = rem
	}
	if count > 0 {
		copy(a.data[target:target+count], a.data[start:start+count])
	}
	return a
}

// Set copies the elements of a source list into the view starting at offset,
// coercing each with the element kind's store rule, the lowering of
// TypedArray.prototype.set. The source arrives as a []float64 snapshot: the
// lowerer reads a typed-array source through Floats and an array source through its
// elements, so by the time control reaches here the source is a plain slice that
// aliases neither the receiver's buffer nor the caller's array. Reading the whole
// source before the first write is what makes an overlapping set from another view
// of the same buffer correct, since the source values are captured before any of
// them is overwritten. A negative offset or a source that would run past the end
// of the view throws a RangeError, matching set, which validates the bounds before
// it writes any element.
func (a *TypedArray[T]) Set(src []float64, offset float64) {
	off := int(offset)
	if offset != offset { // NaN truncates to 0.
		off = 0
	}
	if off < 0 {
		Throw(NewRangeError(FromGoString("offset is out of bounds")))
	}
	if off+len(src) > len(a.data) {
		Throw(NewRangeError(FromGoString("source array is too long")))
	}
	for i, v := range src {
		a.data[off+i] = a.coerce(v)
	}
}

// IndexOf returns the index of the first element strictly equal to target, or -1
// if none is, the lowering of TypedArray.prototype.indexOf. Every element is
// widened to a Number and compared with Go ==, which is the strict equality
// indexOf uses, so a NaN target is never found and a +0 target matches a stored
// -0. The result is a Number, so it is a float64. The optional fromIndex argument
// is a later slice; this is the whole-view scan.
func (a *TypedArray[T]) IndexOf(target float64) float64 {
	for i := range a.data {
		if float64(a.data[i]) == target {
			return float64(i)
		}
	}
	return -1
}

// LastIndexOf returns the index of the last element strictly equal to target, or
// -1 if none is, the lowering of TypedArray.prototype.lastIndexOf. It is IndexOf
// scanning from the end, and like indexOf it uses strict equality, so a NaN target
// is never found. The result is a Number. The optional fromIndex argument is a
// later slice; this is the whole-view scan.
func (a *TypedArray[T]) LastIndexOf(target float64) float64 {
	for i := len(a.data) - 1; i >= 0; i-- {
		if float64(a.data[i]) == target {
			return float64(i)
		}
	}
	return -1
}

// Includes reports whether any element equals target under SameValueZero, the
// lowering of TypedArray.prototype.includes. It differs from indexOf only in that
// SameValueZero treats NaN as equal to NaN, so a Float32Array or Float64Array
// holding a NaN reports true for a NaN target where indexOf would not. The +0 and
// -0 pair is equal under both, which Go == already gives.
func (a *TypedArray[T]) Includes(target float64) bool {
	targetNaN := target != target
	for i := range a.data {
		e := float64(a.data[i])
		if e == target || (targetNaN && e != e) {
			return true
		}
	}
	return false
}

// AtOpt reads the element TypedArray.prototype.at selects, the relative-index read
// that counts from the end when the index is negative, the lowering of
// TypedArray.prototype.at. Its declared type is Number | undefined, so it returns
// an Opt[float64], a present optional in range and the undefined optional outside
// it. The index truncates toward zero with NaN becoming zero, and a negative index
// adds the length once, so at(-1) is the last element; an index still out of range
// after that yields undefined.
func (a *TypedArray[T]) AtOpt(i float64) Opt[float64] {
	idx := int(i)
	if i != i { // NaN truncates to 0.
		idx = 0
	}
	if idx < 0 {
		idx += len(a.data)
	}
	if idx >= 0 && idx < len(a.data) {
		return Some(float64(a.data[idx]))
	}
	return None[float64]()
}

// Join concatenates the elements into a string separated by sep, the lowering of
// TypedArray.prototype.join. Each element becomes a string through NumberToString,
// the ToString a Number takes, so unlike the Array Join no per-element stringify
// closure is threaded in: a typed array's element is always a Number. An empty
// view joins to the empty string, and a single element to itself with no
// separator, matching JavaScript. The UTF-8 fast path stays on bytes through a
// strings.Builder while the separator is valid UTF-8, which every NumberToString
// piece always is; a separator that carries a raw code-unit backing falls to the
// code-unit builder so a lone surrogate in it survives.
func (a *TypedArray[T]) Join(sep BStr) BStr {
	if len(a.data) == 0 {
		return FromGoString("")
	}
	sep = sep.flat()
	if sep.utf16 == nil {
		var b strings.Builder
		lengthU16 := 0
		for i := range a.data {
			if i > 0 {
				b.WriteString(sep.utf8)
				lengthU16 += sep.lengthU16
			}
			es := NumberToString(float64(a.data[i])).flat()
			b.WriteString(es.utf8)
			lengthU16 += es.lengthU16
		}
		return BStr{utf8: b.String(), lengthU16: lengthU16}
	}
	var units []uint16
	for i := range a.data {
		if i > 0 {
			units = sep.appendUnits(units)
		}
		units = NumberToString(float64(a.data[i])).appendUnits(units)
	}
	return BStr{utf16: units, lengthU16: len(units)}
}
