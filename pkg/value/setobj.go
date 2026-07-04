// This file owns Set, the runtime type behind a JavaScript Set<T> the lowerer
// reaches for when a program constructs a collection of unique values (05_type_
// lowering the value model). A JavaScript Set keeps its members in insertion order
// and compares them by SameValueZero, neither of which a bare Go map gives, so Set
// wraps an ordered member slice and a member-equality strategy that captures those
// semantics.
//
// Like Map, this first cut scans its members linearly: it is behaviorally identical
// to a hashed index, preserving the insertion order for...of and forEach observe,
// and the O(1) index is a later performance slice that changes no observable
// result. A Set is always a *Set[T] in generated code, today and after that lands,
// so the optimization can grow underneath without changing how a set is spelled.

package value

// Set is bento's runtime representation of a JavaScript Set<T>. It holds its
// members as a single slice in insertion order, the order for...of and forEach
// observe, and an eq function that decides member identity so number, string, and
// boolean members each compare the way JavaScript's SameValueZero does for that
// kind. The type is monomorphized: the compiler proved T, so there is no boxing on
// the members themselves. It is deliberately the members-only shape of Map, which
// keeps the two collections' semantics in one mental model.
type Set[T any] struct {
	members []T
	eq      func(T, T) bool
}

// NewNumberSet builds an empty Set with number members, the lowering of new
// Set<number>(). Members compare by SameValueZero: NaN is a single member (every
// NaN matches) and +0 and -0 are the same member, which is exactly what a plain ==
// misses for NaN and gets right for the zeroes, so the equality folds the NaN case
// in by hand, the same way NewNumberMap does for its keys.
func NewNumberSet() *Set[float64] {
	return &Set[float64]{eq: func(a, b float64) bool {
		return a == b || (a != a && b != b)
	}}
}

// NewStringSet builds an empty Set with string members, the lowering of new
// Set<string>(). Members compare by the string's UTF-16 code units through
// BStr.Equal, so two strings that print the same are the same member however each
// was built.
func NewStringSet() *Set[BStr] {
	return &Set[BStr]{eq: func(a, b BStr) bool { return a.Equal(b) }}
}

// NewBoolSet builds an empty Set with boolean members, the lowering of new
// Set<boolean>(). There are only two members, so plain == is the whole of
// SameValueZero here.
func NewBoolSet() *Set[bool] {
	return &Set[bool]{eq: func(a, b bool) bool { return a == b }}
}

// find returns the index of the member that matches v, or -1 when the set has no
// such member. It is the linear scan every membership operation shares; the hashed
// index that replaces it is a later performance slice.
func (s *Set[T]) find(v T) int {
	for i := range s.members {
		if s.eq(s.members[i], v) {
			return i
		}
	}
	return -1
}

// Add inserts v if it is not already present and returns the set, the lowering of
// set.add(v). A new member appends in insertion order; a member already present
// leaves the set unchanged and keeps its position, matching JavaScript, and the set
// itself is the result so a chained add lowers with no temporary.
func (s *Set[T]) Add(v T) *Set[T] {
	if s.find(v) < 0 {
		s.members = append(s.members, v)
	}
	return s
}

// Has reports whether the set holds v, the lowering of set.has(v).
func (s *Set[T]) Has(v T) bool { return s.find(v) >= 0 }

// Delete removes v and reports whether it was present, the lowering of
// set.delete(v). The remaining members keep their relative order, matching
// JavaScript, so a later iteration still visits them in insertion order.
func (s *Set[T]) Delete(v T) bool {
	i := s.find(v)
	if i < 0 {
		return false
	}
	s.members = append(s.members[:i], s.members[i+1:]...)
	return true
}

// Clear removes every member, the lowering of set.clear(). The slice is truncated
// to length zero but keeps its backing storage, so a set that is refilled after a
// clear does not reallocate from empty.
func (s *Set[T]) Clear() {
	s.members = s.members[:0]
}

// Size is the member count as a Number, the lowering of the set.size accessor. It
// is a float64 to match the type the checker gives the property and to compose with
// the numeric path with no conversion at the use site.
func (s *Set[T]) Size() float64 { return float64(len(s.members)) }

// Range visits each member in insertion order, the iteration forEach lowers to and
// the shape a later for...of over a set reads. It passes the member by value, so a
// callback cannot alias the set's storage.
func (s *Set[T]) Range(fn func(T)) {
	for _, v := range s.members {
		fn(v)
	}
}
