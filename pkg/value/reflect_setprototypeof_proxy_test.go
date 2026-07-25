package value

import "testing"

// Reflect.setPrototypeOf over a proxy routes through the setPrototypeOf trap rather
// than writing the proxy's own slot. A truthy trap over a non-extensible target whose
// current prototype differs from the requested one violates the [[SetPrototypeOf]]
// invariant, so the call throws a TypeError the same way Object.setPrototypeOf does.
func TestReflectSetPrototypeOfProxyNonExtensibleInvariantThrows(t *testing.T) {
	target := NewObject()
	handler := NewObject().Set(FromGoString("setPrototypeOf"), NewFunc(func(_ []Value) Value {
		return Bool(true)
	}))
	proxy := NewProxy(target, handler)
	target.PreventExtensions()
	threw := false
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				threw = Caught(rec).IsA("TypeError")
			}
		}()
		ReflectSetPrototypeOf(proxy, NewObject())
	}()
	if !threw {
		t.Fatalf("Reflect.setPrototypeOf over non-extensible proxy target: expected a TypeError")
	}
}

// A falsy setPrototypeOf trap refuses the assignment. Reflect.setPrototypeOf reports
// that refusal as false without throwing, the boolean the [[SetPrototypeOf]] result
// carries, where Object.setPrototypeOf would turn the same false into a TypeError.
func TestReflectSetPrototypeOfProxyFalsyTrapReportsFalse(t *testing.T) {
	target := NewObject()
	handler := NewObject().Set(FromGoString("setPrototypeOf"), NewFunc(func(_ []Value) Value {
		return Bool(false)
	}))
	proxy := NewProxy(target, handler)
	if ReflectSetPrototypeOf(proxy, NewObject()) {
		t.Fatalf("Reflect.setPrototypeOf with a falsy trap: expected false")
	}
}

// An extensible target lets a truthy trap succeed, so Reflect.setPrototypeOf reports
// true without consulting the invariant that only guards a non-extensible target.
func TestReflectSetPrototypeOfProxyExtensibleTruthyTrapReportsTrue(t *testing.T) {
	target := NewObject()
	handler := NewObject().Set(FromGoString("setPrototypeOf"), NewFunc(func(_ []Value) Value {
		return Bool(true)
	}))
	proxy := NewProxy(target, handler)
	if !ReflectSetPrototypeOf(proxy, NewObject()) {
		t.Fatalf("Reflect.setPrototypeOf over extensible proxy target with truthy trap: expected true")
	}
}
