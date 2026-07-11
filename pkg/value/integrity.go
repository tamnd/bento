// This file owns the integrity levels at runtime: the extensibility flag and the
// seal and freeze sweeps over an object's descriptors, plus the predicates that
// read that state back (07 group 5). Each mutator turns a switch or a per-property
// flag off and returns its argument, and each predicate reports the derived state
// the spec's TestIntegrityLevel computes. A primitive has no state to change, so a
// mutator hands it back untouched and a predicate reports the answer the spec gives
// for a non-object: not extensible, and both sealed and frozen.

package value

// PreventExtensions clears the receiver's extensible flag so no new property can be
// added, the runtime behind Object.preventExtensions(o). The object's existing
// properties are untouched; only the addition of a new key is blocked, which the
// Set and element-write paths honor. A non-object receiver has no flag to clear and
// is returned unchanged, the no-op Object.preventExtensions performs on a primitive.
func (v Value) PreventExtensions() Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		v.object().nonExtensible = true
	}
	return v
}

// Seal prevents extensions and marks every own property non-configurable, so no
// property can be added, removed, or redefined, the runtime behind Object.seal(o).
// A sealed property keeps its value and writability, so a data property can still be
// assigned; only its configurability is cleared. An array's elements are marked
// non-configurable through the elemsSealed flag, since they carry no per-element
// descriptor. A non-object receiver has nothing to seal and is returned unchanged.
func (v Value) Seal() Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return v
	}
	o := v.object()
	o.nonExtensible = true
	for i := range o.descs {
		o.descs[i].configurable = false
	}
	for i := range o.symDescs {
		o.symDescs[i].configurable = false
	}
	if v.kind == KindArray {
		o.elemsSealed = true
	}
	return v
}

// Freeze seals the object and additionally marks every own data property
// non-writable, so no property can be added, removed, redefined, or reassigned, the
// runtime behind Object.freeze(o). An accessor property has no value to lock, so its
// getter and setter are left in place; only its configurability is cleared, by the
// seal. An array's elements are marked non-writable through the elemsFrozen flag, so
// an element write drops. A non-object receiver has nothing to freeze and is
// returned unchanged.
func (v Value) Freeze() Value {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return v
	}
	o := v.object()
	o.nonExtensible = true
	for i := range o.descs {
		o.descs[i].configurable = false
		if o.descs[i].isData() {
			o.descs[i].writable = false
		}
	}
	for i := range o.symDescs {
		o.symDescs[i].configurable = false
		if o.symDescs[i].isData() {
			o.symDescs[i].writable = false
		}
	}
	if v.kind == KindArray {
		o.elemsSealed = true
		o.elemsFrozen = true
	}
	return v
}

// IsExtensible reports whether new properties may still be added to the receiver,
// the runtime behind Object.isExtensible(o). A non-object is never extensible, the
// answer the spec gives for a primitive, which has no properties to add.
func (v Value) IsExtensible() bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().isExtensible()
	default:
		return false
	}
}

// sealed reports whether the object is sealed: not extensible and every own
// property non-configurable, the state Object.seal leaves and TestIntegrityLevel
// checks for "sealed". An array with elements is sealed only once its elements are
// marked non-configurable too, which the elemsSealed flag records. The kind is
// passed in because only an array carries element state apart from the descriptor
// bag.
func (o *Object) sealed(kind Kind) bool {
	if o.isExtensible() {
		return false
	}
	for i := range o.descs {
		if o.descs[i].configurable {
			return false
		}
	}
	for i := range o.symDescs {
		if o.symDescs[i].configurable {
			return false
		}
	}
	if kind == KindArray && len(o.elems) > 0 && !o.elemsSealed {
		return false
	}
	return true
}

// IsSealed reports whether the receiver is sealed, the runtime behind
// Object.isSealed(o). A non-object is treated as sealed, the answer the spec gives
// for a primitive, which has no property to configure.
func (v Value) IsSealed() bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().sealed(v.kind)
	default:
		return true
	}
}

// frozen reports whether the object is frozen: sealed and every own data property
// non-writable, the state Object.freeze leaves and TestIntegrityLevel checks for
// "frozen". An accessor property has no writability to check, so only data
// properties are examined. An array with elements is frozen only once its elements
// are marked non-writable too, which the elemsFrozen flag records.
func (o *Object) frozen(kind Kind) bool {
	if !o.sealed(kind) {
		return false
	}
	for i := range o.descs {
		if o.descs[i].isData() && o.descs[i].writable {
			return false
		}
	}
	for i := range o.symDescs {
		if o.symDescs[i].isData() && o.symDescs[i].writable {
			return false
		}
	}
	if kind == KindArray && len(o.elems) > 0 && !o.elemsFrozen {
		return false
	}
	return true
}

// IsFrozen reports whether the receiver is frozen, the runtime behind
// Object.isFrozen(o). A non-object is treated as frozen, the answer the spec gives
// for a primitive, which has no property to write.
func (v Value) IsFrozen() bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().frozen(v.kind)
	default:
		return true
	}
}
