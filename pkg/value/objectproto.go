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
//
// The table is the whole of Object.prototype's callable surface, so a program cannot
// find the one member that was left out. Two of them, toString and valueOf, are the
// pair the language calls on its own during a coercion rather than at a call site the
// program wrote, so they are what OrdinaryToPrimitive reads off an object with no
// coercion of its own. The two absentees are data slots rather than methods:
// constructor would have to hand back an Object constructor function the value model
// does not build, and __proto__ an Object.prototype object it does not carry either.
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
	case "__defineGetter__":
		return NewFunc(func(args []Value) Value {
			return recv.defineLegacyAccessor(Arg(args, 0), Arg(args, 1), true)
		}), true
	case "__defineSetter__":
		return NewFunc(func(args []Value) Value {
			return recv.defineLegacyAccessor(Arg(args, 0), Arg(args, 1), false)
		}), true
	case "toString":
		// Object.prototype.toString reports the receiver's class tag. A receiver whose own
		// prototype writes a nearer toString never reads this one: an array is asked
		// Array.prototype first by getChained, a Map, Set, Date and typed array answer off
		// their own box before the chain is walked at all, and a boxed class instance
		// answers the method its class writes. A boxed error and regexp are picked out
		// inside protoToString, since their prototypes write a toString bento keeps as a
		// brand rather than as a property, so a chain walk would miss it.
		return NewFunc(func([]Value) Value { return StringValue(protoToString(recv)) }), true
	case "valueOf":
		// Object.prototype.valueOf returns the receiver unchanged. That identity is what
		// makes the default hint fall through to toString: an object is not a primitive, so
		// OrdinaryToPrimitive rejects the result and asks the next method.
		return NewFunc(func([]Value) Value { return recv }), true
	}
	return Undefined, false
}

// objectProtoHas reports whether Object.prototype carries key, the existence half of
// objectProtoBuiltin that the in operator asks. It answers by making the read and
// dropping the method rather than by consulting a second list, so the two can never
// name different sets: 'toString' in o is true exactly when o.toString reads a method.
func objectProtoHas(recv Value, key BStr) bool {
	_, ok := objectProtoBuiltin(recv, key)
	return ok
}

// protoToString spells the receiver the way the toString nearest it on the prototype
// chain does. A boxed error and a boxed regexp each inherit a prototype writing their
// own, which bento keeps as a brand on the object's storage rather than as a property,
// so a chain walk misses it and would fall to Object.prototype's tag: String(new
// TypeError('bad')) would read "[object Error]" where the engine reads "TypeError: bad".
// Everything else reaching here really does inherit Object.prototype's, which is the
// class tag, read off the receiver so a Symbol.toStringTag naming it wins.
func protoToString(recv Value) BStr {
	if recv.kind == KindObject {
		if e := recv.object().err; e != nil {
			return e.ToBStr()
		}
	}
	if re := recv.asRegExp(); re != nil {
		return re.ToStringBStr()
	}
	return ClassTag(recv)
}

// defineLegacyAccessor installs an accessor property for key on the receiver, the write
// __defineGetter__ and __defineSetter__ make. Only the half named is replaced: defining
// a getter over an existing accessor keeps its setter, which is what lets the two calls
// build one accessor between them. The property is enumerable and configurable, the
// attributes the legacy helpers give, unlike a { get } passed to Object.defineProperty,
// which defaults both to false. A non-callable second argument throws the TypeError the
// engine names the helper in, and the call answers undefined either way.
func (recv Value) defineLegacyAccessor(key, fn Value, wantGet bool) Value {
	which := "__defineSetter__"
	if wantGet {
		which = "__defineGetter__"
	}
	if fn.kind != KindFunc {
		Throw(NewTypeError(FromGoString("Object.prototype." + which + ": Expecting function")))
		return Undefined
	}
	switch recv.kind {
	case KindObject, KindArray, KindFunc:
	default:
		return Undefined
	}
	o := recv.object()
	get, set := fn, Undefined
	if !wantGet {
		get, set = Undefined, fn
	}
	if key.kind == KindSymbol {
		sym := key.symbol()
		if d, ok := o.getSymDesc(sym); ok && d.accessor {
			if wantGet {
				set = d.set
			} else {
				get = d.get
			}
		}
		o.defineSym(sym, accessorProperty(get, set, true, true))
		return Undefined
	}
	name := key.str()
	if key.kind != KindString {
		name = ToString(key)
	}
	if d, ok := o.getOwnDesc(name); ok && d.accessor {
		if wantGet {
			set = d.set
		} else {
			get = d.get
		}
	}
	o.defineOwn(name, accessorProperty(get, set, true, true))
	return Undefined
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
