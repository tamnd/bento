package value

import (
	"encoding/json"
	"os"
	"testing"
)

// Deep equality is a wall of rules no reading of the docs would settle: whether a
// non-enumerable key counts, whether a hole matches a stored undefined, whether an
// error and a plain object carrying the same name and message are the same thing,
// which of the three modes looks at the prototype. Every one of them is a case below,
// and every answer comes from real Node rather than from a guess.
//
// testdata/nodedeepequal_node24.json was produced by testdata/gen_nodedeepequal_ref.js
// under Node v24.18.0. It records all three modes for each pair, read off
// util.isDeepStrictEqual, assert.deepEqual and assert's skipPrototype option. Cases
// are matched by name in both directions, so a case on one side and not the other
// fails rather than being skipped.

// deepPair is one comparison: the two values, in the order the generator wrote them.
type deepPair struct {
	a, b Value
}

// deepCases builds each named pair. The names match the generator's keys, and each
// value is built the way the generator's JavaScript builds it.
func deepCases(t *testing.T) map[string]deepPair {
	t.Helper()

	str := func(s string) Value { return StringValue(FromGoString(s)) }
	obj := func(pairs ...any) Value {
		o := NewObject()
		for i := 0; i < len(pairs); i += 2 {
			o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
		}
		return o
	}
	arr := func(elems ...Value) Value { return NewArrayValue(elems) }
	bigOf := func(s string) Value {
		v, ok := BigIntFromString(s)
		if !ok {
			t.Fatalf("cannot parse bigint %q", s)
		}
		return v
	}
	pair := func(a, b Value) deepPair { return deepPair{a: a, b: b} }
	fnOf := func(name string) Value {
		return WithName(NewFunc(func([]Value) Value { return Undefined }), name)
	}

	sym := NewSymbol(FromGoString("s"))
	tagged := NewSymbol(FromGoString("t"))
	fn := fnOf("named")

	// A class instance is an object whose prototype carries a constructor, which is
	// the only thing the strict comparison reads off a prototype. Two prototypes with
	// two distinct constructor functions stand in for the generator's Point and Other,
	// and the constructor is non-enumerable the way a class declaration leaves it, so
	// it is not a property the key walk compares.
	classProto := func(name string) Value {
		p := NewObject()
		p.object().defineOwn(FromGoString("constructor"), dataProperty(fnOf(name), true, false, true))
		return p
	}
	pointProto, otherProto := classProto("Point"), classProto("Other")
	instanceOf := func(proto Value) Value {
		o := ObjectCreate(proto)
		o.Set(FromGoString("v"), Number(1))
		return o
	}

	nullProtoWithKey := func() Value {
		o := ObjectCreate(Null)
		o.Set(FromGoString("a"), Number(1))
		return o
	}
	hiddenKey := func() Value {
		o := obj("a", Number(1))
		o.object().defineOwn(FromGoString("hidden"), dataProperty(Number(2), false, false, false))
		return o
	}
	getterOf := func(n float64) Value {
		o := NewObject()
		o.object().defineOwn(FromGoString("a"), accessorProperty(
			NewFunc(func([]Value) Value { return Number(n) }), Undefined, true, true))
		return o
	}
	withSymKey := func(key, val Value) Value {
		o := NewObject()
		o.SetElem(key, val)
		return o
	}
	withTag := func(s string) Value { return withSymKey(SymbolToStringTag(), str(s)) }

	arrayWithNamed := func() Value {
		a := arr(Number(1))
		a.Set(FromGoString("x"), Number(1))
		return a
	}
	// holeFirst builds [, ...elems], an array whose index 0 is absent rather than
	// holding undefined, which the elementwise comparison has its own rule for.
	holeFirst := func(elems ...Value) Value {
		a := NewArrayValue(append([]Value{Undefined}, elems...))
		a.DeleteIndex(0)
		return a
	}

	re := func(pattern, flags string) Value { return RegExpValue(NewRegExpLiteral(pattern, flags)) }
	regexpWithNamed := func() Value {
		r := re("a", "")
		r.Set(FromGoString("x"), Number(1))
		return r
	}
	movedRegExp := func() Value {
		r := NewRegExpLiteral("a", "g")
		r.SetLastIndex(1)
		return RegExpValue(r)
	}

	errOf := func(msg string) Value { return NewError(FromGoString(msg)).ToValue() }
	withCode := func(code string) Value {
		return NewNodeError("Error", code, FromGoString("x")).ToValue()
	}
	aggregateOf := func(reason string) Value {
		return NewAggregateError([]Value{errOf(reason)}, FromGoString("m")).ToValue()
	}

	selfRef := func() Value {
		o := obj("a", Number(1))
		o.Set(FromGoString("self"), o)
		return o
	}
	selfRefArray := func() Value {
		a := arr(Number(1), Undefined)
		a.SetIndex(1, a)
		return a
	}
	mutual := func() (Value, Value) {
		a, b := obj("a", Number(1)), obj("a", Number(1))
		a.Set(FromGoString("self"), b)
		b.Set(FromGoString("self"), a)
		return a, b
	}
	deeperCycle := func() Value {
		outer := obj("a", Number(1))
		inner := obj("a", Number(1))
		inner.Set(FromGoString("self"), outer)
		outer.Set(FromGoString("self"), inner)
		return outer
	}
	nested := func(leaf float64) Value {
		return obj("a", obj("b", obj("c", obj("d", obj("e", Number(leaf))))))
	}
	proxyOf := func(target Value) Value { return NewProxy(target, NewObject()) }

	mutualA, mutualB := mutual()
	shared := obj("v", Number(1))

	return map[string]deepPair{
		// Primitives, which every comparison settles before it looks at an object.
		"identical numbers":         pair(Number(1), Number(1)),
		"different numbers":         pair(Number(1), Number(2)),
		"zero and negative zero":    pair(Number(0), Number(negZero())),
		"negative zero twice":       pair(Number(negZero()), Number(negZero())),
		"nan and nan":               pair(Number(nan()), Number(nan())),
		"nan and number":            pair(Number(nan()), Number(1)),
		"number and numeric string": pair(Number(1), str("1")),
		"number and true":           pair(Number(1), True),
		"null and undefined":        pair(Null, Undefined),
		"null and null":             pair(Null, Null),
		"empty string and zero":     pair(str(""), Number(0)),
		"identical strings":         pair(str("a"), str("a")),
		"different strings":         pair(str("a"), str("b")),
		"identical bigints":         pair(bigOf("1"), bigOf("1")),
		"bigint and number":         pair(bigOf("1"), Number(1)),
		"same symbol":               pair(sym, sym),
		"different symbols":         pair(NewSymbol(FromGoString("a")), NewSymbol(FromGoString("a"))),
		"same function":             pair(fn, fn),
		"two functions":             pair(fnOf(""), fnOf("")),
		"object and number":         pair(NewObject(), Number(1)),
		"null and object":           pair(Null, NewObject()),
		"object and null":           pair(NewObject(), Null),

		// Plain objects.
		"empty objects":                   pair(NewObject(), NewObject()),
		"same one key":                    pair(obj("a", Number(1)), obj("a", Number(1))),
		"different value":                 pair(obj("a", Number(1)), obj("a", Number(2))),
		"loose value":                     pair(obj("a", Number(1)), obj("a", str("1"))),
		"extra key":                       pair(obj("a", Number(1)), obj("a", Number(1), "b", Number(2))),
		"missing key":                     pair(obj("a", Number(1), "b", Number(2)), obj("a", Number(1))),
		"key order":                       pair(obj("a", Number(1), "b", Number(2)), obj("b", Number(2), "a", Number(1))),
		"nested objects":                  pair(obj("a", obj("b", obj("c", Number(1)))), obj("a", obj("b", obj("c", Number(1))))),
		"nested difference":               pair(obj("a", obj("b", obj("c", Number(1)))), obj("a", obj("b", obj("c", Number(2))))),
		"undefined value and missing key": pair(obj("a", Undefined), NewObject()),
		"undefined values":                pair(obj("a", Undefined), obj("a", Undefined)),
		"null prototypes":                 pair(nullProtoWithKey(), nullProtoWithKey()),
		"null prototype and plain":        pair(nullProtoWithKey(), obj("a", Number(1))),
		"non enumerable key ignored":      pair(hiddenKey(), obj("a", Number(1))),
		"accessor and data":               pair(getterOf(1), obj("a", Number(1))),
		"accessor different":              pair(getterOf(1), obj("a", Number(2))),
		"same symbol key":                 pair(withSymKey(sym, Number(1)), withSymKey(sym, Number(1))),
		"symbol key one side":             pair(withSymKey(sym, Number(1)), NewObject()),
		"symbol key different value":      pair(withSymKey(sym, Number(1)), withSymKey(sym, Number(2))),
		"different symbol keys":           pair(withSymKey(sym, Number(1)), withSymKey(tagged, Number(1))),
		"to string tag same":              pair(withTag("X"), withTag("X")),
		"to string tag different":         pair(withTag("X"), withTag("Y")),
		"to string tag one side":          pair(withTag("X"), NewObject()),
		"function property same":          pair(obj("f", fn), obj("f", fn)),
		"function property different":     pair(obj("f", fnOf("")), obj("f", fnOf(""))),

		// Prototypes, which only the strict comparison looks at.
		"same class instances":            pair(instanceOf(pointProto), instanceOf(pointProto)),
		"different classes":               pair(instanceOf(pointProto), instanceOf(otherProto)),
		"class instance and plain object": pair(instanceOf(pointProto), obj("v", Number(1))),

		// Arrays.
		"empty arrays":                  pair(arr(), arr()),
		"same elements":                 pair(arr(Number(1), Number(2)), arr(Number(1), Number(2))),
		"different length":              pair(arr(Number(1)), arr(Number(1), Number(2))),
		"different element":             pair(arr(Number(1), Number(2)), arr(Number(1), Number(3))),
		"loose element":                 pair(arr(Number(1)), arr(str("1"))),
		"nested arrays":                 pair(arr(arr(Number(1)), arr(Number(2))), arr(arr(Number(1)), arr(Number(2)))),
		"array and object":              pair(arr(), NewObject()),
		"object and array":              pair(NewObject(), arr()),
		"array named property one side": pair(arrayWithNamed(), arr(Number(1))),
		"array named property both":     pair(arrayWithNamed(), arrayWithNamed()),
		"hole and undefined element":    pair(holeFirst(Number(1)), arr(Undefined, Number(1))),
		"holes both sides":              pair(holeFirst(Number(1)), holeFirst(Number(1))),
		"hole and value":                pair(holeFirst(Number(1)), arr(Number(1), Number(1))),
		"null and undefined element":    pair(arr(Null), arr(Undefined)),
		"array of objects":              pair(arr(obj("a", Number(1))), arr(obj("a", Number(1)))),

		// Regexps, which are their pattern, their flags and their position.
		"same regexps":                   pair(re("ab+c", "gi"), re("ab+c", "gi")),
		"different source":               pair(re("a", ""), re("b", "")),
		"different flags":                pair(re("a", "g"), re("a", "i")),
		"different last index":           pair(movedRegExp(), re("a", "g")),
		"regexp and object":              pair(re("a", ""), NewObject()),
		"regexp named property both":     pair(regexpWithNamed(), regexpWithNamed()),
		"regexp named property one side": pair(regexpWithNamed(), re("a", "")),

		// Errors, whose stack is deliberately not part of the comparison.
		"same errors":                 pair(errOf("x"), errOf("x")),
		"different messages":          pair(errOf("x"), errOf("y")),
		"different error names":       pair(errOf("x"), NewTypeError(FromGoString("x")).ToValue()),
		"error and plain object":      pair(errOf("x"), obj("name", str("Error"), "message", str("x"))),
		"plain object and error":      pair(obj("name", str("Error"), "message", str("x")), errOf("x")),
		"errors with same code":       pair(withCode("E"), withCode("E")),
		"errors with different code":  pair(withCode("E"), withCode("F")),
		"aggregate errors":            pair(aggregateOf("a"), aggregateOf("a")),
		"aggregate different reasons": pair(aggregateOf("a"), aggregateOf("b")),

		// Cycles, which is what the memo set is for.
		"shared subobject one side":    pair(obj("x", shared, "y", shared), obj("x", obj("v", Number(1)), "y", obj("v", Number(1)))),
		"deep nesting":                 pair(nested(1), nested(1)),
		"deep nesting difference":      pair(nested(1), nested(2)),
		"self referencing objects":     pair(selfRef(), selfRef()),
		"mutually referencing objects": pair(mutualA, mutualB),
		"cycle and deeper cycle":       pair(selfRef(), deeperCycle()),
		"self referencing arrays":      pair(selfRefArray(), selfRefArray()),

		// Proxies, which are compared as whatever their traps answer.
		"proxy over object and object":           pair(proxyOf(obj("a", Number(1))), obj("a", Number(1))),
		"proxy over object and different object": pair(proxyOf(obj("a", Number(1))), obj("a", Number(2))),
		"proxy over array and array":             pair(proxyOf(arr(Number(1))), arr(Number(1))),
	}
}

// deepAnswer is one pair's verdict in each of the three modes.
type deepAnswer struct {
	Strict        bool `json:"strict"`
	Loose         bool `json:"loose"`
	SkipPrototype bool `json:"skipPrototype"`
}

// TestDeepEqualMatchesNode holds every mode of the port against the answers Node gave
// for the same pair. Each pair is compared both ways round, since deep equality is
// symmetric and the port's branches are not: the array test, the tag test and the
// error test all read the first value and then ask about the second.
func TestDeepEqualMatchesNode(t *testing.T) {
	want := readDeepReference(t, "testdata/nodedeepequal_node24.json")
	got := deepCases(t)

	for name, c := range got {
		answer, ok := want[name]
		if !ok {
			t.Errorf("the Go side builds case %q but the reference has no entry; regenerate testdata", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			modes := []struct {
				what string
				fn   func(a, b Value) bool
				want bool
			}{
				{"DeepStrictEqual", DeepStrictEqual, answer.Strict},
				{"DeepEqual", DeepEqual, answer.Loose},
				{"DeepStrictEqualSkipPrototype", DeepStrictEqualSkipPrototype, answer.SkipPrototype},
			}
			for _, m := range modes {
				if got := m.fn(c.a, c.b); got != m.want {
					t.Errorf("%s(a, b) = %v, node says %v", m.what, got, m.want)
				}
				if got := m.fn(c.b, c.a); got != m.want {
					t.Errorf("%s(b, a) = %v, node says %v", m.what, got, m.want)
				}
			}
		})
	}
	for name := range want {
		if _, ok := got[name]; !ok {
			t.Errorf("the reference has case %q but the Go side does not build it", name)
		}
	}
}

// readDeepReference loads the Node-generated answers.
func readDeepReference(t *testing.T, path string) map[string]deepAnswer {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the reference: %v", err)
	}
	var out map[string]deepAnswer
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("parsing the reference: %v", err)
	}
	return out
}

// TestIsDeepStrictEqualTakesItsArgumentsLikeNode covers the entry point rather than
// the comparison. util.isDeepStrictEqual() with no arguments compares undefined
// against undefined and is true, and with one argument compares that value against
// undefined, which is what Node answers and what the variadic form has to preserve.
func TestIsDeepStrictEqualTakesItsArgumentsLikeNode(t *testing.T) {
	if !NodeIsDeepStrictEqual() {
		t.Error("isDeepStrictEqual() = false, node says true")
	}
	if !NodeIsDeepStrictEqual(Undefined) {
		t.Error("isDeepStrictEqual(undefined) = false, node says true")
	}
	if NodeIsDeepStrictEqual(NewObject()) {
		t.Error("isDeepStrictEqual({}) = true, node says false")
	}
	if !NodeIsDeepStrictEqual(NewObject(), NewObject()) {
		t.Error("isDeepStrictEqual({}, {}) = false, node says true")
	}
}
