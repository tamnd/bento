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
