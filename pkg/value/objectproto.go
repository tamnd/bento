package value

// objectProtoBuiltin supplies the Object.prototype methods a dynamic property read
// resolves when nothing on the receiver's own chain overrode them. bento's object
// model carries no synthetic Object.prototype object, so a chain walk that reaches
// the end without finding a name like "hasOwnProperty" would answer undefined, and a
// dynamic method call obj.hasOwnProperty(k) would then invoke undefined and throw.
// Every ordinary object inherits these methods from Object.prototype, so on a genuine
// chain miss the read answers the built-in bound to the receiver. A receiver that
// declares its own property of the same name never reaches here: getChained returns
// the own (or nearer prototype) slot first, so a user override still wins, matching
// the language's own-before-inherited lookup order. The returned function closes over
// recv because the dynamic call site threads no separate this; obj.method(x) always
// calls with this === obj, which is exactly recv.
func objectProtoBuiltin(recv Value, key BStr) (Value, bool) {
	switch key.ToGoString() {
	case "hasOwnProperty":
		return NewFunc(func(args []Value) Value {
			return Bool(recv.HasOwnElem(Arg(args, 0)))
		}), true
	case "isPrototypeOf":
		return NewFunc(func(args []Value) Value {
			return Bool(recv.isPrototypeOf(Arg(args, 0)))
		}), true
	case "propertyIsEnumerable":
		return NewFunc(func(args []Value) Value {
			return Bool(recv.propertyIsEnumerable(Arg(args, 0)))
		}), true
	case "toLocaleString":
		// Object.prototype.toLocaleString calls this.toString() with no argument and
		// returns its result, so it answers the same string the receiver's toString does.
		return NewFunc(func(args []Value) Value {
			return recv.ToStringMethod()
		}), true
	case "__lookupGetter__":
		return NewFunc(func(args []Value) Value {
			return recv.lookupAccessor(Arg(args, 0), true)
		}), true
	case "__lookupSetter__":
		return NewFunc(func(args []Value) Value {
			return recv.lookupAccessor(Arg(args, 0), false)
		}), true
	}
	return Undefined, false
}

// isPrototypeOf reports whether recv appears anywhere on target's prototype chain,
// the answer Object.prototype.isPrototypeOf gives. A non-object target has no chain,
// so it reports false; otherwise the walk climbs target's [[Prototype]] links and
// reports true when it reaches recv's own object identity.
func (recv Value) isPrototypeOf(target Value) bool {
	switch target.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return false
	}
	self := recv.object()
	if self == nil {
		return false
	}
	for cur := target.object().proto; cur != nil; cur = cur.proto {
		if cur == self {
			return true
		}
	}
	return false
}

// propertyIsEnumerable reports whether key names an own, enumerable property of recv,
// the answer Object.prototype.propertyIsEnumerable gives: an inherited or absent
// property reports false, and an own property reports its enumerable attribute. A
// symbol key probes the symbol bag; a string key its named slot, with an array's
// in-range indices enumerable and its length non-enumerable.
func (recv Value) propertyIsEnumerable(key Value) bool {
	switch recv.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return false
	}
	o := recv.object()
	if key.kind == KindSymbol {
		if d, ok := o.getSymDesc(key.symbol()); ok {
			return d.enumerable
		}
		return false
	}
	name := key.str()
	if key.kind != KindString {
		name = ToString(key)
	}
	if recv.kind == KindArray {
		s := name.ToGoString()
		if s == "length" {
			return false
		}
		if idx, ok := arrayIndex(s); ok {
			return idx < len(o.elems) && !isHole(o.elems[idx])
		}
	}
	if d, ok := o.getOwnDesc(name); ok {
		return d.enumerable
	}
	return false
}

// lookupAccessor returns the getter (wantGet true) or setter (wantGet false) an
// accessor property installs for key anywhere on recv's prototype chain, the read
// __lookupGetter__ and __lookupSetter__ make. A data property or an absent key has no
// such function, so the result is undefined; the first accessor found while climbing
// wins, the way the legacy lookup resolves against the nearest definition.
func (recv Value) lookupAccessor(key Value, wantGet bool) Value {
	switch recv.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return Undefined
	}
	var name BStr
	if key.kind == KindString {
		name = key.str()
	} else {
		name = ToString(key)
	}
	for cur := recv.object(); cur != nil; cur = cur.proto {
		if d, ok := cur.getOwnDesc(name); ok {
			if !d.accessor {
				return Undefined
			}
			if wantGet {
				return d.get
			}
			return d.set
		}
	}
	return Undefined
}
