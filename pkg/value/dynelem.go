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

import "reflect"

// dynBox boxes a single typed element into a Value. It covers the element types the
// lowerer proves boxable before it emits a collection box: a number, a string, a
// boolean, a date, and the already-boxed Value a collection written with no element
// type (`new Map()`, whose keys and values are dynamic) stores. Any other type is a
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
	case *Date:
		// A date boxes to its own view rather than to a fresh object, so a date read out
		// of a boxed collection is the same date the typed side holds and === says so.
		return e.ToValue()
	case *ArrayBuffer:
		// The three byte-buffer kinds box to their own views for the same reason, and for
		// them it matters twice over: a copy would be a second run of bytes, so a write
		// made through the collection's view would land where nobody reads it.
		return e.ToValue()
	case *SharedArrayBuffer:
		return e.ToValue()
	case *DataView:
		return e.ToValue()
	case typedArrayBacking:
		// The typed-array family is matched by its interface rather than kind by kind,
		// which is what keeps eleven concrete types from needing eleven cases here. Like the
		// buffers they box to their own view, so a write through the collection's element
		// lands in the bytes the typed side reads.
		return e.jsTypedBox()
	case dynSelfBoxing:
		// A container holding a container reaches this arm: a Map, a Set and an Array each
		// carry their own no-argument box, and matching the method rather than the type
		// keeps the generic instantiations, of which there is one per element type in the
		// program, from each needing a case. A Map and a Set box to a view the way the
		// kinds above do; an array copies, which is what its box does everywhere.
		return e.ToValue()
	}
	// A class instance is matched by its registration rather than by a case, since its Go
	// type is generated and cannot be named here. Its box is a view too: the fields are
	// read and written through the instance, and the view carries a pointer back to it so
	// the value can be unboxed into the element the collection holds.
	if v, ok := classDynBox(x); ok {
		return v
	}
	Throw(NewTypeError(FromGoString("bento: this collection's element type has no dynamic form")))
	return Undefined
}

// dynSelfBoxing is an element that already knows how to box itself with no boxer handed
// in. A Map, a Set and an Array have that method, which is what lets a collection hold
// another collection: the outer box reaches an element with only its Go type in hand, so
// a box needing an element boxer as an argument would have nowhere to get one.
type dynSelfBoxing interface {
	ToValue() Value
}

// classDynBox boxes a class instance element, reporting false when the element is not
// one. It is reflection rather than a type switch because the struct a class lowers to
// is generated: this package cannot name it, so the registry is the only thing that
// recognizes it.
func classDynBox(x any) (Value, bool) {
	t := reflect.TypeOf(x)
	if t == nil || t.Kind() != reflect.Pointer || classPrototypeFor(t.Elem()) == nil {
		return Undefined, false
	}
	return jsonStructToValue(reflect.ValueOf(x)), true
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
	case *Date:
		// Only a date box unboxes to a date. Any other object, including one carrying the
		// same properties, is not a date and could never be a member of this collection.
		if d := v.asDate(); d != nil {
			return any(d).(T), true
		}
	case *ArrayBuffer:
		// Only the matching box unboxes, and the two buffer kinds are told apart by which
		// concrete backing the box carries rather than by their shared bytes: a
		// SharedArrayBuffer is not a member of a Set<ArrayBuffer> however its storage is
		// spelled underneath.
		if b, ok := v.asBuffer().(*ArrayBuffer); ok {
			return any(b).(T), true
		}
	case *SharedArrayBuffer:
		if s, ok := v.asBuffer().(*SharedArrayBuffer); ok {
			return any(s).(T), true
		}
	case *DataView:
		if d := v.asDataView(); d != nil {
			return any(d).(T), true
		}
	case typedArrayBacking:
		// The family is matched by its interface here too, and the kind is checked by the
		// assertion back to T: an Int32Array box does not unbox into a Set<Float64Array>,
		// since the concrete backing the box carries is not the element type.
		if t := v.asTypedArray(); t != nil {
			if e, ok := any(t).(T); ok {
				return e, true
			}
		}
	default:
		// A Map and a Set come back through the backing their box carries. Their box is a
		// view kept on the collection itself, so this hands back the very collection the
		// typed side holds, which is what a Map keyed by Maps needs. An array has no arm
		// here on purpose: its box is a copy with no way back, so a Set<number[]> asked
		// whether it holds a boxed array cannot answer yes and does not pretend to.
		if m := v.asMap(); m != nil {
			if e, ok := any(m).(T); ok {
				return e, true
			}
		}
		if s := v.asSet(); s != nil {
			if e, ok := any(s).(T); ok {
				return e, true
			}
		}
		// A weak collection comes back the same way, through the view kept on it, so a
		// Map keyed by WeakMaps finds the entry a boxed WeakMap was stored under.
		if w := v.asWeak(); w != nil {
			if e, ok := any(w).(T); ok {
				return e, true
			}
		}
		// A class instance comes back through the pointer its view carries, so a value read
		// out of a boxed collection is the instance the typed side holds rather than an
		// object that merely looks like one. A plain object with the same fields carries no
		// such pointer and is correctly not a member: it could never have been stored here.
		if p, ok := v.classInstance(); ok {
			if e, ok := p.(T); ok {
				return e, true
			}
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

// StructToValue boxes one generated struct, the generic form an element boxer has to
// have. It is ObjectFromStruct with the type parameter spelled, for the same reason
// ClassToValue is: ObjectFromStruct takes an any, which fits a boxing site that names
// the value directly, and ArrayValueOf wants a func(T) Value it can apply down a typed
// slice. The two names say what the emitter proved about the element, a plain fixed
// shape here and a registered class instance there; the walk underneath is one walk,
// which is what keeps a class named wherever it is reached.
func StructToValue[T any](x T) Value {
	return ObjectFromStruct(x)
}

// Identity is the element boxer for an element that is already a box, the value.Value
// an any[] or an array written with no element type at all holds. ArrayValueOf applies
// a boxer to every element and has no way to skip one, so the array of boxes needs a
// boxer that hands its argument straight back rather than a special case in the loop.
// It is spelled here rather than as a closure at each site so an emitted box of a
// dynamic array reads as the one call the other element types get.
func Identity(v Value) Value { return v }
