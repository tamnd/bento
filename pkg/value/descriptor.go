// This file owns the property descriptor, the full state of one property in an
// object's bag. The spec models every property as either a data property, which
// carries a value and a writable flag, or an accessor property, which carries a
// getter and setter pair; both kinds carry the enumerable and configurable
// attributes. Object.defineProperty, the integrity operations, and the whole of
// propertyHelper.js observe these attributes directly, so the bag stores a
// descriptor per key rather than a bare value. A plain assignment still creates
// the default data descriptor (writable, enumerable, and configurable all true),
// so an object built by ordinary writes behaves exactly as it did before.

package value

// descriptor is the stored state of one own property. It is a data descriptor
// when accessor is false, in which case value and writable are live and get and
// set are ignored, and an accessor descriptor when accessor is true, in which
// case get and set are live and value and writable are ignored. The enumerable
// and configurable flags apply to both kinds. The zero descriptor is a
// non-writable, non-enumerable, non-configurable data property holding undefined,
// which matches the spec's all-false defaults for a freshly defined property.
type descriptor struct {
	value        Value
	get          Value
	set          Value
	writable     bool
	enumerable   bool
	configurable bool
	accessor     bool
}

// dataProperty returns a data descriptor with the given value and flags, the
// shape Object.defineProperty builds for a { value, writable } descriptor and the
// shape a plain assignment builds with every flag true.
func dataProperty(value Value, writable, enumerable, configurable bool) descriptor {
	return descriptor{
		value:        value,
		writable:     writable,
		enumerable:   enumerable,
		configurable: configurable,
	}
}

// defaultDataProperty returns the descriptor a plain property write creates: a
// data property that is writable, enumerable, and configurable, the attributes
// the language gives a property assigned with o.k = v or an object literal.
func defaultDataProperty(value Value) descriptor {
	return dataProperty(value, true, true, true)
}

// accessorProperty returns an accessor descriptor with the given getter and
// setter and flags, the shape Object.defineProperty builds for a { get, set }
// descriptor. A missing getter or setter is passed as undefined.
func accessorProperty(get, set Value, enumerable, configurable bool) descriptor {
	return descriptor{
		get:          get,
		set:          set,
		enumerable:   enumerable,
		configurable: configurable,
		accessor:     true,
	}
}

// isAccessor reports whether the descriptor is an accessor property, so a read
// runs its getter and a write runs its setter, rather than a data property whose
// value is read and written directly.
func (d descriptor) isAccessor() bool { return d.accessor }

// isData reports whether the descriptor is a data property, the complement of
// isAccessor, the shape a read returns value from and a write updates in place.
func (d descriptor) isData() bool { return !d.accessor }

// read returns the value a property read yields for this descriptor, given the
// receiver the read went through. A data property reads its stored value. An
// accessor property runs its getter, or reports undefined when it has none, the
// value a get with no getter gives; the receiver is passed to the getter so a
// getter that leans on its this argument still sees the object it was read from.
func (d descriptor) read(receiver Value) Value {
	if !d.accessor {
		return d.value
	}
	if d.get.kind == KindFunc {
		return d.get.Call(receiver)
	}
	return Undefined
}
