// This file owns the Reflect static methods at runtime, the thin reflective layer
// over the object model the lowerer routes a Reflect.<method> call to (10 group 3).
// Each method is the same internal operation an ordinary syntax form performs,
// exposed as a function that reports success as a value rather than throwing on a
// refusal: Reflect.set returns false where a strict assignment would throw, and
// Reflect.defineProperty returns false where Object.defineProperty would. Every
// method requires an object target and throws the TypeError the spec raises when it
// is handed a primitive.

package value

import "sort"

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
			d.set.Call(val)
			return true
		}
	}
	for cur := o.proto; cur != nil; cur = cur.proto {
		if d, ok := cur.getOwnDesc(name); ok {
			if d.isAccessor() {
				if d.set.kind != KindFunc {
					return false
				}
				d.set.Call(val)
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

// ReflectDeleteProperty implements Reflect.deleteProperty(target, key): the
// [[Delete]] the delete operator performs, returning whether the removal succeeded.
// A configurable or absent property removes and reports true; a non-configurable
// property survives and reports false.
func ReflectDeleteProperty(target, key Value) bool {
	reflectObject(target, "deleteProperty")
	return target.DeleteElem(key)
}

// ReflectOwnKeys implements Reflect.ownKeys(target): every own property key, string
// and symbol alike, in the spec's [[OwnPropertyKeys]] order. Integer-index keys come
// first in ascending numeric order, then the remaining string keys in insertion
// order, then the symbol keys in insertion order. An array contributes its element
// indices and then its length as a string key, ahead of any other string key.
func ReflectOwnKeys(target Value) *Array[Value] {
	if p := target.asProxy(); p != nil {
		return NewArray(p.ownKeys()...)
	}
	o := reflectObject(target, "ownKeys")
	var idxKeys []int
	var strKeys []BStr
	for i := range o.elems {
		if !isHole(o.elems[i]) {
			idxKeys = append(idxKeys, i)
		}
	}
	for _, k := range o.keys {
		if n, ok := arrayIndex(k.ToGoString()); ok {
			idxKeys = append(idxKeys, n)
		} else {
			strKeys = append(strKeys, k)
		}
	}
	sort.Ints(idxKeys)
	out := make([]Value, 0, len(idxKeys)+len(strKeys)+len(o.symKeys)+1)
	for _, n := range idxKeys {
		out = append(out, StringValue(NumberToString(float64(n))))
	}
	if target.kind == KindArray {
		out = append(out, StringValue(FromGoString("length")))
	}
	for _, k := range strKeys {
		out = append(out, StringValue(k))
	}
	for _, s := range o.symKeys {
		out = append(out, symbolValue(s))
	}
	return NewArray(out...)
}

// ReflectDefineProperty implements Reflect.defineProperty(target, key, descriptor):
// the [[DefineOwnProperty]] Object.defineProperty performs, returning whether the
// define succeeded instead of throwing on a define the invariants forbid. It reads
// the descriptor object, validates the change against the target's extensibility and
// the existing property's configurability, and applies it, reporting false for the
// same rejection Object.defineProperty turns into a TypeError.
func ReflectDefineProperty(target, key, descObj Value) bool {
	o := reflectObject(target, "defineProperty")
	in := readDescriptorInput(descObj)
	if key.kind == KindSymbol {
		sym := key.symbol()
		current, exists := o.getSymDesc(sym)
		if !validateDefine(current, exists, in, o.isExtensible()) {
			return false
		}
		o.defineSym(sym, in.toDescriptor(current, exists))
		return true
	}
	var name BStr
	if key.kind == KindString {
		name = key.str()
	} else {
		name = ToString(key)
	}
	current, exists := o.getOwnDesc(name)
	if !validateDefine(current, exists, in, o.isExtensible()) {
		return false
	}
	o.defineOwn(name, in.toDescriptor(current, exists))
	return true
}

// ReflectGetOwnPropertyDescriptor implements Reflect.getOwnPropertyDescriptor(target,
// key): the [[GetOwnProperty]] Object.getOwnPropertyDescriptor performs, returning the
// descriptor object for an own property or undefined when the key is absent. Unlike
// the Object form, which coerces a primitive target to an object, it throws the
// TypeError every Reflect method raises on a non-object target.
func ReflectGetOwnPropertyDescriptor(target, key Value) Value {
	reflectObject(target, "getOwnPropertyDescriptor")
	return target.GetOwnPropertyDescriptor(key)
}

// ReflectGetPrototypeOf implements Reflect.getPrototypeOf(target): the
// [[GetPrototypeOf]] Object.getPrototypeOf performs, reporting the target's prototype
// object or null. Unlike the Object form it throws the TypeError every Reflect method
// raises on a non-object target rather than coercing it.
func ReflectGetPrototypeOf(target Value) Value {
	reflectObject(target, "getPrototypeOf")
	return target.GetPrototype()
}

// ReflectSetPrototypeOf implements Reflect.setPrototypeOf(target, proto): the
// [[SetPrototypeOf]] Object.setPrototypeOf performs, returning whether the write
// succeeded instead of throwing on a refused change. A non-object, non-null prototype
// throws a TypeError, as does a non-object target. Setting the prototype a
// non-extensible object already holds succeeds, while changing it to a different one
// is refused and reports false.
func ReflectSetPrototypeOf(target, proto Value) bool {
	o := reflectObject(target, "setPrototypeOf")
	var np *Object
	switch proto.kind {
	case KindObject, KindArray, KindFunc:
		np = proto.object()
	case KindNull:
		np = nil
	default:
		Throw(NewTypeError(FromGoString("Reflect.setPrototypeOf called with an invalid prototype")))
		return false
	}
	if np == o.proto {
		return true
	}
	if !o.isExtensible() {
		return false
	}
	o.proto = np
	return true
}

// ReflectIsExtensible implements Reflect.isExtensible(target): the [[IsExtensible]]
// Object.isExtensible performs, reporting whether new own properties may be added. It
// throws the TypeError every Reflect method raises on a non-object target rather than
// coercing a primitive the way the Object form does.
func ReflectIsExtensible(target Value) bool {
	o := reflectObject(target, "isExtensible")
	return o.isExtensible()
}

// ReflectPreventExtensions implements Reflect.preventExtensions(target): the
// [[PreventExtensions]] Object.preventExtensions performs, marking the target closed
// to new own properties and reporting success, which for an ordinary object is always
// true. It throws the TypeError every Reflect method raises on a non-object target.
func ReflectPreventExtensions(target Value) bool {
	reflectObject(target, "preventExtensions")
	target.PreventExtensions()
	return true
}

// ReflectApply implements Reflect.apply(target, thisArgument, argumentsList): the
// [[Call]] Function.prototype.apply performs, reading the array-like argumentsList
// into a positional argument list the spec's CreateListFromArrayLike way and calling
// the target with it. bento's callables never read this, so a body that would consult
// thisArgument hands back at its declaration and the argument is threaded no further
// here. A non-callable target throws the TypeError the spec raises.
func ReflectApply(target, thisArg, argsList Value) Value {
	if target.kind != KindFunc {
		Throw(NewTypeError(FromGoString("Reflect.apply called on non-callable target")))
		return Undefined
	}
	_ = thisArg
	n := arrayLikeLen(argsList)
	args := make([]Value, n)
	for i := 0; i < n; i++ {
		args[i] = arrayLikeGet(argsList, i)
	}
	return target.Call(args...)
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
			d.set.Call(val)
			return true
		}
	}
	for cur := o.proto; cur != nil; cur = cur.proto {
		if d, ok := cur.getSymDesc(key); ok {
			if d.isAccessor() {
				if d.set.kind != KindFunc {
					return false
				}
				d.set.Call(val)
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
