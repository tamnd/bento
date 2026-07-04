package value

import "strings"

// Array is bento's runtime representation of a JavaScript array whose element
// type the compiler proved, the *Array[T] header that 05_type_lowering.md
// section 11 names. It wraps a Go backing slice and adds the JavaScript array
// semantics a bare []T lacks.
//
// This slice implements the dense core the lowerer needs first: construction
// from an array literal, the .length as a Number, and in-order iteration for
// for...of. The sparse edges (a length that outruns the backing store, holes,
// and index writes past the end) and the mutating and higher-order methods
// (push, pop, map, filter, slice) land in later slices. The type is introduced
// now so those can grow it without changing how an array is spelled in
// generated code: an array is always a *Array[T], today and after the methods
// arrive.
type Array[T any] struct {
	elems []T
}

// NewArray builds an array from its elements, the lowering of an array literal
// [e0, e1, ...]. The elements are copied into a fresh backing slice so the
// header owns its storage and a later push through one array cannot alias a
// caller's variadic argument array or a slice the literal was built from.
func NewArray[T any](elems ...T) *Array[T] {
	backing := make([]T, len(elems))
	copy(backing, elems)
	return &Array[T]{elems: backing}
}

// Len is the array's length. JavaScript's .length is a Number, so it is a
// float64 here to match the type the checker gives the property and to compose
// with the rest of the numeric path with no conversion at the use site. It is
// the element count while arrays stay dense; the sparse case, where length can
// exceed the backing store, is a later slice.
func (a *Array[T]) Len() float64 { return float64(len(a.elems)) }

// Elems returns the backing slice for in-order iteration, the lowering target
// of for...of. It is the live backing store, not a copy, which matches the
// array iterator visiting the elements in place. A push during iteration is
// visible through the same slice header the range captured at loop entry, which
// matches the array iterator reading up to the current length; the sparse
// grow-and-shrink edges are still a later slice.
func (a *Array[T]) Elems() []T { return a.elems }

// At reads the element a JavaScript index expression a[i] selects. The index is
// a Number, so it is a float64 here to match the type the checker gives the
// argument expression and to take the result of the bitwise and arithmetic path
// with no conversion at the call site. JavaScript truncates the index toward
// zero, and an index outside the array reads as the absent element. The element
// type is not optional, because the checker types a[i] as T under its default
// index signature rather than T | undefined, so an out-of-range read yields the
// zero value of T. That is a faithful lowering of the covered subset, where the
// programs that index an array do so within its bounds; the noUncheckedIndexed
// -Access shape, which types the read as T | undefined and needs an Opt result,
// is a later slice.
func (a *Array[T]) At(i float64) T {
	idx := int(i) // JavaScript ToInteger truncates toward zero.
	if i != i {   // NaN truncates to 0, matching ToIntegerOrInfinity.
		idx = 0
	}
	if idx >= 0 && idx < len(a.elems) {
		return a.elems[idx]
	}
	var zero T
	return zero
}

// Push appends its arguments to the end of the array and returns the new length
// as a Number, matching JavaScript's Array.prototype.push. It is a pointer
// method so the append is visible through every reference to the array, which is
// what a mutation on a shared array must be; a const binding in the source is no
// obstacle, because const freezes the binding, not the array's contents.
func (a *Array[T]) Push(xs ...T) float64 {
	a.elems = append(a.elems, xs...)
	return float64(len(a.elems))
}

// Map returns a new array holding f applied to each element in order, the
// lowering of Array.prototype.map. This is the same-element-type form, where the
// callback returns the element type; a map that changes the element type needs a
// free generic function, a later slice, because a Go method cannot introduce a
// new type parameter for the result. The callback here takes only the element,
// which is the common shape; the index and array parameters JavaScript also
// passes are a later slice. The result is a fresh array, so the receiver is
// unchanged.
func (a *Array[T]) Map(f func(T) T) *Array[T] {
	out := make([]T, len(a.elems))
	for i, x := range a.elems {
		out[i] = f(x)
	}
	return &Array[T]{elems: out}
}

// MapArray is the type-changing form of Map: it builds a fresh Array[U] by
// applying f to each element of a, the lowering of Array.prototype.map when the
// callback returns a different type than the element (number[].map(n =>
// n.toString()) is string[]). Map cannot express this because a Go method may
// not introduce a new type parameter, so the lowerer emits this free function
// with both type arguments spelled out whenever the callback's result type does
// not match the element type. As with Map the callback takes only the element,
// and the receiver is unchanged since the result is a new array.
func MapArray[T, U any](a *Array[T], f func(T) U) *Array[U] {
	out := make([]U, len(a.elems))
	for i, x := range a.elems {
		out[i] = f(x)
	}
	return &Array[U]{elems: out}
}

// Filter returns a new array of the elements for which f returns true, in order,
// the lowering of Array.prototype.filter. As with Map, the callback takes only
// the element for now. The result is a fresh array, so the receiver is
// unchanged.
func (a *Array[T]) Filter(f func(T) bool) *Array[T] {
	out := make([]T, 0, len(a.elems))
	for _, x := range a.elems {
		if f(x) {
			out = append(out, x)
		}
	}
	return &Array[T]{elems: out}
}

// Slice returns a shallow copy of a portion of the array into a new array, the
// lowering of Array.prototype.slice. It takes zero, one, or two Number bounds,
// matching the source call, since JavaScript's slice has both arguments
// optional: slice() copies the whole array, slice(start) runs to the end, and
// slice(start, end) is the half-open range. A bound is read exactly as
// JavaScript specifies, through relativeIndex: it truncates toward zero, a
// negative bound counts from the end, and the result is clamped into range, so
// an out-of-range or crossed pair yields an empty array rather than a panic. The
// result is a fresh array, so the receiver is unchanged.
func (a *Array[T]) Slice(bounds ...float64) *Array[T] {
	n := len(a.elems)
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
	out := make([]T, end-start)
	copy(out, a.elems[start:end])
	return &Array[T]{elems: out}
}

// Reverse reverses the elements in place and returns the same array, the
// lowering of Array.prototype.reverse. It is a pointer method because the
// reversal mutates the receiver, and it returns the receiver rather than a copy
// so that a.reverse() === a holds, matching JavaScript, where reverse returns a
// reference to the same array it reordered. An empty or single-element array is
// unchanged.
func (a *Array[T]) Reverse() *Array[T] {
	for i, j := 0, len(a.elems)-1; i < j; i, j = i+1, j-1 {
		a.elems[i], a.elems[j] = a.elems[j], a.elems[i]
	}
	return a
}

// IndexOf returns the index of the first element equal to target, or -1 if none
// is, the lowering of Array.prototype.indexOf. Equality is supplied by the
// caller through eq rather than fixed here, because a Go method cannot compare
// two values of the type parameter T (it is any, not comparable) and because the
// exact equality JavaScript uses is element-type-specific: the lowerer passes
// strict equality for indexOf, which for a number is Go ==, so a NaN target is
// never found, matching indexOf's use of the strict equality operator. The
// result is a Number, so it is a float64. The optional fromIndex argument is a
// later slice; this is the whole-array scan.
func (a *Array[T]) IndexOf(target T, eq func(T, T) bool) float64 {
	for i, x := range a.elems {
		if eq(x, target) {
			return float64(i)
		}
	}
	return -1
}

// Includes reports whether any element equals target, the lowering of
// Array.prototype.includes. It is IndexOf against the same target, so it shares
// the linear scan; the difference between the two methods is entirely in the eq
// the lowerer passes. includes uses SameValueZero, which unlike strict equality
// treats NaN as equal to NaN, so the lowerer passes a NaN-aware eq for a number
// element here while it passes strict equality for IndexOf. That is why a NaN is
// found by includes but not by indexOf, matching JavaScript.
func (a *Array[T]) Includes(target T, eq func(T, T) bool) bool {
	return a.IndexOf(target, eq) >= 0
}

// Join concatenates the elements into a string separated by sep, the lowering of
// Array.prototype.join. Each element becomes a string through str, supplied by
// the caller for the same reason the search methods take an equality: a Go
// method cannot run the element-type-specific ToString on its type parameter, so
// the lowerer, which knows the element type, passes NumberToString, BoolToString,
// or the identity for a string. An empty array joins to the empty string, and a
// single element to itself with no separator, matching JavaScript.
func (a *Array[T]) Join(sep BStr, str func(T) BStr) BStr {
	if len(a.elems) == 0 {
		return FromGoString("")
	}
	// Accumulate into one buffer in a single pass, so a join of n elements is O(n)
	// total rather than the O(n squared) a fold of pairwise Concat would cost by
	// copying the growing result at every step. The UTF-8 fast path stays on bytes
	// through a strings.Builder while every piece and the separator are valid UTF-8,
	// which is the common case; the first piece that carries a raw code-unit backing
	// falls through to the code-unit builder so a lone surrogate survives, matching
	// how Concat picks its backing.
	sep = sep.flat()
	if sep.utf16 == nil {
		var b strings.Builder
		lengthU16 := 0
		utf8Only := true
		for i := range a.elems {
			es := str(a.elems[i]).flat()
			if es.utf16 != nil {
				utf8Only = false
				break
			}
			if i > 0 {
				b.WriteString(sep.utf8)
				lengthU16 += sep.lengthU16
			}
			b.WriteString(es.utf8)
			lengthU16 += es.lengthU16
		}
		if utf8Only {
			return BStr{utf8: b.String(), lengthU16: lengthU16}
		}
	}
	// Code-unit builder: correct for any backing, including a separator or a piece
	// that holds a lone surrogate. str is a pure coercion, so re-running it here
	// after the fast path bailed costs only the rare surrogate case.
	var units []uint16
	for i := range a.elems {
		if i > 0 {
			units = sep.appendUnits(units)
		}
		units = str(a.elems[i]).appendUnits(units)
	}
	return BStr{utf16: units, lengthU16: len(units)}
}

// Pop removes the last element and returns it, the lowering of
// Array.prototype.pop. Its declared type is T | undefined, so it returns an
// Opt[T]: a present optional holding the removed element on a non-empty array,
// and the undefined optional on an empty array, where JavaScript pop returns
// undefined and leaves the array empty. It is a pointer method because the
// removal must be visible through every reference to the array, the same reason
// Push is. The backing slice is reshortened by one, so the popped slot is no
// longer part of the array.
func (a *Array[T]) Pop() Opt[T] {
	if len(a.elems) == 0 {
		return None[T]()
	}
	last := a.elems[len(a.elems)-1]
	a.elems = a.elems[:len(a.elems)-1]
	return Some(last)
}

// relativeIndex resolves a JavaScript slice bound against a length: it truncates
// the Number toward zero, treats a negative value as counting back from the end,
// and clamps the result into [0, length]. This is the shared step
// Array.prototype.slice applies to each of its bounds. NaN truncates to zero,
// matching ToIntegerOrInfinity, so a NaN bound behaves as 0.
func relativeIndex(v float64, length int) int {
	i := int(v)
	if v != v { // NaN truncates to 0
		i = 0
	}
	if i < 0 {
		i += length
		if i < 0 {
			return 0
		}
		return i
	}
	if i > length {
		return length
	}
	return i
}
