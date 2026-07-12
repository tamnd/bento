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
