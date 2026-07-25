// This file owns the prototype chain at runtime: creating an object with a chosen
// prototype and reading and writing an object's [[Prototype]] slot. A dynamic
// object carries a proto pointer, the object a property read climbs to when the
// own bag misses (07 group 4). The default plain object has no user prototype set,
// so its slot is nil; Object.create and Object.setPrototypeOf are what put a real
// object or an explicit null there.

package value

// ObjectCreate returns a new plain object whose [[Prototype]] is proto, the runtime
// behind Object.create(proto). An object prototype is stored in the new object's
// slot so a later read climbs into it, and a null prototype leaves the slot nil so
// the object is prototype-less and a read never climbs past its own bag. The result
// is a fresh, extensible object with no own properties, the target Object.create
// hands back before an optional descriptor map is applied. A prototype that is
// neither an object nor null throws a TypeError the way the spec rejects it.
func ObjectCreate(proto Value) Value {
	o := &Object{kind: KindObject}
	switch proto.kind {
	case KindObject, KindArray, KindFunc:
		o.proto = proto.object()
	case KindNull:
		o.proto = nil
		o.protoNull = true
	default:
		Throw(NewTypeError(FromGoString("Object prototype may only be an Object or null")))
		return Undefined
	}
	return objectValue(o)
}

// SetPrototype writes the receiver's [[Prototype]] slot and returns the receiver,
// the runtime behind Object.setPrototypeOf(o, proto). An object becomes the new
// prototype and null clears the slot; a prototype that is neither an object nor
// null throws a TypeError the way the spec rejects it. Changing the prototype of a
// non-extensible object to a different one throws a TypeError, while setting it to
// the value it already holds is allowed and leaves the object untouched. A
// non-object receiver has no slot to write, so it is returned unchanged.
func (v Value) SetPrototype(proto Value) Value {
	if p := v.asProxy(); p != nil {
		p.setPrototypeOf(proto)
		return v
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return v
	}
	o := v.object()
	// The target's [[Prototype]] identity is the new object, or explicit null. A change
	// is rejected on a non-extensible object; setting the slot to the value it already
	// holds is a no-op the spec allows even when non-extensible. Because a nil proto
	// slot means both the default and an explicit null, the protoNull flag tells the
	// two apart so setting a default-prototype object to null is seen as the real change
	// it is, not mistaken for a no-op the way a bare nil == nil test would.
	switch proto.kind {
	case KindObject, KindArray, KindFunc:
		np := proto.object()
		if o.proto == np && !o.protoNull {
			return v
		}
		if !o.isExtensible() {
			Throw(NewTypeError(FromGoString("#<Object> is not extensible")))
			return v
		}
		o.proto = np
		o.protoNull = false
		return v
	case KindNull:
		if o.proto == nil && o.protoNull {
			return v
		}
		if !o.isExtensible() {
			Throw(NewTypeError(FromGoString("#<Object> is not extensible")))
			return v
		}
		o.proto = nil
		o.protoNull = true
		return v
	default:
		Throw(NewTypeError(FromGoString("Object prototype may only be an Object or null")))
		return v
	}
}

// SetProtoAssign applies the legacy __proto__ assignment, the runtime shared by the
// object literal __proto__: member and the o.__proto__ = v accessor. An object or
// null becomes the prototype through the same slot write Object.setPrototypeOf
// takes, so a non-extensible object still rejects a real change with a TypeError;
// any other value is ignored without error, the way both __proto__ forms leave a
// primitive prototype alone rather than storing an own property of that name. It
// returns the receiver so the object-literal builder keeps chaining Set calls.
func (v Value) SetProtoAssign(proto Value) Value {
	switch proto.kind {
	case KindObject, KindArray, KindFunc, KindNull:
		return v.SetPrototype(proto)
	default:
		return v
	}
}

// ProtoRead is the read of the legacy `obj.__proto__` member. The name __proto__
// is an accessor inherited from Object.prototype, so an own property of that name
// shadows it: JSON.parse and CreateDataProperty install an own "__proto__" data
// property that a later obj.__proto__ read must return in place of the prototype
// (see JSON/parse/duplicate-proto). Only when the receiver carries no own __proto__
// does the read fall through to the accessor's [[Prototype]] result.
func (v Value) ProtoRead() Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		if o := v.object(); o.hasOwn(FromGoString("__proto__")) {
			return v.Get(FromGoString("__proto__"))
		}
	}
	return v.GetPrototype()
}

// GetPrototype returns the receiver's [[Prototype]] as a value, the runtime behind
// Object.getPrototypeOf(o). A slot holding an object reports that object; a slot
// left nil, whether never set or set to null through Object.create(null), reports
// null. A non-object receiver has no slot this model tracks, so it reports null too.
func (v Value) GetPrototype() Value {
	if p := v.asProxy(); p != nil {
		return p.getPrototypeOf()
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		if o := v.object(); o.proto != nil {
			return objectValue(o.proto)
		}
		return Null
	default:
		return Null
	}
}
