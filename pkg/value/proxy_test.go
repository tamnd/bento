package value

import "testing"

// TestProxyConstructRejectsNonObject pins that a Proxy over a primitive target or
// handler throws a TypeError the way the constructor's first steps reject it.
func TestProxyConstructRejectsNonObject(t *testing.T) {
	cases := []struct {
		name            string
		target, handler Value
	}{
		{"primitive target", Number(1), NewObject()},
		{"primitive handler", NewObject(), Number(1)},
	}
	for _, c := range cases {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: NewProxy did not throw", c.name)
				}
			}()
			NewProxy(c.target, c.handler)
		}()
	}
}

// TestProxyEmptyHandlerForwards pins the base behavior: a Proxy whose handler
// carries no trap forwards every operation to the target, so it reads, writes,
// probes, and deletes exactly as the target would.
func TestProxyEmptyHandlerForwards(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("x"), Number(1))
	p := NewProxy(target, NewObject())

	if got := p.Get(FromGoString("x")); got.AsNumber() != 1 {
		t.Errorf("proxy get did not forward to target, got %v", got)
	}
	p.SetKey(FromGoString("y"), Number(2))
	if got := target.Get(FromGoString("y")); got.AsNumber() != 2 {
		t.Errorf("proxy set did not reach target, got %v", got)
	}
	if !p.HasProperty(FromGoString("x")) {
		t.Error("proxy has did not forward to target")
	}
	if !p.Delete(FromGoString("x")) || target.HasProperty(FromGoString("x")) {
		t.Error("proxy delete did not reach target")
	}
}

// TestProxyForwardsOwnKeysAndDescriptors pins that ownKeys and the descriptor and
// prototype reads forward to the target when the handler is empty.
func TestProxyForwardsOwnKeysAndDescriptors(t *testing.T) {
	proto := NewObject()
	target := ObjectCreate(proto)
	target.Set(FromGoString("a"), Number(1))
	target.Set(FromGoString("b"), Number(2))
	p := NewProxy(target, NewObject())

	keys := ReflectOwnKeys(p).Elems()
	if len(keys) != 2 || keys[0].str().ToGoString() != "a" || keys[1].str().ToGoString() != "b" {
		t.Errorf("proxy ownKeys did not forward in order, got %v", keys)
	}
	d := p.GetOwnPropertyDescriptor(StringValue(FromGoString("a")))
	if d.kind != KindObject || d.Get(FromGoString("value")).AsNumber() != 1 {
		t.Errorf("proxy getOwnPropertyDescriptor did not forward, got %v", d)
	}
	if got := p.GetPrototype(); got.kind != KindObject {
		t.Errorf("proxy getPrototypeOf did not forward, got %v", got)
	}
	if !p.IsExtensible() {
		t.Error("proxy isExtensible did not forward a true")
	}
}

// TestProxyGetTrap pins that a get trap intercepts a property read, receiving the
// target, the key, and the proxy as the receiver, and that its result is what the
// read returns rather than the target's own value.
func TestProxyGetTrap(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("x"), Number(1))
	handler := NewObject()
	handler.Set(FromGoString("get"), NewFunc(func(args []Value) Value {
		key := Arg(args, 1)
		return StringValue(FromGoString("trapped:").ConcatN(key.str()))
	}))
	p := NewProxy(target, handler)
	if got := p.Get(FromGoString("x")).str().ToGoString(); got != "trapped:x" {
		t.Errorf("get trap did not intercept, got %q", got)
	}
	if got := p.Get(FromGoString("y")).str().ToGoString(); got != "trapped:y" {
		t.Errorf("get trap did not see the missing key, got %q", got)
	}
}

// TestProxySetTrap pins that a set trap intercepts a property write and that the
// target is left untouched when the trap does not write through to it.
func TestProxySetTrap(t *testing.T) {
	target := NewObject()
	var wroteKey, wroteVal Value
	handler := NewObject()
	handler.Set(FromGoString("set"), NewFunc(func(args []Value) Value {
		wroteKey = Arg(args, 1)
		wroteVal = Arg(args, 2)
		return Bool(true)
	}))
	p := NewProxy(target, handler)
	p.SetKey(FromGoString("k"), Number(9))
	if wroteKey.str().ToGoString() != "k" || wroteVal.AsNumber() != 9 {
		t.Errorf("set trap saw key %v value %v", wroteKey, wroteVal)
	}
	if target.HasProperty(FromGoString("k")) {
		t.Error("set trap wrote through to the target when it should not have")
	}
}

// TestProxyGetInvariant pins the one get invariant a static target enforces: a
// non-configurable, non-writable own data property forces the trap to report the
// stored value, so a trap that returns anything else throws a TypeError.
func TestProxyGetInvariant(t *testing.T) {
	target := NewObject()
	desc := NewObject()
	desc.Set(FromGoString("value"), Number(42))
	desc.Set(FromGoString("writable"), Bool(false))
	desc.Set(FromGoString("configurable"), Bool(false))
	target.DefineProperty(StringValue(FromGoString("fixed")), desc)
	handler := NewObject()
	handler.Set(FromGoString("get"), NewFunc(func(args []Value) Value {
		return Number(0)
	}))
	p := NewProxy(target, handler)
	defer func() {
		if recover() == nil {
			t.Error("get trap violating the non-writable invariant did not throw")
		}
	}()
	p.Get(FromGoString("fixed"))
}

// TestProxyHasTrap pins that a has trap answers the in operator, receiving the
// target and the key, and that its boolean drives the result regardless of what the
// target actually holds.
func TestProxyHasTrap(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("real"), Number(1))
	handler := NewObject()
	handler.Set(FromGoString("has"), NewFunc(func(args []Value) Value {
		return Bool(Arg(args, 1).str().ToGoString() == "virtual")
	}))
	p := NewProxy(target, handler)
	if !p.HasProperty(FromGoString("virtual")) {
		t.Error("has trap did not report a virtual property present")
	}
	if p.HasProperty(FromGoString("real")) {
		t.Error("has trap did not override the target's own property")
	}
}

// TestProxyDeleteTrap pins that a deleteProperty trap intercepts delete and that a
// falsy return reports the delete as refused without touching the target.
func TestProxyDeleteTrap(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("k"), Number(1))
	handler := NewObject()
	handler.Set(FromGoString("deleteProperty"), NewFunc(func(args []Value) Value {
		return Bool(false)
	}))
	p := NewProxy(target, handler)
	if p.Delete(FromGoString("k")) {
		t.Error("delete trap returning falsy did not report a refused delete")
	}
	if !target.HasProperty(FromGoString("k")) {
		t.Error("delete trap returning falsy still removed the target's property")
	}
}

// TestProxyHasInvariant pins that a has trap cannot hide a non-configurable own
// property of the target: reporting it absent throws a TypeError.
func TestProxyHasInvariant(t *testing.T) {
	target := NewObject()
	desc := NewObject()
	desc.Set(FromGoString("value"), Number(1))
	desc.Set(FromGoString("configurable"), Bool(false))
	target.DefineProperty(StringValue(FromGoString("fixed")), desc)
	handler := NewObject()
	handler.Set(FromGoString("has"), NewFunc(func(args []Value) Value {
		return Bool(false)
	}))
	p := NewProxy(target, handler)
	defer func() {
		if recover() == nil {
			t.Error("has trap hiding a non-configurable property did not throw")
		}
	}()
	p.HasProperty(FromGoString("fixed"))
}

// TestProxyOwnKeysTrap pins that an ownKeys trap supplies the key list, so
// Reflect.ownKeys reports what the trap returns rather than the target's own keys.
func TestProxyOwnKeysTrap(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("a"), Number(1))
	handler := NewObject()
	handler.Set(FromGoString("ownKeys"), NewFunc(func(args []Value) Value {
		return NewArrayValue([]Value{StringValue(FromGoString("a")), StringValue(FromGoString("x"))})
	}))
	p := NewProxy(target, handler)
	keys := ReflectOwnKeys(p).Elems()
	if len(keys) != 2 || keys[1].str().ToGoString() != "x" {
		t.Errorf("ownKeys trap did not drive the key list, got %v", keys)
	}
}

// TestProxyOwnKeysInvariant pins that an ownKeys trap that drops a non-configurable
// own key of the target throws a TypeError.
func TestProxyOwnKeysInvariant(t *testing.T) {
	target := NewObject()
	desc := NewObject()
	desc.Set(FromGoString("value"), Number(1))
	desc.Set(FromGoString("configurable"), Bool(false))
	target.DefineProperty(StringValue(FromGoString("fixed")), desc)
	handler := NewObject()
	handler.Set(FromGoString("ownKeys"), NewFunc(func(args []Value) Value {
		return NewArrayValue(nil)
	}))
	p := NewProxy(target, handler)
	defer func() {
		if recover() == nil {
			t.Error("ownKeys trap dropping a non-configurable key did not throw")
		}
	}()
	ReflectOwnKeys(p)
}

// TestProxyGetOwnPropertyDescriptorTrap pins that a getOwnPropertyDescriptor trap
// supplies the descriptor a property read of it returns.
func TestProxyGetOwnPropertyDescriptorTrap(t *testing.T) {
	target := NewObject()
	handler := NewObject()
	handler.Set(FromGoString("getOwnPropertyDescriptor"), NewFunc(func(args []Value) Value {
		d := NewObject()
		d.Set(FromGoString("value"), Number(7))
		d.Set(FromGoString("writable"), Bool(true))
		d.Set(FromGoString("enumerable"), Bool(true))
		d.Set(FromGoString("configurable"), Bool(true))
		return d
	}))
	p := NewProxy(target, handler)
	d := p.GetOwnPropertyDescriptor(StringValue(FromGoString("any")))
	if d.kind != KindObject || d.Get(FromGoString("value")).AsNumber() != 7 {
		t.Errorf("getOwnPropertyDescriptor trap did not supply the descriptor, got %v", d)
	}
}

// TestProxyDefinePropertyTrap pins that a defineProperty trap intercepts the
// definition and that a falsy return throws the way a refused definition does.
func TestProxyDefinePropertyTrap(t *testing.T) {
	target := NewObject()
	var sawKey Value
	handler := NewObject()
	handler.Set(FromGoString("defineProperty"), NewFunc(func(args []Value) Value {
		sawKey = Arg(args, 1)
		return Bool(false)
	}))
	p := NewProxy(target, handler)
	desc := NewObject()
	desc.Set(FromGoString("value"), Number(1))
	func() {
		defer func() {
			if recover() == nil {
				t.Error("defineProperty trap returning falsy did not throw")
			}
		}()
		p.DefineProperty(StringValue(FromGoString("k")), desc)
	}()
	if sawKey.str().ToGoString() != "k" {
		t.Errorf("defineProperty trap saw key %v", sawKey)
	}
	if target.HasProperty(FromGoString("k")) {
		t.Error("defineProperty trap defined on the target despite a falsy return")
	}
}

// TestProxyGetPrototypeOfTrap pins that a getPrototypeOf trap supplies the prototype
// Object.getPrototypeOf reports over an extensible target.
func TestProxyGetPrototypeOfTrap(t *testing.T) {
	custom := NewObject()
	custom.Set(FromGoString("tag"), Number(1))
	handler := NewObject()
	handler.Set(FromGoString("getPrototypeOf"), NewFunc(func(args []Value) Value {
		return custom
	}))
	p := NewProxy(NewObject(), handler)
	if got := p.GetPrototype(); got.kind != KindObject || got.Get(FromGoString("tag")).AsNumber() != 1 {
		t.Errorf("getPrototypeOf trap did not supply the prototype, got %v", got)
	}
}

// TestProxySetPrototypeOfTrap pins that a setPrototypeOf trap intercepts the
// assignment and that a falsy return throws the way a refused assignment does.
func TestProxySetPrototypeOfTrap(t *testing.T) {
	var saw bool
	handler := NewObject()
	handler.Set(FromGoString("setPrototypeOf"), NewFunc(func(args []Value) Value {
		saw = true
		return Bool(false)
	}))
	p := NewProxy(NewObject(), handler)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("setPrototypeOf trap returning falsy did not throw")
			}
		}()
		p.SetPrototype(NewObject())
	}()
	if !saw {
		t.Error("setPrototypeOf trap was not called")
	}
}

// TestProxyIsExtensibleInvariant pins that an isExtensible trap must agree with the
// target's own extensibility: a disagreeing boolean throws a TypeError.
func TestProxyIsExtensibleInvariant(t *testing.T) {
	handler := NewObject()
	handler.Set(FromGoString("isExtensible"), NewFunc(func(args []Value) Value {
		return Bool(false)
	}))
	p := NewProxy(NewObject(), handler) // target is extensible, trap says not
	defer func() {
		if recover() == nil {
			t.Error("isExtensible trap disagreeing with the target did not throw")
		}
	}()
	p.IsExtensible()
}

// TestProxyPreventExtensionsTrap pins that a preventExtensions trap intercepts the
// request and that a truthy return is honored only once the target is itself
// non-extensible, so a trap that seals the target passes the invariant.
func TestProxyPreventExtensionsTrap(t *testing.T) {
	target := NewObject()
	handler := NewObject()
	handler.Set(FromGoString("preventExtensions"), NewFunc(func(args []Value) Value {
		Arg(args, 0).PreventExtensions()
		return Bool(true)
	}))
	p := NewProxy(target, handler)
	p.PreventExtensions()
	if target.IsExtensible() {
		t.Error("preventExtensions trap did not seal the target")
	}
}

// TestProxyApplyTrap pins that an apply trap intercepts a call to a callable proxy,
// receiving the target, the this value, and the arguments as one array.
func TestProxyApplyTrap(t *testing.T) {
	target := NewFunc(func(args []Value) Value { return Number(0) })
	handler := NewObject()
	handler.Set(FromGoString("apply"), NewFunc(func(args []Value) Value {
		list := Arg(args, 2)
		return Number(list.GetIndex(0).AsNumber() + list.GetIndex(1).AsNumber() + 100)
	}))
	p := NewProxy(target, handler)
	if got := p.Call(Number(2), Number(3)); got.AsNumber() != 105 {
		t.Errorf("apply trap did not intercept the call, got %v", got)
	}
}

// TestProxyConstructTrap pins that a construct trap produces the object new returns,
// receiving the target, the arguments as one array, and the new target.
func TestProxyConstructTrap(t *testing.T) {
	target := NewFunc(func(args []Value) Value { return Undefined })
	handler := NewObject()
	handler.Set(FromGoString("construct"), NewFunc(func(args []Value) Value {
		o := NewObject()
		o.Set(FromGoString("built"), Bool(true))
		o.Set(FromGoString("first"), Arg(args, 1).GetIndex(0))
		return o
	}))
	p := NewProxy(target, handler)
	res := p.asProxy().construct([]Value{Number(7)}, p)
	if res.kind != KindObject || !ToBoolean(res.Get(FromGoString("built"))) || res.Get(FromGoString("first")).AsNumber() != 7 {
		t.Errorf("construct trap did not produce the object, got %v", res)
	}
}

// TestProxyConstructInvariant pins that a construct trap must return an object: a
// primitive return throws a TypeError.
func TestProxyConstructInvariant(t *testing.T) {
	target := NewFunc(func(args []Value) Value { return Undefined })
	handler := NewObject()
	handler.Set(FromGoString("construct"), NewFunc(func(args []Value) Value {
		return Number(1)
	}))
	p := NewProxy(target, handler)
	defer func() {
		if recover() == nil {
			t.Error("construct trap returning a non-object did not throw")
		}
	}()
	p.asProxy().construct(nil, p)
}

// TestProxyRevocable pins Proxy.revocable: it returns a { proxy, revoke } object, the
// proxy works until revoke is called, and every operation on it throws afterward.
func TestProxyRevocable(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("x"), Number(1))
	r := ProxyRevocable(target, NewObject())
	p := r.Get(FromGoString("proxy"))
	revoke := r.Get(FromGoString("revoke"))
	if got := p.Get(FromGoString("x")); got.AsNumber() != 1 {
		t.Errorf("revocable proxy did not forward before revoke, got %v", got)
	}
	revoke.Call()
	defer func() {
		if recover() == nil {
			t.Error("a read on a revoked proxy did not throw")
		}
	}()
	p.Get(FromGoString("x"))
}

// TestProxyCallableForwards pins that a Proxy over a callable target is itself
// callable and forwards the call to the target when the handler has no apply trap.
func TestProxyCallableForwards(t *testing.T) {
	target := NewFunc(func(args []Value) Value {
		return Number(Arg(args, 0).AsNumber() + Arg(args, 1).AsNumber())
	})
	p := NewProxy(target, NewObject())
	if p.kind != KindFunc {
		t.Fatalf("proxy over a callable target has kind %v, want KindFunc", p.kind)
	}
	if got := p.Call(Number(2), Number(3)); got.AsNumber() != 5 {
		t.Errorf("proxy call did not forward to target, got %v", got)
	}
}
