// This file bridges the four weakly-holding types into the dynamic value.Value
// world: a WeakMap, a WeakSet, a WeakRef and a FinalizationRegistry. It is to those
// what mapvalue.go is to a Map, and it reads the same way, with one thing the strong
// collections do not have to answer.
//
// A weak collection has no size and no iteration, on purpose: what it holds may be
// collected between one read and the next, so a program that could walk it could
// observe the garbage collector. Node prints that as text rather than hiding it,
// "WeakMap { <items unknown> }", so the box needs no entry walk and no size, only a
// name and a placeholder. A WeakRef and a FinalizationRegistry print with empty
// braces, since neither claims to hold items at all.
//
// The box is a view kept on the collection itself, the same as a Map's, so two
// crossings of one WeakMap hand back one object and === says so. That also makes a
// weak collection a value another collection can hold and be keyed by, since the box
// unboxes back to the very collection the typed side holds.

package value

// weakBacking is the live weakly-holding value behind a box. All four kinds share one
// interface rather than one each, because what the box needs from them is the same
// three things: the name to print, whether to print the unreadable-items placeholder,
// and the member surface. The member switch is per kind, since a WeakMap's get and a
// WeakRef's deref have nothing in common.
type weakBacking interface {
	// jsWeakName is the constructor name, which the inspector prints before the braces
	// and Object.prototype.toString brands the value with.
	jsWeakName() string
	// jsWeakOpaque reports whether the value holds items it cannot show, which is true
	// of a WeakMap and a WeakSet and false of a WeakRef and a FinalizationRegistry.
	jsWeakOpaque() bool
	// jsWeakMember reads one member off the live value, reporting false for a name that
	// is not one so the read climbs the ordinary chain and ends at undefined.
	jsWeakMember(name string) (Value, bool)
}

// asWeak returns the live weak value a box carries, or nil when the value is not one.
// It is the probe the reads, the inspector and the deep comparison make before their
// ordinary object handling, the same shape asMap has.
func (v Value) asWeak() weakBacking {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsWeak
}

// ToValue boxes a typed WeakMap into a dynamic value, building the box once and
// keeping it on the map so two crossings of one map hand back one object.
func (m *WeakMap[T, V]) ToValue() Value {
	if m.boxed == nil {
		m.boxed = &Object{kind: KindObject, jsWeak: m}
	}
	return objectValue(m.boxed)
}

func (m *WeakMap[T, V]) jsWeakName() string { return "WeakMap" }

func (m *WeakMap[T, V]) jsWeakOpaque() bool { return true }

// jsWeakMember is the boxed spelling of the four methods beside it in weakmap.go. A
// key that the typed map's key type could not hold is reported absent by the reads,
// which is what dynUnbox answers, and raises on a write the way a Map's does.
func (m *WeakMap[T, V]) jsWeakMember(name string) (Value, bool) {
	switch name {
	case "get":
		return boundMethod("get", func(args []Value) Value {
			k, ok := dynUnbox[*T](Arg(args, 0))
			if !ok {
				return Undefined
			}
			return OptToValue(m.Get(k), dynBox[V])
		}), true
	case "set":
		return boundMethod("set", func(args []Value) Value {
			m.Set(dynUnboxOrThrow[*T](Arg(args, 0), "key"), dynUnboxOrThrow[V](Arg(args, 1), "value"))
			return m.ToValue()
		}), true
	case "has":
		return boundMethod("has", func(args []Value) Value {
			k, ok := dynUnbox[*T](Arg(args, 0))
			return Bool(ok && m.Has(k))
		}), true
	case "delete":
		return boundMethod("delete", func(args []Value) Value {
			k, ok := dynUnbox[*T](Arg(args, 0))
			return Bool(ok && m.Delete(k))
		}), true
	}
	return Undefined, false
}

// ToValue boxes a typed WeakSet into a dynamic value, the WeakMap case with a member
// rather than a pair.
func (s *WeakSet[T]) ToValue() Value {
	if s.boxed == nil {
		s.boxed = &Object{kind: KindObject, jsWeak: s}
	}
	return objectValue(s.boxed)
}

func (s *WeakSet[T]) jsWeakName() string { return "WeakSet" }

func (s *WeakSet[T]) jsWeakOpaque() bool { return true }

func (s *WeakSet[T]) jsWeakMember(name string) (Value, bool) {
	switch name {
	case "add":
		return boundMethod("add", func(args []Value) Value {
			s.Add(dynUnboxOrThrow[*T](Arg(args, 0), "member"))
			return s.ToValue()
		}), true
	case "has":
		return boundMethod("has", func(args []Value) Value {
			m, ok := dynUnbox[*T](Arg(args, 0))
			return Bool(ok && s.Has(m))
		}), true
	case "delete":
		return boundMethod("delete", func(args []Value) Value {
			m, ok := dynUnbox[*T](Arg(args, 0))
			return Bool(ok && s.Delete(m))
		}), true
	}
	return Undefined, false
}

// ToValue boxes a WeakRef into a dynamic value. A WeakRef holds no items, so its box
// is not opaque and prints as empty braces, which is what Node prints for one.
func (w *WeakRef[T]) ToValue() Value {
	if w.boxed == nil {
		w.boxed = &Object{kind: KindObject, jsWeak: w}
	}
	return objectValue(w.boxed)
}

func (w *WeakRef[T]) jsWeakName() string { return "WeakRef" }

func (w *WeakRef[T]) jsWeakOpaque() bool { return false }

func (w *WeakRef[T]) jsWeakMember(name string) (Value, bool) {
	if name != "deref" {
		return Undefined, false
	}
	return boundMethod("deref", func([]Value) Value {
		return OptToValue(w.Deref(), dynBox[*T])
	}), true
}

// ToValue boxes a FinalizationRegistry into a dynamic value.
func (r *FinalizationRegistry[T]) ToValue() Value {
	if r.boxed == nil {
		r.boxed = &Object{kind: KindObject, jsWeak: r}
	}
	return objectValue(r.boxed)
}

func (r *FinalizationRegistry[T]) jsWeakName() string { return "FinalizationRegistry" }

func (r *FinalizationRegistry[T]) jsWeakOpaque() bool { return false }

// jsWeakMember reads register and unregister off the live registry. Only one of them
// works through a box. Registering wires the target to runtime.AddCleanup, which is
// generic over the target's own Go type, and that type is what a boxed call does not
// have: the registry is generic only over its held-value type, and the emitted typed
// call is where the target type is known. So a register through the box raises rather
// than registering something whose cleanup could never run, the same choice a boxed
// instance's constructor makes. Unregistering needs no such type: a token is compared
// by reference identity, which the instance the view points back at gives.
func (r *FinalizationRegistry[T]) jsWeakMember(name string) (Value, bool) {
	switch name {
	case "register":
		return boundMethod("register", func([]Value) Value {
			Throw(NewTypeError(FromGoString("bento: registering a target through a boxed FinalizationRegistry is a later slice")))
			return Undefined
		}), true
	case "unregister":
		return boundMethod("unregister", func(args []Value) Value {
			token, ok := Arg(args, 0).classInstance()
			return Bool(ok && r.Unregister(token))
		}), true
	}
	return Undefined, false
}
