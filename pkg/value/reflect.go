// This file owns the Reflect static methods at runtime, the thin reflective layer
// over the object model the lowerer routes a Reflect.<method> call to (10 group 3).
// Each method is the same internal operation an ordinary syntax form performs,
// exposed as a function that reports success as a value rather than throwing on a
// refusal: Reflect.set returns false where a strict assignment would throw, and
// Reflect.defineProperty returns false where Object.defineProperty would. Every
// method requires an object target and throws the TypeError the spec raises when it
// is handed a primitive.

package value

// reflectObject coerces a Reflect method's target to its backing object, throwing
// the TypeError every Reflect method raises when the target is not an object. The
// method name is threaded through only to name the error the way the runtime does.
func reflectObject(target Value, method string) *Object {
	switch target.kind {
	case KindObject, KindArray, KindFunc:
		return target.object()
	default:
		Throw(NewTypeError(FromGoString("Reflect." + method + " called on non-object")))
		return nil
	}
}

// ReflectGet implements Reflect.get(target, key): the ordinary [[Get]] with the
// receiver defaulting to the target, so it reads the same value target[key] would,
// climbing the prototype chain and running an inherited getter with the target as
// its this. The three-argument receiver form is a later slice, gated at lowering.
func ReflectGet(target, key Value) Value {
	reflectObject(target, "get")
	return target.GetElem(key)
}

// ReflectHas implements Reflect.has(target, key): the [[HasProperty]] the in
// operator performs, climbing the prototype chain so an inherited property reports
// true. A symbol key is probed by identity, a string or other key by its
// property-key string, matching how the target stores each.
func ReflectHas(target, key Value) bool {
	o := reflectObject(target, "has")
	if key.kind == KindSymbol {
		return o.hasSymChained(key.symbol())
	}
	if key.kind == KindString {
		return target.HasProperty(key.str())
	}
	return target.HasProperty(ToString(key))
}

// hasSymChained reports whether the object carries a symbol-keyed property anywhere
// on its prototype chain, the symbol mirror of hasChained the in operator makes for
// a symbol key.
func (o *Object) hasSymChained(key *Symbol) bool {
	for cur := o; cur != nil; cur = cur.proto {
		for i := range cur.symKeys {
			if cur.symKeys[i] == key {
				return true
			}
		}
	}
	return false
}

// ReflectSet implements Reflect.set(target, key, value): the ordinary [[Set]] with
// the receiver defaulting to the target, returning whether the write succeeded
// instead of throwing on a refused write the way a strict assignment would. The
// four-argument receiver form is a later slice, gated at lowering.
func ReflectSet(target, key, val Value) bool {
	o := reflectObject(target, "set")
	if key.kind == KindSymbol {
		return ordinarySetSym(target, o, key.symbol(), val)
	}
	if key.kind == KindString {
		return ordinarySet(target, o, key.str(), val)
	}
	return ordinarySet(target, o, ToString(key), val)
}

// ordinarySet performs the [[Set]] of a string-keyed property on an object whose
// receiver is itself, returning success. An array's element and length writes are
// own data properties outside the named bag, so they resolve against the backing
// store: a frozen store or a non-extensible grow refuses. An own named property
// writes its value in place for a data property or runs its setter for an accessor,
// refusing a non-writable data property or an accessor with no setter. A property
// found only on the prototype chain runs an inherited setter, refuses an inherited
// non-writable data property, and otherwise creates a fresh own data property on the
// target, which a non-extensible target refuses.
func ordinarySet(target Value, o *Object, name BStr, val Value) bool {
	if target.kind == KindArray {
		s := name.ToGoString()
		if s == "length" {
			if o.elemsFrozen {
				return false
			}
			setArrayLength(o, val)
			return true
		}
		if idx, ok := arrayIndex(s); ok {
			if idx < len(o.elems) {
				if o.elemsFrozen {
					return false
				}
				o.elems[idx] = val
				return true
			}
			if o.nonExtensible {
				return false
			}
			for len(o.elems) <= idx {
				o.elems = append(o.elems, hole)
			}
			o.elems[idx] = val
			return true
		}
	}
	for i := range o.keys {
		if o.keys[i].Equal(name) {
			d := o.descs[i]
			if d.isData() {
				if !d.writable {
					return false
				}
				o.descs[i].value = val
				return true
			}
			if d.set.kind != KindFunc {
				return false
			}
			d.set.Call(target, val)
			return true
		}
	}
	for cur := o.proto; cur != nil; cur = cur.proto {
		if d, ok := cur.getOwnDesc(name); ok {
			if d.isAccessor() {
				if d.set.kind != KindFunc {
					return false
				}
				d.set.Call(target, val)
				return true
			}
			if !d.writable {
				return false
			}
			break // an inherited writable data property falls through to a fresh own create
		}
	}
	if o.nonExtensible {
		return false
	}
	o.keys = append(o.keys, name)
	o.descs = append(o.descs, defaultDataProperty(val))
	return true
}

// ordinarySetSym is the symbol mirror of ordinarySet, resolving a symbol-keyed
// property by identity through the symbol bag and its prototype chain rather than
// the named bag. It shares the same refusal rules: a non-writable data property, an
// accessor with no setter, and a non-extensible target on a fresh key all report
// false.
func ordinarySetSym(target Value, o *Object, key *Symbol, val Value) bool {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			d := o.symDescs[i]
			if d.isData() {
				if !d.writable {
					return false
				}
				o.symDescs[i].value = val
				return true
			}
			if d.set.kind != KindFunc {
				return false
			}
			d.set.Call(target, val)
			return true
		}
	}
	for cur := o.proto; cur != nil; cur = cur.proto {
		if d, ok := cur.getSymDesc(key); ok {
			if d.isAccessor() {
				if d.set.kind != KindFunc {
					return false
				}
				d.set.Call(target, val)
				return true
			}
			if !d.writable {
				return false
			}
			break
		}
	}
	if o.nonExtensible {
		return false
	}
	o.symKeys = append(o.symKeys, key)
	o.symDescs = append(o.symDescs, defaultDataProperty(val))
	return true
}
