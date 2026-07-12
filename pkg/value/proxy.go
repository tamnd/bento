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

// has runs the has trap for the in operator, the [[HasProperty]](P) internal
// method. With no trap the probe forwards to the target and walks its prototype
// chain. With a trap the answer is handler.has(target, key), checked so the trap
// cannot hide a fixed property: a non-configurable own property may not be reported
// absent, and neither may any own property of a non-extensible target.
func (p *proxyData) has(key BStr) bool {
	p.checkRevoked("has")
	trap := p.trap("has")
	if trap.kind == KindUndefined {
		return p.target.HasProperty(key)
	}
	if ToBoolean(trap.Call(p.target, StringValue(key))) {
		return true
	}
	if d := p.target.GetOwnPropertyDescriptor(StringValue(key)); d.kind == KindObject {
		if !ToBoolean(d.Get(FromGoString("configurable"))) {
			Throw(NewTypeError(FromGoString("'has' on proxy: trap returned falsy for property which exists in the proxy target as a non-configurable property")))
		}
		if !p.target.IsExtensible() {
			Throw(NewTypeError(FromGoString("'has' on proxy: trap returned falsy for property but the proxy target is not extensible")))
		}
	}
	return false
}

func (p *proxyData) deleteKey(key BStr) bool {
	return p.deleteWith(StringValue(key))
}

func (p *proxyData) deleteSym(key *Symbol) bool {
	return p.deleteWith(symbolValue(key))
}

// deleteWith runs the deleteProperty trap for a boxed key, the [[Delete]](P)
// internal method behind delete o[k]. With no trap the removal forwards to the
// target. With a trap a falsy return is a refused delete reported as false, and a
// truthy return is checked so the trap cannot claim to remove a property the target
// still holds fixed: a non-configurable own property, or any own property of a
// non-extensible target, cannot be reported deleted.
func (p *proxyData) deleteWith(keyVal Value) bool {
	p.checkRevoked("deleteProperty")
	trap := p.trap("deleteProperty")
	if trap.kind == KindUndefined {
		return p.target.DeleteElem(keyVal)
	}
	if !ToBoolean(trap.Call(p.target, keyVal)) {
		return false
	}
	d := p.target.GetOwnPropertyDescriptor(keyVal)
	if d.kind != KindObject {
		return true
	}
	if !ToBoolean(d.Get(FromGoString("configurable"))) {
		Throw(NewTypeError(FromGoString("'deleteProperty' on proxy: trap returned truthy for property which is non-configurable in the proxy target")))
	}
	if !p.target.IsExtensible() {
		Throw(NewTypeError(FromGoString("'deleteProperty' on proxy: trap returned truthy for property but the proxy target is non-extensible")))
	}
	return true
}

func (p *proxyData) call(args []Value) Value {
	p.checkRevoked("apply")
	return p.target.Call(args...)
}

// defineProperty runs the defineProperty trap, the [[DefineOwnProperty]](P, Desc)
// internal method behind Object.defineProperty. With no trap the definition
// forwards to the target. With a trap a falsy return is a refused definition, which
// the Object.defineProperty layer turns into a TypeError the way a failed
// [[DefineOwnProperty]] does.
func (p *proxyData) defineProperty(key, descObj Value) {
	p.checkRevoked("defineProperty")
	trap := p.trap("defineProperty")
	if trap.kind == KindUndefined {
		p.target.DefineProperty(key, descObj)
		return
	}
	if !ToBoolean(trap.Call(p.target, key, descObj)) {
		Throw(NewTypeError(FromGoString("'defineProperty' on proxy: trap returned falsy for property")))
	}
}

// getOwnPropertyDescriptor runs the getOwnPropertyDescriptor trap, the
// [[GetOwnProperty]](P) internal method behind Object.getOwnPropertyDescriptor and
// the per-key filter Object.keys applies. With no trap the descriptor forwards from
// the target. With a trap the result must be an object or undefined, and reporting a
// non-configurable own property of the target absent, or any own property of a
// non-extensible target absent, throws the way the invariant forbids hiding a fixed
// property.
func (p *proxyData) getOwnPropertyDescriptor(key Value) Value {
	p.checkRevoked("getOwnPropertyDescriptor")
	trap := p.trap("getOwnPropertyDescriptor")
	if trap.kind == KindUndefined {
		return p.target.GetOwnPropertyDescriptor(key)
	}
	res := trap.Call(p.target, key)
	if res.kind != KindObject && res.kind != KindUndefined {
		Throw(NewTypeError(FromGoString("'getOwnPropertyDescriptor' on proxy: trap must return an object or undefined")))
	}
	if res.kind == KindUndefined {
		if d := p.target.GetOwnPropertyDescriptor(key); d.kind == KindObject {
			if !ToBoolean(d.Get(FromGoString("configurable"))) {
				Throw(NewTypeError(FromGoString("'getOwnPropertyDescriptor' on proxy: trap reported a non-configurable target property as non-existent")))
			}
			if !p.target.IsExtensible() {
				Throw(NewTypeError(FromGoString("'getOwnPropertyDescriptor' on proxy: trap reported an existing property of a non-extensible target as non-existent")))
			}
		}
	}
	return res
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

// ownKeys runs the ownKeys trap, the [[OwnPropertyKeys]]() internal method behind
// Reflect.ownKeys, Object.keys, and the getOwnPropertyNames family. With no trap the
// key list forwards from the target. With a trap the result must be a list of only
// string and symbol keys with no duplicate, it must include every non-configurable
// own key of the target, and when the target is not extensible it must be exactly
// the target's own keys, the invariants that keep the trap from hiding or inventing
// a fixed key.
func (p *proxyData) ownKeys() []Value {
	p.checkRevoked("ownKeys")
	trap := p.trap("ownKeys")
	if trap.kind == KindUndefined {
		return ReflectOwnKeys(p.target).Elems()
	}
	res := trap.Call(p.target)
	if res.kind != KindArray {
		Throw(NewTypeError(FromGoString("'ownKeys' on proxy: trap result must be an array-like object")))
	}
	trapKeys := res.object().elems
	for i, k := range trapKeys {
		if k.kind != KindString && k.kind != KindSymbol {
			Throw(NewTypeError(FromGoString("'ownKeys' on proxy: trap result must only contain string and symbol keys")))
		}
		for _, seen := range trapKeys[:i] {
			if sameValue(k, seen) {
				Throw(NewTypeError(FromGoString("'ownKeys' on proxy: trap result must not contain duplicate keys")))
			}
		}
	}
	extensible := p.target.IsExtensible()
	targetKeys := ReflectOwnKeys(p.target).Elems()
	for _, tk := range targetKeys {
		d := p.target.GetOwnPropertyDescriptor(tk)
		configurable := d.kind == KindObject && ToBoolean(d.Get(FromGoString("configurable")))
		if configurable && extensible {
			continue
		}
		if !containsKey(trapKeys, tk) {
			Throw(NewTypeError(FromGoString("'ownKeys' on proxy: trap result did not include a non-configurable or non-extensible target key")))
		}
	}
	if !extensible && len(trapKeys) != len(targetKeys) {
		Throw(NewTypeError(FromGoString("'ownKeys' on proxy: trap result must be exactly the target's own keys when the target is not extensible")))
	}
	return trapKeys
}

// containsKey reports whether keys holds a property key that is the same value as k,
// the membership test the ownKeys invariants run over string and symbol keys.
func containsKey(keys []Value, k Value) bool {
	for _, key := range keys {
		if sameValue(key, k) {
			return true
		}
	}
	return false
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
