package value

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
