// This file owns WeakMap, the runtime type behind a JavaScript WeakMap<K, V> the
// lowerer reaches for when a program keys a collection by an object held weakly
// (05_type_lowering the value model, 25 §24.3). A WeakMap differs from a Map in two
// ways that matter here: its keys must be objects, compared by reference identity,
// and it holds those keys weakly, so a key the rest of the program has dropped does
// not stay alive through the map and its entry disappears once the key is collected.
// A WeakMap has no size and no iteration, exactly because a weakly held key set has
// no stable count or order to expose, so the surface is only get, set, has, and
// delete.
//
// The keys are objects, which lower to Go struct pointers, so the type is generic
// over the pointee T and keys on *T. Each key is stored as a weak.Pointer[T], which
// does not keep the object alive: once the last strong reference elsewhere is gone
// the runtime may collect it, and the weak pointer then reads back nil, so find
// skips the dead entry. This is the honest weak-key model; the exact turn a dead
// entry's storage is reclaimed is the garbage-collection-timing ceiling the
// milestone names, not something a test can pin through this type.

package value

import "weak"

// WeakMap is bento's runtime representation of a JavaScript WeakMap<*T, V>. It holds
// its entries as parallel weak-key and value slices: a key is a weak.Pointer[T] that
// does not keep the object alive, and the value rides alongside it. There is no
// insertion order to preserve because a WeakMap exposes no iteration, so the slices
// are only a store the linear scan searches, the same first-cut shape Map takes.
type WeakMap[T any, V any] struct {
	keys []weak.Pointer[T]
	vals []V
}

// NewWeakMap builds an empty WeakMap keyed by *T, the lowering of new WeakMap<K, V>()
// for an object key type K whose render is *T. There is no key-kind switch the way a
// Map has, because a WeakMap key is always an object compared by reference identity,
// which Go's == on the strong pointer gives once the weak pointer is resolved.
func NewWeakMap[T any, V any]() *WeakMap[T, V] {
	return &WeakMap[T, V]{}
}

// find returns the index of the entry whose live key is k, or -1 when the map has no
// such live key. A key whose weak pointer has gone nil is a collected object; it can
// never match the live, non-nil k a caller passes, so the scan skips it and the stale
// slot lingers until a later mutation compacts it. It is the linear scan every keyed
// operation shares, the same shape Map.find takes.
func (m *WeakMap[T, V]) find(k *T) int {
	for i := range m.keys {
		if m.keys[i].Value() == k {
			return i
		}
	}
	return -1
}

// Set inserts or updates the entry for k and returns the map, the lowering of
// weakMap.set(k, v). A new key appends; an existing key takes the new value. The key
// is wrapped weakly so the map does not extend its lifetime, and the map itself is
// the result so a chained set lowers with no temporary.
func (m *WeakMap[T, V]) Set(k *T, v V) *WeakMap[T, V] {
	if i := m.find(k); i >= 0 {
		m.vals[i] = v
		return m
	}
	m.keys = append(m.keys, weak.Make(k))
	m.vals = append(m.vals, v)
	return m
}

// Get returns the value for k as an optional, undefined when the key is absent or has
// been collected, the lowering of weakMap.get(k) whose declared type is V | undefined.
func (m *WeakMap[T, V]) Get(k *T) Opt[V] {
	if i := m.find(k); i >= 0 {
		return Some(m.vals[i])
	}
	return None[V]()
}

// Has reports whether the map holds a live entry for k, the lowering of weakMap.has(k).
func (m *WeakMap[T, V]) Has(k *T) bool { return m.find(k) >= 0 }

// Delete removes the entry for k and reports whether it was present, the lowering of
// weakMap.delete(k).
func (m *WeakMap[T, V]) Delete(k *T) bool {
	i := m.find(k)
	if i < 0 {
		return false
	}
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	m.vals = append(m.vals[:i], m.vals[i+1:]...)
	return true
}
