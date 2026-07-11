// This file owns Object.defineProperty at runtime: reading a descriptor object
// into a stored descriptor and applying it to a key on the bag. A descriptor
// object carries any of value, writable, get, set, enumerable, and configurable,
// and the ones it leaves out default to false or undefined on a fresh define and
// keep their current setting on a redefine, the merge ECMAScript's
// [[DefineOwnProperty]] performs. The extensibility and configurability invariants
// that turn an illegal redefine into a TypeError are layered on separately; this
// file does the field reading and the value application.

package value

// descriptorInput is the set of attributes a descriptor object provided, each
// paired with a present flag so a redefine keeps the attributes the caller left
// out and a fresh define fills them with the spec's all-false defaults.
type descriptorInput struct {
	value        Value
	get          Value
	set          Value
	writable     bool
	enumerable   bool
	configurable bool

	hasValue        bool
	hasGet          bool
	hasSet          bool
	hasWritable     bool
	hasEnumerable   bool
	hasConfigurable bool
}

// readDescriptorInput extracts the attributes a descriptor object carries, the
// ToPropertyDescriptor step of Object.defineProperty. Each field is read only when
// the object has the matching property, so an omitted attribute stays absent and
// its present flag stays false; the boolean attributes go through ToBoolean the way
// the spec coerces them.
func readDescriptorInput(descObj Value) descriptorInput {
	var in descriptorInput
	if descObj.HasProperty(FromGoString("enumerable")) {
		in.hasEnumerable = true
		in.enumerable = ToBoolean(descObj.Get(FromGoString("enumerable")))
	}
	if descObj.HasProperty(FromGoString("configurable")) {
		in.hasConfigurable = true
		in.configurable = ToBoolean(descObj.Get(FromGoString("configurable")))
	}
	if descObj.HasProperty(FromGoString("value")) {
		in.hasValue = true
		in.value = descObj.Get(FromGoString("value"))
	}
	if descObj.HasProperty(FromGoString("writable")) {
		in.hasWritable = true
		in.writable = ToBoolean(descObj.Get(FromGoString("writable")))
	}
	if descObj.HasProperty(FromGoString("get")) {
		in.hasGet = true
		in.get = descObj.Get(FromGoString("get"))
	}
	if descObj.HasProperty(FromGoString("set")) {
		in.hasSet = true
		in.set = descObj.Get(FromGoString("set"))
	}
	return in
}

// toDescriptor folds the input onto the current descriptor and returns the
// descriptor to store. On a fresh define the base is the zero descriptor, a
// non-writable, non-enumerable, non-configurable data property holding undefined,
// so every attribute the caller omits defaults to false or undefined. On a
// redefine the base is the existing descriptor, so an omitted attribute keeps its
// current setting. A descriptor that names get or set becomes an accessor and a
// descriptor that names value or writable becomes a data property, converting the
// stored kind when they disagree, the way the spec turns a data property into an
// accessor and back.
func (in descriptorInput) toDescriptor(current descriptor, exists bool) descriptor {
	base := descriptor{}
	if exists {
		base = current
	}
	switch {
	case in.hasGet || in.hasSet:
		base.accessor = true
	case in.hasValue || in.hasWritable:
		base.accessor = false
	}
	if base.accessor {
		if in.hasGet {
			base.get = in.get
		}
		if in.hasSet {
			base.set = in.set
		}
		base.value = Undefined
		base.writable = false
	} else {
		if in.hasValue {
			base.value = in.value
		}
		if in.hasWritable {
			base.writable = in.writable
		}
		base.get = Undefined
		base.set = Undefined
	}
	if in.hasEnumerable {
		base.enumerable = in.enumerable
	}
	if in.hasConfigurable {
		base.configurable = in.configurable
	}
	return base
}

// DefineProperty applies a descriptor object to a key on the receiver and returns
// the receiver, the runtime behind Object.defineProperty(o, key, desc). A symbol
// key defines onto the symbol bag; any other key takes its property-key string and
// defines onto the named bag. The define is validated against the object's
// extensibility and the existing property's configurability first, and a change
// the spec forbids throws a TypeError rather than mutating the bag. A non-object
// receiver throws a TypeError the way the spec rejects a primitive target.
func (v Value) DefineProperty(key, descObj Value) Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		Throw(NewTypeError(FromGoString("Object.defineProperty called on non-object")))
		return v
	}
	in := readDescriptorInput(descObj)
	o := v.object()
	if key.kind == KindSymbol {
		sym := key.symbol()
		current, exists := o.getSymDesc(sym)
		if !validateDefine(current, exists, in, o.isExtensible()) {
			Throw(NewTypeError(FromGoString("Cannot redefine property")))
			return v
		}
		o.defineSym(sym, in.toDescriptor(current, exists))
		return v
	}
	var name BStr
	if key.kind == KindString {
		name = key.str()
	} else {
		name = ToString(key)
	}
	current, exists := o.getOwnDesc(name)
	if !validateDefine(current, exists, in, o.isExtensible()) {
		Throw(NewTypeError(FromGoString("Cannot redefine property: ").ConcatN(name)))
		return v
	}
	o.defineOwn(name, in.toDescriptor(current, exists))
	return v
}

// validateDefine reports whether a define is allowed, the rejection half of
// ValidateAndApplyPropertyDescriptor. A property that does not yet exist may be
// added only when the object is extensible. An existing configurable property
// accepts any redefine. An existing non-configurable property rejects a change
// that would make it configurable, flip its enumerable flag, change its kind
// between data and accessor, or, when it is a non-writable data property, raise
// its writable flag or replace its value; a non-configurable accessor likewise
// rejects a getter or setter swap. A descriptor that names none of the value,
// writable, get, or set fields is a generic redefine of only the shared flags,
// which the configurable and enumerable checks above already cover.
func validateDefine(current descriptor, exists bool, in descriptorInput, extensible bool) bool {
	if !exists {
		return extensible
	}
	if current.configurable {
		return true
	}
	if in.hasConfigurable && in.configurable {
		return false
	}
	if in.hasEnumerable && in.enumerable != current.enumerable {
		return false
	}
	wantsAccessor := in.hasGet || in.hasSet
	wantsData := in.hasValue || in.hasWritable
	if !wantsAccessor && !wantsData {
		return true
	}
	if wantsAccessor && !current.accessor {
		return false
	}
	if wantsData && current.accessor {
		return false
	}
	if current.accessor {
		if in.hasGet && !StrictEquals(in.get, current.get) {
			return false
		}
		if in.hasSet && !StrictEquals(in.set, current.set) {
			return false
		}
		return true
	}
	if !current.writable {
		if in.hasWritable && in.writable {
			return false
		}
		if in.hasValue && !sameValue(in.value, current.value) {
			return false
		}
	}
	return true
}

// sameValue is the SameValue comparison the define invariants use to tell whether
// a redefine changes a non-writable property's value. It parts from strict
// equality only on numbers, where it treats two NaNs as equal and the signed zeros
// as distinct, so replacing a value with an equal one is not a change.
func sameValue(a, b Value) bool {
	if a.kind == KindNumber && b.kind == KindNumber {
		return NumberSameValue(a.AsNumber(), b.AsNumber())
	}
	return StrictEquals(a, b)
}

// DefineProperties applies a map of descriptor objects to the receiver and returns
// it, the runtime behind Object.defineProperties(o, props). It walks props's own
// enumerable properties, string keys in enumeration order then symbol keys in
// insertion order, and defines each onto the receiver through the same path
// Object.defineProperty takes, so the batched form and the single form share one
// definition. A non-object receiver or a nullish props throws a TypeError the way
// the spec rejects them.
func (v Value) DefineProperties(props Value) Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		Throw(NewTypeError(FromGoString("Object.defineProperties called on non-object")))
		return v
	}
	switch props.kind {
	case KindObject, KindArray, KindFunc:
	default:
		Throw(NewTypeError(FromGoString("Object.defineProperties called with a non-object descriptor map")))
		return v
	}
	p := props.object()
	for _, name := range p.orderedStringKeysFiltered(true) {
		v.DefineProperty(StringValue(name), props.Get(name))
	}
	for i := range p.symKeys {
		if p.symDescs[i].enumerable {
			v.DefineProperty(symbolValue(p.symKeys[i]), p.getSym(props, p.symKeys[i]))
		}
	}
	return v
}

// defineOwn stores a descriptor at a named key, overwriting the slot in place when
// the key already exists and appending in insertion order when it is new, the
// store defineProperty makes once it has settled the descriptor.
func (o *Object) defineOwn(key BStr, d descriptor) {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			o.descs[i] = d
			return
		}
	}
	o.keys = append(o.keys, key)
	o.descs = append(o.descs, d)
}

// defineSym stores a descriptor at a symbol key, the symbol mirror of defineOwn,
// keyed by the symbol's identity so it never collides with a string key.
func (o *Object) defineSym(key *Symbol, d descriptor) {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			o.symDescs[i] = d
			return
		}
	}
	o.symKeys = append(o.symKeys, key)
	o.symDescs = append(o.symDescs, d)
}
