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
