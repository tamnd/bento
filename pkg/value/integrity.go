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
