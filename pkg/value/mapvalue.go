// This file bridges a *Map[K, V] into the dynamic value.Value world: the box a map
// takes when it flows into an any binding, a console.log argument, or an assert
// call. It is the Map half of the collection boxing wall; setvalue.go is the Set
// half and reads the same way.
//
// The box is a view, not a copy. An ordinary object value carries the live map in
// its jsMap field, so typeof reports "object", the value is truthy, and a read or a
// write through the box reaches the same entries the typed side holds: a boxed
// map.set(k, v) is visible to the typed map.get(k) that follows it. The map caches
// its own box, so boxing one map twice yields one object and `a === b` holds for two
// bindings of the same map, which a fresh box per crossing would get wrong.
//
// A Map has no own enumerable properties in JavaScript, which is why the member
// surface is a switch here rather than properties installed on the box: the key
// walks that back console.log, Object.keys, and JSON.stringify see an empty object,
// which is exactly what they see in Node, while a read of .size or .get finds the
// live collection.

package value

// mapBacking is the live typed map behind a box. Every method is in terms of boxed
// values, so the box's member surface is written once against this interface while
// each *Map[K, V] instantiation keeps its own monomorphized storage. A key that the
// typed map's key type could not hold is reported absent by the reads and raises on
// a write, which dynUnbox and dynUnboxOrThrow decide.
type mapBacking interface {
	jsSize() int
	jsEntry(i int) (Value, Value)
	jsGet(k Value) Value
	jsSet(k, v Value)
	jsHas(k Value) bool
	jsDelete(k Value) bool
	jsClear()
	// jsBox is the map's own box, the value map.set(k, v) returns and the third
	// argument forEach hands its callback.
	jsBox() Value
}

// ToValue boxes a typed map into a dynamic value. The box is built once and kept on
// the map, so every crossing of the same map hands back the same object: a JavaScript
// Map is a reference, and two boxes would compare unequal under === and print as two
// values under console.log even though the program has one map.
func (m *Map[K, V]) ToValue() Value {
	if m.boxed == nil {
		m.boxed = &Object{kind: KindObject, jsMap: m}
	}
	return objectValue(m.boxed)
}

// The mapBacking implementation. Each method is the boxed spelling of the concrete
// method beside it in mapobj.go, with dynBox lifting an entry out and dynUnbox
// deciding whether a dynamic key could be one of this map's keys at all.
func (m *Map[K, V]) jsSize() int { return len(m.keys) }

func (m *Map[K, V]) jsEntry(i int) (Value, Value) {
	return dynBox(m.keys[i]), dynBox(m.vals[i])
}

func (m *Map[K, V]) jsGet(k Value) Value {
	tk, ok := dynUnbox[K](k)
	if !ok {
		return Undefined
	}
	if i := m.find(tk); i >= 0 {
		return dynBox(m.vals[i])
	}
	return Undefined
}

func (m *Map[K, V]) jsSet(k, v Value) {
	m.Set(dynUnboxOrThrow[K](k, "key"), dynUnboxOrThrow[V](v, "value"))
}

func (m *Map[K, V]) jsHas(k Value) bool {
	tk, ok := dynUnbox[K](k)
	return ok && m.find(tk) >= 0
}

func (m *Map[K, V]) jsDelete(k Value) bool {
	tk, ok := dynUnbox[K](k)
	return ok && m.Delete(tk)
}

func (m *Map[K, V]) jsClear() { m.Clear() }

func (m *Map[K, V]) jsBox() Value { return m.ToValue() }

// asMap returns the live map a value boxes, or nil when the value is not a map box.
// It is the probe the reads, the inspector, and the deep comparison make before their
// ordinary object handling, the same shape asRegExp has.
func (v Value) asMap() mapBacking {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsMap
}

// mapGet reads a member off a boxed map, mirroring the concrete methods the
// statically-typed path emits: .size reports the entry count and each method reads a
// callable bound to the live map, so a dynamic m.set(k, v) lands in the same storage
// the typed m.get(k) reads. A name that is not a Map member reports ok=false, so the
// caller climbs the ordinary chain and answers undefined for a miss, which is what a
// read of an unrelated name off a Map does in JavaScript.
func mapGet(m mapBacking, name string) (Value, bool) {
	switch name {
	case "size":
		return Number(float64(m.jsSize())), true
	case "get":
		return boundMethod("get", func(args []Value) Value {
			return m.jsGet(Arg(args, 0))
		}), true
	case "set":
		// set returns the map itself, so a chained m.set(1, 'a').set(2, 'b') reads
		// through the box the way it does on the typed side.
		return boundMethod("set", func(args []Value) Value {
			m.jsSet(Arg(args, 0), Arg(args, 1))
			return m.jsBox()
		}), true
	case "has":
		return boundMethod("has", func(args []Value) Value {
			return Bool(m.jsHas(Arg(args, 0)))
		}), true
	case "delete":
		return boundMethod("delete", func(args []Value) Value {
			return Bool(m.jsDelete(Arg(args, 0)))
		}), true
	case "clear":
		return boundMethod("clear", func(args []Value) Value {
			m.jsClear()
			return Undefined
		}), true
	case "forEach":
		// The callback takes (value, key, map), the order Map.prototype.forEach passes
		// and the reverse of the entry order everywhere else. A thisArg is dropped, the
		// same as every other boxed callback here: bento's functions take no this.
		return boundMethod("forEach", func(args []Value) Value {
			cb := Arg(args, 0)
			for i := 0; i < m.jsSize(); i++ {
				k, v := m.jsEntry(i)
				cb.Call(v, k, m.jsBox())
			}
			return Undefined
		}), true
	case "keys":
		return boundMethod("keys", func(args []Value) Value { return mapIterator(m, mapIterKeys) }), true
	case "values":
		return boundMethod("values", func(args []Value) Value { return mapIterator(m, mapIterValues) }), true
	case "entries":
		return boundMethod("entries", func(args []Value) Value { return mapIterator(m, mapIterEntries) }), true
	}
	return Undefined, false
}

// mapSymGet reads a symbol-keyed member off a boxed map: the default iterator, which
// is entries, and the toStringTag that makes Object.prototype.toString.call(map) read
// "[object Map]". They are answered here rather than installed as own symbol
// properties so the key walks stay empty, the way a real Map's do.
func mapSymGet(m mapBacking, key *Symbol) (Value, bool) {
	switch key {
	case symbolIterator:
		return boundMethod("[Symbol.iterator]", func(args []Value) Value {
			return mapIterator(m, mapIterEntries)
		}), true
	case symbolToStringTag:
		return StringValue(FromGoString("Map")), true
	}
	return Undefined, false
}

// The three projections a map iterator yields, the same split an array's keys,
// values and entries take.
const (
	mapIterKeys = iota
	mapIterValues
	mapIterEntries
)

// mapIterator builds the iterator object map.keys(), map.values(), map.entries() and
// a for...of over a boxed map walk. It snapshots the entries, which is the behavior
// the typed side already documents for Keys and Values: an entry added during the
// loop is not visited. A live cursor over a collection being mutated is one slice for
// both the typed and the boxed side, not two.
func mapIterator(m mapBacking, kind int) Value {
	n := m.jsSize()
	keys := make([]Value, n)
	vals := make([]Value, n)
	for i := 0; i < n; i++ {
		keys[i], vals[i] = m.jsEntry(i)
	}
	i := 0
	return iterObject(func() IterResult {
		if i >= len(keys) {
			return IterResult{Value: Undefined, Done: true}
		}
		k, v := keys[i], vals[i]
		i++
		switch kind {
		case mapIterKeys:
			return IterResult{Value: k}
		case mapIterValues:
			return IterResult{Value: v}
		default:
			return IterResult{Value: NewArrayValue([]Value{k, v})}
		}
	})
}

// boundMethod wraps a collection method as a named callable, the value a read of
// m.get or s.add off a box hands back. The name rides the function so console.log of
// the method reads "[Function: get]" rather than "[Function (anonymous)]", which is
// what Node prints for a built-in method read off a collection.
func boundMethod(name string, fn callFn) Value {
	return WithName(NewFunc(fn), name)
}
