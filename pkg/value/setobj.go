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

import "slices"

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

// NewRefSet builds an empty Set whose members are objects compared by reference
// identity, the lowering of new Set<T>() for an object member type T. As with
// NewRefMap, an object member matches under SameValueZero by reference identity,
// which is Go's == on the struct pointers objects lower to, so two members are the
// same member exactly when they are the same object. T is constrained to comparable
// because only a comparable member can back that ==; the lowerer only reaches this
// constructor for a member type that renders to a pointer.
func NewRefSet[T comparable]() *Set[T] {
	return &Set[T]{eq: func(a, b T) bool { return a == b }}
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

// Range visits each member in insertion order, the shape a later for...of over a
// set reads. It passes the member by value, so a callback cannot alias the set's
// storage.
func (s *Set[T]) Range(fn func(T)) {
	for _, v := range s.members {
		fn(v)
	}
}

// ForEach visits each member in insertion order, the shape Set.prototype.forEach
// hands its callback. The specification passes the member twice and then the set
// (value, value, set); a callback that reads only the first parameter, the common
// form, takes this one-argument shape. The member is passed by value, so a callback
// cannot alias the set's storage.
func (s *Set[T]) ForEach(fn func(T)) {
	for _, v := range s.members {
		fn(v)
	}
}

// Members returns the set's members in insertion order, the traversal set.values(),
// set.keys(), and a for...of over the set read (a Set's keys and values are its
// members, so all three project to this). It copies the backing slice so a mutation
// to the set during the loop does not disturb the range in progress; the live-view
// an iterator has of concurrent mutation is a later slice.
func (s *Set[T]) Members() []T {
	return append([]T(nil), s.members...)
}

// newLike returns an empty set that shares this set's member equality, the base
// every algebra result builds on so the new set compares members the way its
// operands do. The members are left empty for the caller to fill in the order the
// operation requires.
func (s *Set[T]) newLike() *Set[T] {
	return &Set[T]{eq: s.eq}
}

// Union returns a new set of the members in this set or the other, the lowering of
// set.union(other) (ES2025). The result lists this set's members in their order
// first, then the other set's members not already present, which is the order the
// specification builds by seeding the result with the receiver and appending the
// argument's new members.
func (s *Set[T]) Union(other *Set[T]) *Set[T] {
	out := s.newLike()
	out.members = append(out.members, s.members...)
	for _, v := range other.members {
		out.Add(v)
	}
	return out
}

// Intersection returns a new set of the members in both sets, the lowering of
// set.intersection(other) (ES2025). The specification walks the smaller set and
// keeps the members the larger one also holds, so the result order follows
// whichever operand is smaller: this set's order when it is the smaller, the
// other's order otherwise. Both operands are deduped already, so each kept member
// is appended directly.
func (s *Set[T]) Intersection(other *Set[T]) *Set[T] {
	out := s.newLike()
	if len(s.members) <= len(other.members) {
		for _, v := range s.members {
			if other.Has(v) {
				out.members = append(out.members, v)
			}
		}
	} else {
		for _, v := range other.members {
			if s.Has(v) {
				out.members = append(out.members, v)
			}
		}
	}
	return out
}

// Difference returns a new set of the members in this set but not the other, the
// lowering of set.difference(other) (ES2025). It keeps this set's members that the
// other lacks, in this set's order.
func (s *Set[T]) Difference(other *Set[T]) *Set[T] {
	out := s.newLike()
	for _, v := range s.members {
		if !other.Has(v) {
			out.members = append(out.members, v)
		}
	}
	return out
}

// SymmetricDifference returns a new set of the members in exactly one of the two
// sets, the lowering of set.symmetricDifference(other) (ES2025). It lists this
// set's members the other lacks, in this set's order, then the other's members
// this set lacks, in the other's order, which is the order the specification
// builds by seeding with the receiver and toggling each of the argument's members.
func (s *Set[T]) SymmetricDifference(other *Set[T]) *Set[T] {
	out := s.newLike()
	for _, v := range s.members {
		if !other.Has(v) {
			out.members = append(out.members, v)
		}
	}
	for _, v := range other.members {
		if !s.Has(v) {
			out.members = append(out.members, v)
		}
	}
	return out
}

// IsSubsetOf reports whether every member of this set is in the other, the
// lowering of set.isSubsetOf(other) (ES2025). A set larger than the other cannot
// be a subset, so the size check settles that case before the membership scan.
func (s *Set[T]) IsSubsetOf(other *Set[T]) bool {
	if len(s.members) > len(other.members) {
		return false
	}
	for _, v := range s.members {
		if !other.Has(v) {
			return false
		}
	}
	return true
}

// IsSupersetOf reports whether every member of the other set is in this one, the
// lowering of set.isSupersetOf(other) (ES2025). A set smaller than the other
// cannot be a superset, so the size check settles that case before the scan.
func (s *Set[T]) IsSupersetOf(other *Set[T]) bool {
	if len(s.members) < len(other.members) {
		return false
	}
	for _, v := range other.members {
		if !s.Has(v) {
			return false
		}
	}
	return true
}

// IsDisjointFrom reports whether the two sets share no member, the lowering of
// set.isDisjointFrom(other) (ES2025). It scans the smaller set against the larger,
// the work the specification does, and a single shared member settles it as false.
func (s *Set[T]) IsDisjointFrom(other *Set[T]) bool {
	if len(s.members) <= len(other.members) {
		return !slices.ContainsFunc(s.members, other.Has)
	}
	return !slices.ContainsFunc(other.members, s.Has)
}
