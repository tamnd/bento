package value

import (
	"sort"
	"strings"
)

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

// ArrayFrom builds an array that takes ownership of an existing backing slice,
// the lowering target of an array literal that splices in a spread element. The
// spread lowering builds the backing slice with append, starting from a fresh
// []T so the result aliases none of the spread sources, then hands that slice
// here rather than re-copying it through NewArray's variadic. The array owns the
// slice from this point, which is sound because the caller built it for this one
// array and keeps no other reference to it.
func ArrayFrom[T any](elems []T) *Array[T] {
	return &Array[T]{elems: elems}
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

// AtOpt reads the element Array.prototype.at selects, the relative-index read
// that counts from the end when the index is negative. It is the sibling of At:
// At lowers the index expression a[i], whose default index signature types the
// read as T, so At returns a bare T with the zero value out of range; at is a
// method whose declared type is T | undefined, so AtOpt returns an Opt[T], a
// present optional in range and the undefined optional outside it. JavaScript
// truncates the index toward zero and adds the length once to a negative index,
// so at(-1) is the last element; an index still out of range after that, or a
// NaN that truncates to zero on an empty array, yields undefined. The receiver
// is unchanged, since at only reads.
func (a *Array[T]) AtOpt(i float64) Opt[T] {
	idx := int(i) // JavaScript ToInteger truncates toward zero.
	if i != i {   // NaN truncates to 0, matching ToIntegerOrInfinity.
		idx = 0
	}
	if idx < 0 {
		idx += len(a.elems)
	}
	if idx >= 0 && idx < len(a.elems) {
		return Some(a.elems[idx])
	}
	return None[T]()
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

// Reduce folds the array left to right into a single accumulator, the lowering
// of Array.prototype.reduce called with an initial value. It is a free function
// rather than a method because the accumulator type A may differ from the
// element type T (numbers.reduce((acc, n) => acc + String(n), "") is a string
// over a number array), and a Go method cannot introduce the new type parameter
// A the way this function's second type argument does. The callback takes the
// accumulator and the element, the two-parameter shape reduce needs; the index
// and array arguments JavaScript also passes are a later slice. Starting from
// init, each element updates the accumulator in order, and an empty array
// returns init unchanged, matching JavaScript's reduce with an initial value.
func Reduce[T, A any](a *Array[T], f func(A, T) A, init A) A {
	acc := init
	for _, x := range a.elems {
		acc = f(acc, x)
	}
	return acc
}

// ReduceNoInit folds the array left to right with no initial value, the lowering
// of Array.prototype.reduce called with only a callback. With no init the
// accumulator seeds from the first element and the fold runs from the second, so
// the accumulator type is the element type T and the callback is func(T, T) T,
// unlike the initial-value form whose accumulator may be a different type A. That
// same difference is why this is a method: the accumulator type does not vary from
// the element type here, so no second type parameter is needed. An empty array has
// no seed, so it throws a TypeError the way JavaScript does rather than inventing
// one.
func (a *Array[T]) ReduceNoInit(f func(T, T) T) T {
	if len(a.elems) == 0 {
		Throw(NewTypeError(FromGoString("Reduce of empty array with no initial value")))
	}
	acc := a.elems[0]
	for _, x := range a.elems[1:] {
		acc = f(acc, x)
	}
	return acc
}

// ReduceRight folds the array right to left into a single accumulator, the
// lowering of Array.prototype.reduceRight called with an initial value. Like
// Reduce it is a free function so the accumulator type A can differ from the
// element type T, which a method could not introduce. Starting from init, each
// element from the last to the first updates the accumulator, and an empty array
// returns init unchanged.
func ReduceRight[T, A any](a *Array[T], f func(A, T) A, init A) A {
	acc := init
	for i := len(a.elems) - 1; i >= 0; i-- {
		acc = f(acc, a.elems[i])
	}
	return acc
}

// ReduceRightNoInit folds the array right to left with no initial value, the
// lowering of Array.prototype.reduceRight called with only a callback. The
// accumulator seeds from the last element and the fold runs toward the first, so
// the accumulator type is the element type T and no second type parameter is
// needed, which is why this is a method. An empty array has no seed, so it throws
// a TypeError the way JavaScript does.
func (a *Array[T]) ReduceRightNoInit(f func(T, T) T) T {
	if len(a.elems) == 0 {
		Throw(NewTypeError(FromGoString("Reduce of empty array with no initial value")))
	}
	acc := a.elems[len(a.elems)-1]
	for i := len(a.elems) - 2; i >= 0; i-- {
		acc = f(acc, a.elems[i])
	}
	return acc
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

// Some reports whether at least one element satisfies the predicate, the
// lowering of Array.prototype.some. It short-circuits on the first element the
// callback accepts, matching JavaScript, and an empty array is false because no
// element passes. The callback here takes only the element, the common shape;
// the index and array arguments JavaScript also passes are a later slice.
func (a *Array[T]) Some(f func(T) bool) bool {
	for _, x := range a.elems {
		if f(x) {
			return true
		}
	}
	return false
}

// Every reports whether all elements satisfy the predicate, the lowering of
// Array.prototype.every. It short-circuits on the first element the callback
// rejects, matching JavaScript, and an empty array is true because no element
// fails, the vacuous case JavaScript also returns true for. The callback takes
// only the element; the index and array arguments are a later slice.
func (a *Array[T]) Every(f func(T) bool) bool {
	for _, x := range a.elems {
		if !f(x) {
			return false
		}
	}
	return true
}

// ForEach runs the callback for each element in order for its side effect, the
// lowering of Array.prototype.forEach. It returns nothing, matching the method's
// undefined result, so a call stands in the statement position. The callback
// takes only the element; the index and array arguments are a later slice, and
// forEach cannot be stopped early, matching JavaScript.
func (a *Array[T]) ForEach(f func(T)) {
	for _, x := range a.elems {
		f(x)
	}
}

// Find returns the first element the callback accepts, the lowering of
// Array.prototype.find. Its declared type is T | undefined, so it returns an
// Opt[T]: a present optional holding that element, or the undefined optional
// when no element passes, where JavaScript find returns undefined. It
// short-circuits on the first match, matching JavaScript. The callback takes
// only the element; the index and array arguments are a later slice.
func (a *Array[T]) Find(f func(T) bool) Opt[T] {
	for _, x := range a.elems {
		if f(x) {
			return Some(x)
		}
	}
	return None[T]()
}

// FindIndex returns the index of the first element the callback accepts, or -1
// when none does, the lowering of Array.prototype.findIndex. The result is a
// Number, so it is a float64, and -1 is the not-found sentinel JavaScript uses,
// so no optional is needed the way find's element result needs one. It
// short-circuits on the first match. The callback takes only the element; the
// index and array arguments are a later slice.
func (a *Array[T]) FindIndex(f func(T) bool) float64 {
	for i, x := range a.elems {
		if f(x) {
			return float64(i)
		}
	}
	return -1
}

// FindLast returns the last element the callback accepts, the lowering of
// Array.prototype.findLast. Like find its declared type is T | undefined, so it
// returns an Opt[T], present with the matching element or the undefined optional
// when none passes. It walks from the end and short-circuits on the first match,
// matching JavaScript, which visits indices in descending order. The callback
// takes only the element; the index and array arguments are a later slice.
func (a *Array[T]) FindLast(f func(T) bool) Opt[T] {
	for i := len(a.elems) - 1; i >= 0; i-- {
		if f(a.elems[i]) {
			return Some(a.elems[i])
		}
	}
	return None[T]()
}

// FindLastIndex returns the index of the last element the callback accepts, or -1
// when none does, the lowering of Array.prototype.findLastIndex. Like findIndex the
// result is a Number, so it is a float64 with -1 as the not-found sentinel, and no
// optional is needed. It walks from the end and short-circuits on the first match,
// matching JavaScript's descending visit order. The callback takes only the
// element; the index and array arguments are a later slice.
func (a *Array[T]) FindLastIndex(f func(T) bool) float64 {
	for i := len(a.elems) - 1; i >= 0; i-- {
		if f(a.elems[i]) {
			return float64(i)
		}
	}
	return -1
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

// Concat returns a new array formed by appending the elements of each argument
// array to a copy of the receiver, the lowering of Array.prototype.concat. In
// JavaScript concat spreads array arguments one level and appends non-array
// arguments as single elements. The lowering wraps a non-array argument in a
// one-element array at the call site, so by the time control reaches here every
// argument is an *Array[T] whose elements splice in. The result is a fresh array
// that aliases none of its sources, matching concat, which never mutates the
// arrays it reads.
func (a *Array[T]) Concat(others ...*Array[T]) *Array[T] {
	n := len(a.elems)
	for _, o := range others {
		n += len(o.elems)
	}
	out := make([]T, 0, n)
	out = append(out, a.elems...)
	for _, o := range others {
		out = append(out, o.elems...)
	}
	return &Array[T]{elems: out}
}

// Flat concatenates the elements of an array of arrays into one flat array, the
// lowering of Array.prototype.flat at its default depth of one over an array
// whose element type is itself an array. It is a free function rather than a
// method because it needs the inner element type T as a type argument, which the
// receiver's *Array[*Array[T]] shape names but a method on *Array[*Array[T]]
// could not introduce. The result is a fresh array that aliases none of the inner
// arrays, matching flat, which copies elements into a new array. Deeper depths
// and a mixed array of arrays and values are later slices.
func Flat[T any](a *Array[*Array[T]]) *Array[T] {
	out := make([]T, 0, len(a.elems))
	for _, inner := range a.elems {
		out = append(out, inner.elems...)
	}
	return &Array[T]{elems: out}
}

// FlatMap maps each element to an array and concatenates the results one level
// deep, the lowering of Array.prototype.flatMap over a callback that returns an
// array. It is a free function because the callback maps the element type T to an
// array of a possibly different element type U, the two type arguments the shape
// names, which a method could not introduce. It is the map-then-flat pair fused
// into one pass, so no intermediate array of arrays is built. The result is a
// fresh array; a callback returning a bare value rather than an array is a later
// slice.
func FlatMap[T, U any](a *Array[T], f func(T) *Array[U]) *Array[U] {
	out := make([]U, 0, len(a.elems))
	for _, x := range a.elems {
		out = append(out, f(x).elems...)
	}
	return &Array[U]{elems: out}
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

// Fill overwrites a range of the array with a single value in place and returns
// the array, the lowering of Array.prototype.fill. It takes the fill value and
// zero, one, or two Number bounds, matching the source call, since fill's start
// and end are both optional: fill(v) fills the whole array, fill(v, start) runs
// to the end, and fill(v, start, end) is the half-open range. The bounds are
// read through relativeIndex, the same step slice uses, so a negative bound
// counts from the end and an out-of-range or crossed pair fills nothing rather
// than panicking. It is a pointer method returning the receiver, so the write is
// visible through every reference and a.fill(0) can be read on as the same
// array, matching JavaScript where fill mutates in place and returns this.
func (a *Array[T]) Fill(v T, bounds ...float64) *Array[T] {
	n := len(a.elems)
	start := 0
	end := n
	if len(bounds) >= 1 {
		start = relativeIndex(bounds[0], n)
	}
	if len(bounds) >= 2 {
		end = relativeIndex(bounds[1], n)
	}
	for i := start; i < end; i++ {
		a.elems[i] = v
	}
	return a
}

// CopyWithin copies a block of the array to another position within the same
// array in place and returns the array, the lowering of
// Array.prototype.copyWithin. It takes a target and up to two bounds, all
// Numbers, matching the source call: copyWithin(target) copies the whole array
// onto itself starting at target, copyWithin(target, start) copies from start to
// the end, and copyWithin(target, start, end) copies the half-open source range.
// Each index is read through relativeIndex, the same step slice and fill use, so a
// negative index counts from the end and an out-of-range one clamps rather than
// panicking. The number of elements copied is capped so it neither runs past the
// source range nor writes past the end of the array, matching copyWithin, which
// never changes the length. The copy uses Go's builtin copy, whose memmove
// semantics reproduce copyWithin's overlap behavior of reading the source range
// as if to a temporary before writing, so an overlapping copy is correct. It is a
// pointer method returning the receiver, so the write is visible through every
// reference, matching JavaScript where copyWithin mutates in place and returns
// this.
func (a *Array[T]) CopyWithin(bounds ...float64) *Array[T] {
	n := len(a.elems)
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
		copy(a.elems[target:target+count], a.elems[start:start+count])
	}
	return a
}

// Splice removes deleteCount elements starting at start, inserts items in their
// place, and returns the removed elements as a new array, the lowering of
// Array.prototype.splice called with a delete count. The start goes through
// relativeIndex, so a negative one counts from the end, and the count is clamped
// into [0, len-start] so it never runs past the end nor goes negative. The array
// is rebuilt into a fresh backing slice of the head, the inserted items, and the
// tail, so the result and the receiver share no storage with each other or with
// the items. It is a pointer method that replaces the receiver's backing slice,
// so the change is visible through every reference, matching JavaScript where
// splice mutates in place and returns the removed elements.
func (a *Array[T]) Splice(start, deleteCount float64, items ...T) *Array[T] {
	n := len(a.elems)
	s := relativeIndex(start, n)
	count := int(deleteCount)
	if deleteCount != deleteCount || count < 0 { // NaN or negative removes nothing
		count = 0
	}
	if count > n-s {
		count = n - s
	}
	removed := make([]T, count)
	copy(removed, a.elems[s:s+count])
	rebuilt := make([]T, 0, n-count+len(items))
	rebuilt = append(rebuilt, a.elems[:s]...)
	rebuilt = append(rebuilt, items...)
	rebuilt = append(rebuilt, a.elems[s+count:]...)
	a.elems = rebuilt
	return &Array[T]{elems: removed}
}

// SpliceToEnd removes every element from start to the end and returns them as a
// new array, the lowering of the one-argument splice(start) form where the delete
// count is omitted and defaults to the rest of the array. The start goes through
// relativeIndex the same way Splice reads it. The removed elements are copied out
// before the receiver is truncated, so the result aliases none of the receiver's
// storage. It is a pointer method that shrinks the receiver in place, matching
// JavaScript where splice mutates the array and returns what it removed.
func (a *Array[T]) SpliceToEnd(start float64) *Array[T] {
	n := len(a.elems)
	s := relativeIndex(start, n)
	removed := make([]T, n-s)
	copy(removed, a.elems[s:])
	a.elems = a.elems[:s:s]
	return &Array[T]{elems: removed}
}

// Sort orders the array in place by a comparator and returns the array, the
// lowering of Array.prototype.sort called with a compare function. The
// comparator returns a Number that is negative when its first argument should
// sort before its second, zero when their order is left as is, and positive
// otherwise, so an element sorts before another exactly when cmp returns a
// negative value. The sort is stable, matching the guarantee modern JavaScript
// engines give, so two elements the comparator calls equal keep their relative
// order. It is a pointer method returning the receiver, the same shape reverse
// takes, so the ordering is visible through every reference and the returned
// array is the same array, not a copy, matching JavaScript where sort mutates in
// place and returns this. A comparator that returns NaN, which JavaScript treats
// as zero, reads here as not-before through the NaN < 0 being false, so those
// elements keep their order too.
func (a *Array[T]) Sort(cmp func(T, T) float64) *Array[T] {
	sort.SliceStable(a.elems, func(i, j int) bool {
		return cmp(a.elems[i], a.elems[j]) < 0
	})
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

// LastIndexOf returns the index of the last element equal to target, or -1 if
// none is, the lowering of Array.prototype.lastIndexOf. It is IndexOf scanning
// from the end instead of the front, and it takes the same caller-supplied
// equality for the same reason: a Go method cannot compare two values of its
// type parameter, and the equality is element-type-specific. lastIndexOf uses
// strict equality like indexOf, so the lowerer passes the same closure it passes
// for indexOf, and a NaN target is never found. The result is a Number, so it is
// a float64. The optional fromIndex argument is a later slice; this is the
// whole-array scan.
func (a *Array[T]) LastIndexOf(target T, eq func(T, T) bool) float64 {
	for i := len(a.elems) - 1; i >= 0; i-- {
		if eq(a.elems[i], target) {
			return float64(i)
		}
	}
	return -1
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

// Shift removes the first element and returns it, the lowering of
// Array.prototype.shift. It is the front-of-array sibling of Pop: like pop its
// declared type is T | undefined, so it returns an Opt[T], a present optional
// holding the removed element on a non-empty array and the undefined optional on
// an empty array, where JavaScript shift returns undefined and leaves the array
// empty. It is a pointer method because the removal must be visible through every
// reference to the array, the same reason Push and Pop are. The backing slice is
// advanced by one, so every remaining element's index drops by one, which is the
// downward shift JavaScript names the method for.
func (a *Array[T]) Shift() Opt[T] {
	if len(a.elems) == 0 {
		return None[T]()
	}
	first := a.elems[0]
	a.elems = a.elems[1:]
	return Some(first)
}

// Unshift prepends its arguments to the front of the array in order and returns
// the new length as a Number, the lowering of Array.prototype.unshift. It is the
// front-of-array sibling of Push, and like Push it is a pointer method so the
// insertion is visible through every reference. The arguments keep their order,
// so unshift(1, 2) on [3] yields [1, 2, 3]. A fresh backing slice is built with
// the arguments ahead of the existing elements rather than prepending in place,
// so the caller's variadic argument array is never aliased or mutated, the same
// ownership NewArray keeps.
func (a *Array[T]) Unshift(xs ...T) float64 {
	combined := make([]T, 0, len(xs)+len(a.elems))
	combined = append(combined, xs...)
	combined = append(combined, a.elems...)
	a.elems = combined
	return float64(len(a.elems))
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
