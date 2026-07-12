// This file owns Proxy at runtime, the exotic object whose every internal method
// routes through a handler before it reaches the target it wraps (10_advanced
// group 4). A Proxy is not a distinct Value kind: it rides the same Object storage
// as a plain object, with a proxy pointer set and its kind taken from the target
// so a proxy over a callable is itself callable and typeof reports "function". A
// handler that carries no trap for an operation forwards that operation to the
// target unchanged, which is the whole of a Proxy with an empty handler and the
// base every trap builds on. This first cut lands the target-and-handler value and
// that forwarding; the traps that intercept each operation are the slices that
// follow.

package value

// proxyData is the exotic state behind a Proxy: the object it wraps, the handler
// whose properties are its traps, and the revoked flag Proxy.revocable flips. The
// state hangs off the Object through a single pointer so a plain object pays only
// one nil word for a feature it does not use.
type proxyData struct {
	target  Value
	handler Value
	revoked bool
}

// isObjectLike reports whether v is one of the reference kinds a Proxy target and
// handler must be, the ToObject-free object test the Proxy constructor makes on
// both arguments before it builds anything.
func isObjectLike(v Value) bool {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return true
	}
	return false
}

// NewProxy builds a Proxy over target with handler, the runtime behind new
// Proxy(target, handler). Both must be objects, so a primitive for either throws a
// TypeError the way the constructor's first steps reject it. The proxy takes its
// kind from the target: a callable target yields a callable proxy so typeof
// reports "function" and a call reaches the apply path, and any other object
// target yields an object proxy. The proxy's own property bag stays empty; every
// read, write, and probe routes through the handler and the target instead.
func NewProxy(target, handler Value) Value {
	if !isObjectLike(target) || !isObjectLike(handler) {
		Throw(NewTypeError(FromGoString("Cannot create proxy with a non-object as target or handler")))
		return Undefined
	}
	kind := KindObject
	if target.kind == KindFunc {
		kind = KindFunc
	}
	return objectValue(&Object{kind: kind, proxy: &proxyData{target: target, handler: handler}})
}

// asProxy returns the exotic state behind v, or nil when v is not a Proxy, the one
// probe every routed Value method makes before it runs its ordinary path.
func (v Value) asProxy() *proxyData {
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().proxy
	}
	return nil
}

// checkRevoked throws the TypeError a revoked proxy raises on any operation, the
// guard every trap runs first so a revoked proxy is inert rather than a window
// onto its former target.
func (p *proxyData) checkRevoked(op string) {
	if p.revoked {
		Throw(NewTypeError(FromGoString("Cannot perform '" + op + "' on a proxy that has been revoked")))
	}
}

// trap returns handler[name] when the handler carries a callable trap for the
// named operation, or undefined when it does not, in which case the caller
// forwards the operation to the target. A trap present but not callable throws a
// TypeError the way GetMethod rejects a non-function trap.
func (p *proxyData) trap(name string) Value {
	t := p.handler.Get(FromGoString(name))
	if t.kind == KindUndefined || t.kind == KindNull {
		return Undefined
	}
	if t.kind != KindFunc {
		Throw(NewTypeError(FromGoString("'" + name + "' on proxy: trap is not a function")))
		return Undefined
	}
	return t
}

// The methods below are the routed internal operations. This slice forwards each
// to the target, the behavior of a proxy whose handler carries no trap; a later
// slice replaces each forward with the handler-trap call and its invariant checks.

func (p *proxyData) get(recv Value, key BStr) Value {
	return p.getWith(recv, StringValue(key))
}

func (p *proxyData) getSym(recv Value, key *Symbol) Value {
	return p.getWith(recv, symbolValue(key))
}

// getWith runs the get trap for a boxed key, the [[Get]](P, Receiver) internal
// method. With no trap the read forwards to the target. With a trap the result is
// what handler.get(target, key, receiver) returns, checked against the one
// invariant a static target can enforce: a non-configurable, non-writable own data
// property must report its stored value, and a non-configurable accessor with no
// getter must report undefined, so the trap cannot lie about a fixed property.
func (p *proxyData) getWith(recv, keyVal Value) Value {
	p.checkRevoked("get")
	trap := p.trap("get")
	if trap.kind == KindUndefined {
		return p.target.GetElem(keyVal)
	}
	res := trap.Call(p.target, keyVal, recv)
	if d := p.target.GetOwnPropertyDescriptor(keyVal); d.kind == KindObject && !ToBoolean(d.Get(FromGoString("configurable"))) {
		if d.HasOwnElem(StringValue(FromGoString("value"))) {
			if !ToBoolean(d.Get(FromGoString("writable"))) && !sameValue(res, d.Get(FromGoString("value"))) {
				Throw(NewTypeError(FromGoString("'get' on proxy: property is a read-only and non-configurable data property on the proxy target but the proxy did not return its actual value")))
			}
		} else if d.Get(FromGoString("get")).kind == KindUndefined && res.kind != KindUndefined {
			Throw(NewTypeError(FromGoString("'get' on proxy: property is a non-configurable accessor property on the proxy target and does not have a getter function, but the trap did not return undefined")))
		}
	}
	return res
}

func (p *proxyData) setKey(recv Value, key BStr, val Value) {
	p.setWith(recv, StringValue(key), val)
}

func (p *proxyData) setSym(recv Value, key *Symbol, val Value) {
	p.setWith(recv, symbolValue(key), val)
}

// setWith runs the set trap for a boxed key, the [[Set]](P, V, Receiver) internal
// method. With no trap the write forwards to the target. With a trap the write is
// whatever handler.set(target, key, value, receiver) performs; a falsy return is a
// refused write, dropped the way this model drops a write to a non-writable
// property rather than throwing. A truthy return is checked against the target: a
// non-configurable, non-writable own data property may only be set to its stored
// value, and a non-configurable accessor with no setter may not be set at all.
func (p *proxyData) setWith(recv, keyVal, val Value) {
	p.checkRevoked("set")
	trap := p.trap("set")
	if trap.kind == KindUndefined {
		p.target.SetElem(keyVal, val)
		return
	}
	if !ToBoolean(trap.Call(p.target, keyVal, val, recv)) {
		return
	}
	if d := p.target.GetOwnPropertyDescriptor(keyVal); d.kind == KindObject && !ToBoolean(d.Get(FromGoString("configurable"))) {
		if d.HasOwnElem(StringValue(FromGoString("value"))) {
			if !ToBoolean(d.Get(FromGoString("writable"))) && !sameValue(val, d.Get(FromGoString("value"))) {
				Throw(NewTypeError(FromGoString("'set' on proxy: trap returned truthy for property which exists in the proxy target as a non-configurable and non-writable data property with a different value")))
			}
		} else if d.Get(FromGoString("set")).kind == KindUndefined {
			Throw(NewTypeError(FromGoString("'set' on proxy: trap returned truthy for property which exists in the proxy target as a non-configurable and non-writable accessor property without a setter")))
		}
	}
}

func (p *proxyData) has(key BStr) bool {
	p.checkRevoked("has")
	return p.target.HasProperty(key)
}

func (p *proxyData) deleteKey(key BStr) bool {
	p.checkRevoked("deleteProperty")
	return p.target.Delete(key)
}

func (p *proxyData) deleteSym(key *Symbol) bool {
	p.checkRevoked("deleteProperty")
	return p.target.deleteSymKey(key)
}

func (p *proxyData) call(args []Value) Value {
	p.checkRevoked("apply")
	return p.target.Call(args...)
}

func (p *proxyData) defineProperty(key, descObj Value) {
	p.checkRevoked("defineProperty")
	p.target.DefineProperty(key, descObj)
}

func (p *proxyData) getOwnPropertyDescriptor(key Value) Value {
	p.checkRevoked("getOwnPropertyDescriptor")
	return p.target.GetOwnPropertyDescriptor(key)
}

func (p *proxyData) getPrototypeOf() Value {
	p.checkRevoked("getPrototypeOf")
	return p.target.GetPrototype()
}

func (p *proxyData) setPrototypeOf(proto Value) {
	p.checkRevoked("setPrototypeOf")
	p.target.SetPrototype(proto)
}

func (p *proxyData) isExtensible() bool {
	p.checkRevoked("isExtensible")
	return p.target.IsExtensible()
}

func (p *proxyData) preventExtensions() {
	p.checkRevoked("preventExtensions")
	p.target.PreventExtensions()
}

func (p *proxyData) ownKeys() []Value {
	p.checkRevoked("ownKeys")
	return ReflectOwnKeys(p.target).Elems()
}

// stringKeys projects the proxy's own keys down to the string keys the
// Object.keys and Object.getOwnPropertyNames family reports, dropping the symbol
// keys. With enumerableOnly set it keeps only the keys whose descriptor reports
// enumerable, the [[GetOwnPropertyDescriptor]]-per-key filter Object.keys applies
// on top of [[OwnPropertyKeys]], so both walk the proxy through its traps.
func (p *proxyData) stringKeys(enumerableOnly bool) []BStr {
	var out []BStr
	for _, k := range p.ownKeys() {
		if k.kind != KindString {
			continue
		}
		if enumerableOnly {
			d := p.getOwnPropertyDescriptor(k)
			if d.kind != KindObject || !ToBoolean(d.Get(FromGoString("enumerable"))) {
				continue
			}
		}
		out = append(out, k.str())
	}
	return out
}
