// This file owns the prototype chain at runtime: creating an object with a chosen
// prototype and reading and writing an object's [[Prototype]] slot. A dynamic
// object carries a proto pointer, the object a property read climbs to when the
// own bag misses (07 group 4). The default plain object has no user prototype set,
// so its slot is nil; Object.create and Object.setPrototypeOf are what put a real
// object or an explicit null there.

package value

// ObjectCreate returns a new plain object whose [[Prototype]] is proto, the runtime
// behind Object.create(proto). An object prototype is stored in the new object's
// slot so a later read climbs into it. The result is a fresh, extensible object
// with no own properties, the target Object.create hands back before an optional
// descriptor map is applied. A prototype that is neither an object nor null throws
// a TypeError the way the spec rejects it; the null case is layered on separately.
func ObjectCreate(proto Value) Value {
	o := &Object{kind: KindObject}
	switch proto.kind {
	case KindObject, KindArray, KindFunc:
		o.proto = proto.object()
	default:
		Throw(NewTypeError(FromGoString("Object prototype may only be an Object or null")))
		return Undefined
	}
	return objectValue(o)
}
