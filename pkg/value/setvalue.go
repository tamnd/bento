// This file bridges a *Set[T] into the dynamic value.Value world, the Set half of
// what mapvalue.go does for a Map. It reads the same way: the box is a view onto the
// live set rather than a copy, the set caches its own box so identity holds across
// crossings, and the member surface is a switch rather than installed properties
// because a Set carries no own enumerable properties in JavaScript.
//
// The set algebra (union, intersection, difference and the rest of ES2025) is not on
// the box. Each of those takes another set-like and builds a third set, which needs
// the argument's typed element to match the receiver's; a dynamic argument does not
// carry one. A read of s.union off a box is undefined, so a call of it raises the
// "is not a function" TypeError rather than answering a wrong set, and the typed path
// keeps its own direct lowering of all seven.

package value

// setBacking is the live typed set behind a box, the Set counterpart of mapBacking.
// Every method is in terms of boxed values, so one member surface serves every
// *Set[T] instantiation.
type setBacking interface {
	jsSize() int
	jsMember(i int) Value
	jsAdd(v Value)
	jsHas(v Value) bool
	jsDelete(v Value) bool
	jsClear()
	// jsBox is the set's own box, the value set.add(v) returns and the third argument
	// forEach hands its callback.
	jsBox() Value
}

// ToValue boxes a typed set into a dynamic value, building the box once and keeping
// it on the set so two crossings of one set hand back one object.
func (s *Set[T]) ToValue() Value {
	if s.boxed == nil {
		s.boxed = &Object{kind: KindObject, jsSet: s}
	}
	return objectValue(s.boxed)
}

// The setBacking implementation, each method the boxed spelling of the concrete one
// beside it in setobj.go.
func (s *Set[T]) jsSize() int { return len(s.members) }

func (s *Set[T]) jsMember(i int) Value { return dynBox(s.members[i]) }

func (s *Set[T]) jsAdd(v Value) { s.Add(dynUnboxOrThrow[T](v, "member")) }

func (s *Set[T]) jsHas(v Value) bool {
	tv, ok := dynUnbox[T](v)
	return ok && s.find(tv) >= 0
}

func (s *Set[T]) jsDelete(v Value) bool {
	tv, ok := dynUnbox[T](v)
	return ok && s.Delete(tv)
}

func (s *Set[T]) jsClear() { s.Clear() }

func (s *Set[T]) jsBox() Value { return s.ToValue() }

// asSet returns the live set a value boxes, or nil when the value is not a set box.
func (v Value) asSet() setBacking {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsSet
}

// setGet reads a member off a boxed set. Like a Map's, each method is a callable
// bound to the live set, so a dynamic s.add(v) is visible to the typed s.has(v) that
// follows it, and a name that is not a Set member reports ok=false so the read climbs
// the ordinary chain to undefined.
func setGet(s setBacking, name string) (Value, bool) {
	switch name {
	case "size":
		return Number(float64(s.jsSize())), true
	case "add":
		// add returns the set itself, so a chained s.add(1).add(2) reads through the box.
		return boundMethod("add", func(args []Value) Value {
			s.jsAdd(Arg(args, 0))
			return s.jsBox()
		}), true
	case "has":
		return boundMethod("has", func(args []Value) Value {
			return Bool(s.jsHas(Arg(args, 0)))
		}), true
	case "delete":
		return boundMethod("delete", func(args []Value) Value {
			return Bool(s.jsDelete(Arg(args, 0)))
		}), true
	case "clear":
		return boundMethod("clear", func(args []Value) Value {
			s.jsClear()
			return Undefined
		}), true
	case "forEach":
		// The callback takes (value, value, set): a Set has no key, so the specification
		// passes the member twice to keep the callback shape a Map's forEach has.
		return boundMethod("forEach", func(args []Value) Value {
			cb := Arg(args, 0)
			for i := 0; i < s.jsSize(); i++ {
				m := s.jsMember(i)
				cb.Call(m, m, s.jsBox())
			}
			return Undefined
		}), true
	case "values", "keys":
		// A Set's keys are its values, so both names read the same iterator, which is
		// also what Set.prototype.keys being Set.prototype.values means in the language.
		return boundMethod(name, func(args []Value) Value { return setIterator(s, false) }), true
	case "entries":
		return boundMethod("entries", func(args []Value) Value { return setIterator(s, true) }), true
	}
	return Undefined, false
}

// setSymGet reads a symbol-keyed member off a boxed set: the default iterator, which
// is values, and the toStringTag behind "[object Set]".
func setSymGet(s setBacking, key *Symbol) (Value, bool) {
	switch key {
	case symbolIterator:
		return boundMethod("[Symbol.iterator]", func(args []Value) Value {
			return setIterator(s, false)
		}), true
	case symbolToStringTag:
		return StringValue(FromGoString("Set")), true
	}
	return Undefined, false
}

// setIterator builds the iterator object set.values(), set.keys(), set.entries() and
// a for...of over a boxed set walk. entries yields each member twice as a pair, the
// [v, v] shape the specification gives a Set's entries so it reads like a Map's. It
// snapshots the members for the reason mapIterator does.
func setIterator(s setBacking, entries bool) Value {
	members := make([]Value, s.jsSize())
	for i := range members {
		members[i] = s.jsMember(i)
	}
	i := 0
	return iterObject(func() IterResult {
		if i >= len(members) {
			return IterResult{Value: Undefined, Done: true}
		}
		m := members[i]
		i++
		if entries {
			return IterResult{Value: NewArrayValue([]Value{m, m})}
		}
		return IterResult{Value: m}
	})
}
