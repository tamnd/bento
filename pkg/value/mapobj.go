// This file owns Map, the runtime type behind a JavaScript Map<K, V> the lowerer
// reaches for when a program constructs a keyed collection (05_type_lowering the
// value model, 16_go_interop section 6.5, where a Go map[K]V projects to it). A
// JavaScript Map keeps its entries in insertion order and compares keys by
// SameValueZero, neither of which a bare Go map gives, so Map wraps ordered key
// and value slices and a key-equality strategy that captures those semantics.
//
// This first cut scans its keys linearly, the same shape Object keeps for its
// named properties: it is behaviorally identical to a hashed index, preserving
// insertion order the way JavaScript enumerates, and the O(1) index is a later
// performance slice (the value runtime's shape-and-hash work) that changes no
// observable result. A Map is always a *Map[K, V] in generated code, today and
// after that lands, so the optimization can grow underneath without changing how
// a map is spelled.

package value

// Map is bento's runtime representation of a JavaScript Map<K, V>. It holds its
// entries as parallel key and value slices in insertion order, the order for...of,
// forEach, and the go: crossing all observe, and an eq function that decides key
// identity so number, string, and boolean keys each compare the way JavaScript's
// SameValueZero does for that kind. The type is monomorphized: the compiler proved
// K and V, so there is no boxing on the entries themselves.
type Map[K any, V any] struct {
	keys []K
	vals []V
	eq   func(K, K) bool
}

// NewNumberMap builds an empty Map with number keys, the lowering of new Map<number,
// V>(). Keys compare by SameValueZero: NaN is a single key (every NaN matches) and
// +0 and -0 are the same key, which is exactly what a plain == misses for NaN and
// gets right for the zeroes, so the equality folds the NaN case in by hand.
func NewNumberMap[V any]() *Map[float64, V] {
	return &Map[float64, V]{eq: func(a, b float64) bool {
		return a == b || (a != a && b != b)
	}}
}

// NewStringMap builds an empty Map with string keys, the lowering of new Map<string,
// V>(). Keys compare by the string's UTF-16 code units through BStr.Equal, so two
// strings that print the same are the same key however each was built.
func NewStringMap[V any]() *Map[BStr, V] {
	return &Map[BStr, V]{eq: func(a, b BStr) bool { return a.Equal(b) }}
}

// NewBoolMap builds an empty Map with boolean keys, the lowering of new Map<boolean,
// V>(). There are only two keys, so plain == is the whole of SameValueZero here.
func NewBoolMap[V any]() *Map[bool, V] {
	return &Map[bool, V]{eq: func(a, b bool) bool { return a == b }}
}

// NewRefMap builds an empty Map whose keys are objects compared by reference
// identity, the lowering of new Map<K, V>() for an object key type K. A JavaScript
// object key matches under SameValueZero, which for objects is reference identity:
// two object keys are the same key exactly when they are the same object. Objects
// lower to Go struct pointers, so Go's == on those pointers is that identity, and
// there is no NaN case to fold in the way a number key has. K is constrained to
// comparable because only a comparable key can back the == the equality uses; the
// lowerer only reaches this constructor for a key type that renders to a pointer.
func NewRefMap[K comparable, V any]() *Map[K, V] {
	return &Map[K, V]{eq: func(a, b K) bool { return a == b }}
}

// find returns the index of the entry whose key matches k, or -1 when the map has
// no such key. It is the linear scan every keyed operation shares; the hashed index
// that replaces it is a later performance slice.
func (m *Map[K, V]) find(k K) int {
	for i := range m.keys {
		if m.eq(m.keys[i], k) {
			return i
		}
	}
	return -1
}

// Set inserts or updates the entry for k and returns the map, the lowering of
// map.set(k, v). A new key appends in insertion order; an existing key keeps its
// position and takes the new value, matching JavaScript, and the map itself is the
// result so a chained set lowers with no temporary.
func (m *Map[K, V]) Set(k K, v V) *Map[K, V] {
	if i := m.find(k); i >= 0 {
		m.vals[i] = v
		return m
	}
	m.keys = append(m.keys, k)
	m.vals = append(m.vals, v)
	return m
}

// Get returns the value for k as an optional, undefined when the key is absent, the
// lowering of map.get(k) whose declared type is V | undefined. It hands back an
// Opt[V] so the value composes with the same narrowing and nullish paths any other
// optional takes.
func (m *Map[K, V]) Get(k K) Opt[V] {
	if i := m.find(k); i >= 0 {
		return Some(m.vals[i])
	}
	return None[V]()
}

// Has reports whether the map holds an entry for k, the lowering of map.has(k).
func (m *Map[K, V]) Has(k K) bool { return m.find(k) >= 0 }

// Delete removes the entry for k and reports whether it was present, the lowering
// of map.delete(k). The remaining entries keep their relative order, matching
// JavaScript, so a later iteration still visits them in insertion order.
func (m *Map[K, V]) Delete(k K) bool {
	i := m.find(k)
	if i < 0 {
		return false
	}
	m.keys = append(m.keys[:i], m.keys[i+1:]...)
	m.vals = append(m.vals[:i], m.vals[i+1:]...)
	return true
}

// Clear removes every entry, the lowering of map.clear(). The slices are truncated
// to length zero but keep their backing storage, so a map that is refilled after a
// clear does not reallocate from empty.
func (m *Map[K, V]) Clear() {
	m.keys = m.keys[:0]
	m.vals = m.vals[:0]
}

// Size is the entry count as a Number, the lowering of the map.size accessor. It is
// a float64 to match the type the checker gives the property and to compose with
// the numeric path with no conversion at the use site.
func (m *Map[K, V]) Size() float64 { return float64(len(m.keys)) }

// Range visits each entry in insertion order, the iteration the go: crossing reads
// to marshal a bento map to a Go map. It passes the key and value by value, so a
// callback cannot alias the map's storage.
func (m *Map[K, V]) Range(fn func(K, V)) {
	for i, k := range m.keys {
		fn(k, m.vals[i])
	}
}

// ForEach visits each entry in insertion order, passing the value then the key, the
// order Map.prototype.forEach hands its callback (value, key, map). It is the two-
// argument shape a forEach callback that reads both takes; a callback that reads
// only the value lowers to ForEachValue instead. The entries are passed by value,
// so a callback cannot alias the map's storage.
func (m *Map[K, V]) ForEach(fn func(V, K)) {
	for i, k := range m.keys {
		fn(m.vals[i], k)
	}
}

// ForEachValue visits each entry's value in insertion order, the shape a forEach
// callback that reads only its first parameter takes, so the common (value) => ...
// form needs no unused key binding.
func (m *Map[K, V]) ForEachValue(fn func(V)) {
	for _, v := range m.vals {
		fn(v)
	}
}

// Keys returns the map's keys in insertion order, the traversal map.keys() and a
// for...of over the keys read. It copies the backing slice so a mutation to the map
// during the loop does not disturb the range in progress; the live-view an iterator
// has of concurrent mutation is a later slice.
func (m *Map[K, V]) Keys() []K {
	return append([]K(nil), m.keys...)
}

// Values returns the map's values in insertion order, the traversal map.values()
// and a for...of over the values read. It copies the backing slice for the same
// reason Keys does, so the two snapshots a for-of over entries pairs are consistent
// and stable across a body that mutates the map.
func (m *Map[K, V]) Values() []V {
	return append([]V(nil), m.vals...)
}
