// This file owns WeakSet, the runtime type behind a JavaScript WeakSet<T> the lowerer
// reaches for when a program collects objects held weakly (05_type_lowering the value
// model, 25 §24.4). A WeakSet is to a Set what a WeakMap is to a Map: its members must
// be objects, compared by reference identity, and it holds them weakly, so a member
// the rest of the program has dropped does not stay alive through the set and its slot
// disappears once the object is collected. It has no size and no iteration, so the
// surface is only add, has, and delete.
//
// The members are objects, which lower to Go struct pointers, so the type is generic
// over the pointee T and holds each member as a weak.Pointer[T]. A weak pointer does
// not keep the object alive: once the last strong reference elsewhere is gone the
// runtime may collect it, and the weak pointer then reads back nil, so find skips the
// dead slot. The exact turn a dead slot's storage is reclaimed is the
// garbage-collection-timing ceiling the milestone names.

package value

import "weak"

// WeakSet is bento's runtime representation of a JavaScript WeakSet<*T>. It holds its
// members as a slice of weak.Pointer[T] that do not keep the objects alive. There is
// no order to preserve because a WeakSet exposes no iteration, so the slice is only a
// store the linear scan searches, the same first-cut shape Set takes.
type WeakSet[T any] struct {
	members []weak.Pointer[T]
}

// NewWeakSet builds an empty WeakSet over *T, the lowering of new WeakSet<E>() for an
// object member type E whose render is *T. There is no member-kind switch the way a
// Set has, because a WeakSet member is always an object compared by reference
// identity, which Go's == on the strong pointer gives once the weak pointer resolves.
func NewWeakSet[T any]() *WeakSet[T] {
	return &WeakSet[T]{}
}

// find returns the index of the live member equal to k, or -1 when the set has no such
// live member. A member whose weak pointer has gone nil is a collected object; it can
// never match the live, non-nil k a caller passes, so the scan skips it.
func (s *WeakSet[T]) find(k *T) int {
	for i := range s.members {
		if s.members[i].Value() == k {
			return i
		}
	}
	return -1
}

// Add inserts k if absent and returns the set, the lowering of weakSet.add(k). The
// member is wrapped weakly so the set does not extend its lifetime, and the set itself
// is the result so a chained add lowers with no temporary.
func (s *WeakSet[T]) Add(k *T) *WeakSet[T] {
	if s.find(k) < 0 {
		s.members = append(s.members, weak.Make(k))
	}
	return s
}

// Has reports whether the set holds a live member equal to k, the lowering of
// weakSet.has(k).
func (s *WeakSet[T]) Has(k *T) bool { return s.find(k) >= 0 }

// Delete removes k and reports whether it was present, the lowering of
// weakSet.delete(k).
func (s *WeakSet[T]) Delete(k *T) bool {
	i := s.find(k)
	if i < 0 {
		return false
	}
	s.members = append(s.members[:i], s.members[i+1:]...)
	return true
}
