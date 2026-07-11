// This file owns the enumeration statics that copy or list an object's own
// properties over the dynamic bag (07 group 6): Object.assign copies own
// enumerable properties across objects, and the listing statics turn the bag into
// entry pairs and symbol-key arrays. Each walks the bag in the spec's enumeration
// order, integer indices ascending then the remaining string keys in insertion
// order, so the result reads the way the source enumerates. A primitive receiver
// carries no bag, so a listing static answers with an empty array.

package value

// Assign copies the own enumerable properties of each source onto the receiver, the
// target, through the ordinary get and set path, the runtime behind
// Object.assign(target, ...sources). A source's string keys copy in the spec's
// enumeration order, integer indices ascending then the remaining keys in insertion
// order, followed by its own enumerable symbol keys in insertion order. A null or
// undefined source contributes nothing, matching the spec's skip, and a string
// source contributes its index characters, the own enumerable properties its wrapper
// exposes. Each property lands through the target's Set, so the target's own
// writability and extensibility govern whether it takes. The target is returned so
// the call reads as the assignment expression Object.assign evaluates to.
func (v Value) Assign(sources ...Value) Value {
	for _, src := range sources {
		switch src.kind {
		case KindObject, KindArray, KindFunc:
			o := src.object()
			for _, k := range o.orderedStringKeysFiltered(true) {
				v.SetKey(k, src.Get(k))
			}
			for i := range o.symKeys {
				if o.symDescs[i].enumerable {
					v.setSymKey(o.symKeys[i], o.getSym(src, o.symKeys[i]))
				}
			}
		case KindString:
			n := int(src.str().Length())
			for i := 0; i < n; i++ {
				key := NumberToString(float64(i))
				v.SetKey(key, src.Get(key))
			}
		}
	}
	return v
}

// Entries returns the receiver's own enumerable string-keyed properties as a boxed
// array of [key, value] pairs in the spec's enumeration order, the value
// Object.entries builds for a dynamic receiver. Each pair is a two-element array
// whose first element is the key string and whose second is the value the same read
// Object.values makes resolves. The result is a boxed value rather than a typed
// array because its elements are themselves arrays, so a member read off a pair,
// entries[i][0], dispatches through the dynamic Get the way the source's own reads
// do. A receiver with no object storage yields an empty array.
func (v Value) Entries() Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return NewArrayValue(nil)
	}
	keys := v.object().orderedStringKeysFiltered(true)
	pairs := make([]Value, len(keys))
	for i, k := range keys {
		pairs[i] = NewArrayValue([]Value{StringValue(k), v.Get(k)})
	}
	return NewArrayValue(pairs)
}

// FromEntries builds a fresh object from an iterable of key-value pairs, the
// runtime behind Object.fromEntries(iterable). Each entry is read for its first two
// elements, the key and the value, and the key is set on the new object through the
// ordinary property-key coercion, so a later entry with the same key overwrites an
// earlier one. The entries are taken from the iterable's dense elements, the array
// form the static covers, so a non-array iterable yields an empty object.
func FromEntries(iterable Value) Value {
	out := NewObject()
	if iterable.kind == KindArray {
		for _, entry := range iterable.object().elems {
			out.SetElem(entry.GetIndex(0), entry.GetIndex(1))
		}
	}
	return out
}
