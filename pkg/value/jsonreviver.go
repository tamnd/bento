// This file owns JSON.parse with a reviver: the second argument that is called
// for every key and value the parse produced and whose return replaces the value
// or, when it returns undefined, deletes the property (10_value_model, JSON
// parsing). It follows the specification's InternalizeJSONProperty walk, which
// visits a value's children before the value itself, so a reviver sees a fully
// revived child by the time it runs on the parent.
//
// The reviver operates on the boxed dynamic Value tree JSONParse builds, the same
// tree the replacer walk in jsonreplacer.go operates on: the specification hands
// the reviver each value and takes back whatever it returns, so the value must be
// a Value the compiled arrow can read and rebuild. The walk mutates the tree in
// place, deleting a property whose reviver result is undefined and overwriting one
// whose result differs, then returns the revived root.

package value

// JSONParseReviver is JSON.parse(text, reviver): it parses the text into a boxed
// Value tree and then walks the tree bottom-up, calling the reviver for every key
// and value. A reviver result of undefined deletes the property; any other result
// replaces it. The root is held under the empty key of a wrapper object, the way
// the specification seeds the walk, so the reviver runs once more on the whole
// document under the "" key and can replace it outright.
func JSONParseReviver(s BStr, reviver func(BStr, Value) Value) Value {
	root := JSONParse(s)
	holder := NewObject()
	holder.Set(FromGoString(""), root)
	return internalizeJSON(holder, FromGoString(""), reviver)
}

// internalizeJSON revives holder[name]: it reads the value, recurses into the
// children of an array or object first so each child is revived before its parent,
// and finally calls the reviver on the value under its key. A child whose reviver
// result is undefined is deleted, leaving an array hole or dropping an object key
// the way the specification's [[Delete]] does; any other result overwrites the
// child in place.
func internalizeJSON(holder Value, name BStr, reviver func(BStr, Value) Value) Value {
	val := holder.Get(name)
	switch val.kind {
	case KindArray:
		o := val.object()
		for i := range o.elems {
			key := NumberToString(float64(i))
			revived := internalizeJSON(val, key, reviver)
			if revived.kind == KindUndefined {
				val.Delete(key)
			} else {
				o.elems[i] = revived
			}
		}
	case KindObject:
		for _, key := range internalizeObjectKeys(val) {
			revived := internalizeJSON(val, key, reviver)
			if revived.kind == KindUndefined {
				val.Delete(key)
			} else {
				val.Set(key, revived)
			}
		}
	}
	return reviver(name, val)
}

// internalizeObjectKeys snapshots an object's own enumerable string keys in
// insertion order before the walk mutates it, so deleting a key mid-walk does not
// disturb the iteration the way ranging the live key slice would.
func internalizeObjectKeys(v Value) []BStr {
	o := v.object()
	keys := make([]BStr, 0, len(o.keys))
	for i := range o.keys {
		if o.descs[i].enumerable {
			keys = append(keys, o.keys[i])
		}
	}
	return keys
}
