// This file bridges one element of a monomorphized collection in and out of the
// boxed dynamic world. A Map<K, V> and a Set<T> are generic Go types whose elements
// are the compiler-proved K, V and T with no box on them, which is what makes a
// typed collection fast; a box handed to console.log or to assert has to present
// those elements as value.Value all the same, and a write through the box has to
// land back in the typed storage.
//
// The two directions are not symmetric. Boxing always succeeds for the element types
// the lowerer boxes a collection for, since it only emits the box when it has already
// proved each of them has a dynamic form. Unboxing can fail: a Map<number, string>
// asked for the key "a" holds no such key and cannot hold one, so the read answers
// absent and a write raises rather than dropping the entry on the floor.

package value

// dynBox boxes a single typed element into a Value. It covers the element types the
// lowerer proves boxable before it emits a collection box: a number, a string, a
// boolean, and the already-boxed Value a collection written with no element type
// (`new Map()`, whose keys and values are dynamic) stores. Any other type is a
// lowering that should not have been emitted, so it raises rather than answering a
// stand-in value that would read as a real element.
func dynBox[T any](x T) Value {
	switch e := any(x).(type) {
	case Value:
		return e
	case float64:
		return Number(e)
	case BStr:
		return StringValue(e)
	case bool:
		return Bool(e)
	}
	Throw(NewTypeError(FromGoString("bento: this collection's element type has no dynamic form")))
	return Undefined
}

// dynUnbox converts a boxed value back into the collection's typed element,
// reporting false when the value cannot be one. A typed collection holds exactly one
// kind of element, so a dynamic key or member of another kind is not merely absent
// from the collection, it could never be in it: map.get(k) for such a key answers
// undefined and map.has(k) answers false, which is what the read paths do with the
// false. A dynamic Value element takes anything, since that is what its type says,
// which is the case a collection written the JavaScript way (`new Map()`, `new Set()`)
// always takes.
func dynUnbox[T any](v Value) (T, bool) {
	var zero T
	switch any(zero).(type) {
	case Value:
		return any(v).(T), true
	case float64:
		if v.kind == KindNumber {
			return any(v.AsNumber()).(T), true
		}
	case BStr:
		if v.kind == KindString {
			return any(v.AsString()).(T), true
		}
	case bool:
		if v.kind == KindBool {
			return any(v.AsBool()).(T), true
		}
	}
	return zero, false
}

// dynUnboxOrThrow is dynUnbox for a write, where an element that does not fit cannot
// be reported as absent: map.set(k, v) through a box either stores the entry or it
// does not happen, and silently dropping it would leave the program reading a map
// that lost a write it made. A typed collection whose element type the value does not
// fit raises a TypeError naming what happened, which is honest and, unlike the
// dropped write, cannot be mistaken for success. It is reachable only from a
// collection the checker gave a narrower element type than the value being written,
// so a collection written the JavaScript way never reaches it.
func dynUnboxOrThrow[T any](v Value, what string) T {
	e, ok := dynUnbox[T](v)
	if !ok {
		Throw(NewTypeError(FromGoString("bento: a " + what + " of this type cannot be stored in a collection typed for another")))
	}
	return e
}
