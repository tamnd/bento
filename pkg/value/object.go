// This file owns Object, the reference type behind the object and array kinds of
// a boxed Value (10_value_model section 6). The spec's Object uses hidden-class
// shapes so a property read is a shape check plus an index rather than a map
// probe, which is the dynamic world's single biggest speed lever. This first cut
// keeps an ordered property map instead: it is behaviorally identical, preserving
// insertion order the way JavaScript does, and the shape machinery is a later
// performance slice that does not change any observable result.

package value

import "sort"

// Object is the storage behind a KindObject or KindArray value. A plain object
// keeps its properties in insertion order as parallel key and value slices, the
// order JavaScript enumerates and serializes in. An array keeps its elements in a
// dense slice, separate from named properties, because indices are hot and must
// not go through the property map. One struct backs both so an array can still
// carry a named property without changing representation.
type Object struct {
	kind          Kind         // KindObject or KindArray
	keys          []BStr       // string property names in insertion order (named properties)
	descs         []descriptor // property descriptors, parallel to keys
	symKeys       []*Symbol    // symbol property keys in insertion order, kept apart from string keys
	symDescs      []descriptor // symbol property descriptors, parallel to symKeys
	elems         []Value      // dense element storage for an array
	call          callFn       // the invocable body of a callable, nil for a plain object
	proto         *Object      // the [[Prototype]] a read climbs on an own miss; nil is the end of the user chain
	nonExtensible bool         // set once Object.preventExtensions blocks new keys; zero value is extensible
}

// isExtensible reports whether new properties may still be added, the state
// Object.isExtensible reads and Object.preventExtensions clears. The flag is
// stored inverted so a freshly built object is extensible with no constructor
// having to set it.
func (o *Object) isExtensible() bool { return !o.nonExtensible }

// callFn is the body of a callable function value: it takes its arguments already
// boxed and returns a boxed result, so a dynamic call site invokes it uniformly
// without knowing the static signature the function had before it was boxed. A
// missing trailing argument is not the callee's concern; the call site fills the
// slot with undefined the way the language binds an omitted parameter, so the body
// reads a fixed-length view.
type callFn func(args []Value) Value

// NewFunc boxes a Go closure as a callable function value, the box a static
// function takes when it flows into a dynamic slot so a dynamic call site can
// invoke it. A function is an object too (it can carry properties like name and
// length), so it rides the same Object storage as a plain object with the call
// body set; the kind stays KindFunc so typeof reports "function" and a property
// read still finds the object's own keys.
func NewFunc(fn callFn) Value {
	return objectValue(&Object{kind: KindFunc, call: fn})
}

// WithName records name as a function value's own name property and returns the
// value, the effect named evaluation has when an anonymous function is assigned to
// an identifier: value = function() {} binds the function's name to "value". The
// name rides the function's own "name" property, so a later read of f.name returns
// it the way Function.prototype.name does. A non-function value is returned
// untouched, since only a function carries a name.
func WithName(f Value, name string) Value {
	if f.kind == KindFunc {
		f.Set(FromGoString("name"), StringValue(FromGoString(name)))
	}
	return f
}

// Arg returns the ith boxed argument, or undefined when the call passed fewer, the
// value JavaScript binds to a parameter the caller omitted. A boxed callable reads
// its arguments through this helper so its body never indexes past the slice a
// short call hands it.
func Arg(args []Value, i int) Value {
	if i >= 0 && i < len(args) {
		return args[i]
	}
	return Undefined
}

// NewObject returns an empty plain object value, the target JSON.parse builds a
// key at a time as it reads an object literal.
func NewObject() Value {
	return objectValue(&Object{kind: KindObject})
}

// NewArrayValue returns an array value holding the given elements, the target
// JSON.parse builds as it reads an array literal. The elements are taken as given,
// in order, so the array's indices match the source order.
func NewArrayValue(elems []Value) Value {
	return objectValue(&Object{kind: KindArray, elems: elems})
}

// Set writes a named property, appending it in insertion order if the key is new
// and overwriting in place if it already exists, so a repeated key keeps its first
// position the way JavaScript's own property order does. It returns the receiver
// value so JSON.parse can build an object in an expression.
func (v Value) Set(key BStr, val Value) Value {
	o := v.object()
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			o.descs[i] = o.descs[i].write(v, val)
			return v
		}
	}
	o.keys = append(o.keys, key)
	o.descs = append(o.descs, defaultDataProperty(val))
	return v
}

// SetKey writes v[key] = val by the receiver's kind, the store mirror of the
// kind-aware Get read. An array claims a numeric key into its dense element
// storage, growing the slice with undefined holes so a[5] = x on a shorter array
// leaves the gap the way JavaScript does, and a non-numeric key lands in the named
// property map an array can still carry. An object and a function store the key as
// a named property through Set. It returns val so a bracket write can sit in an
// expression, the way JavaScript's assignment evaluates to its right-hand side. A
// kind with no writable storage drops the write and returns val, the value the
// language still hands back.
func (v Value) SetKey(key BStr, val Value) Value {
	switch v.kind {
	case KindArray:
		o := v.object()
		if idx, ok := arrayIndex(key.ToGoString()); ok {
			for len(o.elems) <= idx {
				o.elems = append(o.elems, Undefined)
			}
			o.elems[idx] = val
			return val
		}
		v.Set(key, val)
		return val
	case KindObject, KindFunc:
		v.Set(key, val)
		return val
	default:
		return val
	}
}

// Call invokes a callable function value with the given boxed arguments, the
// lowering of fn(args) when the callee's type is dynamic so its shape is known
// only at runtime. A callable runs its boxed body; any other value throws a
// TypeError the way JavaScript does when a call target turns out not to be a
// function.
func (v Value) Call(args ...Value) Value {
	if v.kind == KindFunc {
		if o := v.object(); o.call != nil {
			return o.call(args)
		}
	}
	Throw(NewTypeError(v.TypeOf().ConcatN(FromGoString(" is not a function"))))
	return Undefined
}

// Delete removes v[key] by the receiver's kind, the runtime behind the delete
// operator, and reports the boolean delete yields. An array clears a numeric key
// to a hole rather than shifting the later elements, the way delete a[i] leaves a
// gap without changing length, and a non-numeric key falls to the named property
// map an array can still carry. An object and a function drop the own key through
// deleteOwn. A primitive receiver has no own property this path stores, so there
// is nothing to remove and the result is true, the value delete gives for a
// property that is absent. Every property this model creates is configurable, so
// a removal never fails and the result is always true; a non-configurable
// property comes only from a descriptor the object model does not build yet.
func (v Value) Delete(key BStr) bool {
	switch v.kind {
	case KindArray:
		o := v.object()
		if idx, ok := arrayIndex(key.ToGoString()); ok {
			if idx >= 0 && idx < len(o.elems) {
				o.elems[idx] = Undefined
			}
			return true
		}
		return o.deleteOwn(key)
	case KindObject, KindFunc:
		return v.object().deleteOwn(key)
	default:
		return true
	}
}

// deleteOwn removes a named own property, closing the gap in the parallel key and
// value slices so the remaining properties keep their insertion order. A key the
// object does not carry is already absent, so the result is true either way, the
// boolean the delete operator gives for a configurable or missing property.
func (o *Object) deleteOwn(key BStr) bool {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			o.descs = append(o.descs[:i], o.descs[i+1:]...)
			return true
		}
	}
	return true
}

// getOwn returns the value of a named own property, or undefined when the object
// has no such key, the JavaScript result for a missing property. The value comes
// through the property's descriptor, so a data property reports its stored value
// and an accessor property runs its getter with recv as the receiver. The lookup
// is a linear scan of the ordered keys, which the shape machinery will later
// replace with a shape check and an index.
func (o *Object) getOwn(recv Value, key BStr) Value {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			return o.descs[i].read(recv)
		}
	}
	return Undefined
}

// getOwnDesc returns the descriptor of a named own property and whether the object
// carries it, the raw read Object.defineProperty and Object.getOwnPropertyDescriptor
// build on, distinct from getOwn which resolves the descriptor to a value.
func (o *Object) getOwnDesc(key BStr) (descriptor, bool) {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			return o.descs[i], true
		}
	}
	return descriptor{}, false
}

// hasOwn reports whether the object carries key as an own named property, the
// existence probe the in operator makes instead of reading the value, so a property
// present with an undefined value still reports true where getOwn could not tell it
// apart from a miss.
func (o *Object) hasOwn(key BStr) bool {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			return true
		}
	}
	return false
}

// setSym writes a symbol-keyed property, keyed by the symbol's identity rather
// than by any string form, so a symbol key never collides with a string key or
// with another symbol of the same description. A key already present is
// overwritten in place; a new key appends, keeping the symbol properties in
// insertion order the way the spec enumerates them after the string keys.
func (o *Object) setSym(recv Value, key *Symbol, val Value) {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			o.symDescs[i] = o.symDescs[i].write(recv, val)
			return
		}
	}
	o.symKeys = append(o.symKeys, key)
	o.symDescs = append(o.symDescs, defaultDataProperty(val))
}

// getSym returns the value of a symbol-keyed own property, or undefined when the
// object carries no such symbol, the JavaScript result for a missing property.
func (o *Object) getSym(recv Value, key *Symbol) Value {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			return o.symDescs[i].read(recv)
		}
	}
	return Undefined
}

// getSymDesc returns the descriptor of a symbol-keyed own property and whether the
// object carries it, the symbol mirror of getOwnDesc that defineProperty reads to
// merge a redefine into the existing descriptor.
func (o *Object) getSymDesc(key *Symbol) (descriptor, bool) {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			return o.symDescs[i], true
		}
	}
	return descriptor{}, false
}

// hasSym reports whether the object carries key as an own symbol property, the
// existence probe behind a symbol key in the in operator.
func (o *Object) hasSym(key *Symbol) bool {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			return true
		}
	}
	return false
}

// deleteSym removes a symbol-keyed own property, closing the gap in the parallel
// symbol key and value slices so the remaining symbol properties keep their
// insertion order. A symbol the object does not carry is already absent, so the
// result is true either way, the boolean delete gives for a configurable or
// missing property.
func (o *Object) deleteSym(key *Symbol) bool {
	for i := range o.symKeys {
		if o.symKeys[i] == key {
			o.symKeys = append(o.symKeys[:i], o.symKeys[i+1:]...)
			o.symDescs = append(o.symDescs[:i], o.symDescs[i+1:]...)
			return true
		}
	}
	return true
}

// orderedStringKeys returns the object's own string-keyed properties in the order
// the specification enumerates them: canonical integer-index keys first in
// ascending numeric order, then the remaining string keys in insertion order. An
// array contributes its dense element indices, which are integer keys by
// construction, and a plain object that was given numeric string keys out of order
// still enumerates them ascending, so the order is deterministic regardless of how
// the keys arrived.
func (o *Object) orderedStringKeys() []BStr {
	return o.orderedStringKeysFiltered(false)
}

// orderedStringKeysFiltered returns the own string keys in the spec's enumeration
// order, optionally dropping the non-enumerable ones. Object.getOwnPropertyNames
// wants every own string key, so it passes enumerableOnly false; Object.keys and
// Object.values want only the enumerable ones, so they pass true. An array's dense
// element indices are always enumerable data properties, so they are kept either
// way; a named property's enumerability comes from its descriptor.
func (o *Object) orderedStringKeysFiltered(enumerableOnly bool) []BStr {
	var idxKeys []int
	var strKeys []BStr
	for i := range o.elems {
		idxKeys = append(idxKeys, i)
	}
	for i, k := range o.keys {
		if enumerableOnly && !o.descs[i].enumerable {
			continue
		}
		if n, ok := arrayIndex(k.ToGoString()); ok {
			idxKeys = append(idxKeys, n)
		} else {
			strKeys = append(strKeys, k)
		}
	}
	sort.Ints(idxKeys)
	out := make([]BStr, 0, len(idxKeys)+len(strKeys))
	for _, n := range idxKeys {
		out = append(out, NumberToString(float64(n)))
	}
	return append(out, strKeys...)
}

// ObjectRest returns a new plain object holding the receiver's own enumerable
// properties except those named in omit, the value an object rest element binds:
// { a, ...rest } gathers every own property but a. The properties copy in the
// spec's own-property order, integer indices ascending then the remaining string
// keys in insertion order, so the rest object enumerates the way the source does. A
// receiver with no object storage yields an empty object, the rest of nothing.
func (v Value) ObjectRest(omit ...BStr) Value {
	rest := NewObject()
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return rest
	}
	o := v.object()
	skip := func(k BStr) bool {
		for _, om := range omit {
			if om.Equal(k) {
				return true
			}
		}
		return false
	}
	for _, k := range o.orderedStringKeys() {
		if !skip(k) {
			rest.Set(k, v.Get(k))
		}
	}
	return rest
}

// OwnKeys returns the receiver's own string-keyed property names as a string array
// in the spec's enumeration order, including the non-enumerable ones, the value
// Object.getOwnPropertyNames builds for a dynamic receiver whose keys are known
// only at runtime. Symbol keys never appear, which matches the string-side static.
// A receiver with no object storage yields an empty array.
func (v Value) OwnKeys() *Array[BStr] {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return NewArray(v.object().orderedStringKeysFiltered(false)...)
	default:
		return NewArray[BStr]()
	}
}

// OwnEnumerableKeys returns the receiver's own enumerable string-keyed property
// names in the spec's enumeration order, the value Object.keys builds for a dynamic
// receiver. It differs from OwnKeys only in that a property defined non-enumerable
// through Object.defineProperty is left out. A receiver with no object storage
// yields an empty array.
func (v Value) OwnEnumerableKeys() *Array[BStr] {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return NewArray(v.object().orderedStringKeysFiltered(true)...)
	default:
		return NewArray[BStr]()
	}
}

// OwnValues returns the receiver's own enumerable property values as a value array
// in the same order OwnEnumerableKeys walks the names, the value Object.values
// builds for a dynamic receiver. A non-enumerable property contributes no value,
// matching Object.keys. A receiver with no object storage yields an empty array.
func (v Value) OwnValues() *Array[Value] {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		keys := v.object().orderedStringKeysFiltered(true)
		vals := make([]Value, len(keys))
		for i, k := range keys {
			vals[i] = v.Get(k)
		}
		return NewArray(vals...)
	default:
		return NewArray[Value]()
	}
}

// HasOwnElem reports whether the receiver carries key as an own property, the
// value Object.hasOwn returns for a dynamic receiver. A symbol key is looked up by
// identity in the symbol bag; any other key is taken to its property-key string,
// so o.hasOwn(s) and o.hasOwn("k") each probe the slot the matching read would
// reach. An array answers for its length and its in-range element indices as well
// as any named property, and a receiver with no object storage has nothing to own.
func (v Value) HasOwnElem(key Value) bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return false
	}
	o := v.object()
	if key.kind == KindSymbol {
		return o.hasSym(key.symbol())
	}
	var name BStr
	if key.kind == KindString {
		name = key.str()
	} else {
		name = ToString(key)
	}
	if v.kind == KindArray {
		s := name.ToGoString()
		if s == "length" {
			return true
		}
		if idx, ok := arrayIndex(s); ok {
			return idx < len(o.elems)
		}
	}
	return o.hasOwn(name)
}
