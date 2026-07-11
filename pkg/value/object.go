// This file owns Object, the reference type behind the object and array kinds of
// a boxed Value (10_value_model section 6). The spec's Object uses hidden-class
// shapes so a property read is a shape check plus an index rather than a map
// probe, which is the dynamic world's single biggest speed lever. This first cut
// keeps an ordered property map instead: it is behaviorally identical, preserving
// insertion order the way JavaScript does, and the shape machinery is a later
// performance slice that does not change any observable result.

package value

// Object is the storage behind a KindObject or KindArray value. A plain object
// keeps its properties in insertion order as parallel key and value slices, the
// order JavaScript enumerates and serializes in. An array keeps its elements in a
// dense slice, separate from named properties, because indices are hot and must
// not go through the property map. One struct backs both so an array can still
// carry a named property without changing representation.
type Object struct {
	kind  Kind    // KindObject or KindArray
	keys  []BStr  // property names in insertion order (named properties)
	vals  []Value // property values, parallel to keys
	elems []Value // dense element storage for an array
	call  callFn  // the invocable body of a callable, nil for a plain object
}

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
			o.vals[i] = val
			return v
		}
	}
	o.keys = append(o.keys, key)
	o.vals = append(o.vals, val)
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
			o.vals = append(o.vals[:i], o.vals[i+1:]...)
			return true
		}
	}
	return true
}

// getOwn returns the value of a named own property, or undefined when the object
// has no such key, the JavaScript result for a missing property. The lookup is a
// linear scan of the ordered keys, which the shape machinery will later replace
// with a shape check and an index.
func (o *Object) getOwn(key BStr) Value {
	for i := range o.keys {
		if o.keys[i].Equal(key) {
			return o.vals[i]
		}
	}
	return Undefined
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
