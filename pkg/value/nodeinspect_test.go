package value

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// util.inspect is what console.log prints, so its exact spelling is compatibility
// rather than cosmetics: programs are read against "{ a: 1, b: 'x' }" and tests are
// written against it. These hold the port to real Node output rather than to a
// description of it, so a rule read wrong out of Node's source fails here.
//
// testdata/nodeinspect_node24.json was produced by testdata/gen_nodeinspect_ref.js
// under Node v24.18.0. Each case is matched by name: the generator builds the value
// in JavaScript and records what Node printed, and inspectCases below builds the
// same value through the value model. A name in one and not the other fails, so the
// two lists cannot drift apart quietly.

// inspectCases builds each named value. The names match the generator's keys.
func inspectCases(t *testing.T) map[string]Value {
	t.Helper()

	bigOf := func(s string) Value {
		v, ok := BigIntFromString(s)
		if !ok {
			t.Fatalf("cannot parse bigint %q", s)
		}
		return v
	}
	obj := func(pairs ...any) Value {
		o := NewObject()
		for i := 0; i < len(pairs); i += 2 {
			o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
		}
		return o
	}
	str := func(s string) Value { return StringValue(FromGoString(s)) }
	nums := func(n int) Value {
		elems := make([]Value, n)
		for i := range elems {
			elems[i] = Number(float64(i))
		}
		return NewArrayValue(elems)
	}

	// The values built once and referred to twice, so identity holds the way it does
	// in the generator: a cycle is only a cycle if it is the same object.
	circularObject := obj("a", Number(1))
	circularObject.Set(FromGoString("self"), circularObject)
	circularArray := NewArrayValue([]Value{Number(1)})
	circularArray.object().elems = append(circularArray.object().elems, circularArray)
	sharedNested := obj("n", Number(1))
	deepCircular := obj("a", obj("b", NewObject()))
	deepCircular.Get(FromGoString("a")).Get(FromGoString("b")).Set(FromGoString("back"), deepCircular)

	arrayWithProp := NewArrayValue([]Value{Number(1), Number(2)})
	arrayWithProp.Set(FromGoString("x"), Number(5))

	hole := Value{kind: KindHole}
	nullProtoWithKeys := ObjectCreate(Null)
	nullProtoWithKeys.Set(FromGoString("a"), Number(1))

	// A constructor-named instance: an object whose prototype carries a named
	// constructor function, which is the shape Node reads a class name out of.
	proto := NewObject()
	proto.Set(FromGoString("constructor"), WithName(NewFunc(func([]Value) Value { return Undefined }), "Named"))
	classInstance := ObjectCreate(proto)
	classInstance.Set(FromGoString("a"), Number(1))

	symbolKeyed := NewObject()
	symbolKeyed.SetElem(NewSymbol(FromGoString("k")), Number(1))
	symbolKeyed.Set(FromGoString("plain"), Number(2))

	// The collections are built through the dynamic constructors, which is what a
	// `new Map()` with no key type lowers to and so the box these cases render.
	mapOf := func(kv ...Value) Value {
		m := NewDynMap[Value]()
		for i := 0; i < len(kv); i += 2 {
			m.Set(kv[i], kv[i+1])
		}
		return m.ToValue()
	}
	setOf := func(members ...Value) Value {
		s := NewDynSet()
		for _, v := range members {
			s.Add(v)
		}
		return s.ToValue()
	}
	counted := func(n int) []Value {
		out := make([]Value, n)
		for i := range out {
			out[i] = Number(float64(i))
		}
		return out
	}
	countedPairs := func(n int) []Value {
		out := make([]Value, 0, 2*n)
		for i := 0; i < n; i++ {
			out = append(out, Number(float64(i)), Number(float64(i)))
		}
		return out
	}
	mapWithProp := mapOf(Number(1), Number(2))
	mapWithProp.Set(FromGoString("x"), Number(5))
	setWithProp := setOf(Number(1))
	setWithProp.Set(FromGoString("x"), Number(5))
	circularMap := NewDynMap[Value]()
	circularMapValue := circularMap.ToValue()
	circularMap.Set(str("self"), circularMapValue)
	circularSet := NewDynSet()
	circularSetValue := circularSet.ToValue()
	circularSet.Add(circularSetValue)

	errWithCode := NewNodeError("TypeError", "E1", FromGoString("bad"))
	errWithProps := NewError(FromGoString("x")).ToValue()
	errWithProps.Set(FromGoString("foo"), Number(1))
	errWithProps.Set(FromGoString("bar"), str("y"))

	return map[string]Value{
		"undefined":                     Undefined,
		"null":                          Null,
		"true":                          True,
		"false":                         False,
		"zero":                          Number(0),
		"negative zero":                 Number(negZero()),
		"integer":                       Number(42),
		"fraction":                      Number(1.5),
		"nan":                           Number(nan()),
		"infinity":                      Number(inf(1)),
		"negative infinity":             Number(inf(-1)),
		"large number":                  Number(1e21),
		"bigint":                        bigOf("1"),
		"negative bigint":               bigOf("-12345678901234567890"),
		"string":                        str("hi"),
		"empty string":                  str(""),
		"string with single quote":      str("he'llo"),
		"string with both quotes":       str(`he'llo "there"`),
		"string with every quote":       str("a'b\"c`d"),
		"string with template opener":   str("a'b\"c${d}"),
		"string with newline":           str("a\nb"),
		"string with tab and backslash": str("a\tb\\c"),
		"string with control char":      str("a\x01b"),
		"string with del":               str("a\x7fb"),
		"string with lone surrogate":    StringValue(fromUnits('a', 0xd800, 'b')),
		"string with surrogate pair":    str("a\U0001f600b"),
		"long single line string":       str(strings.Repeat("x", 100)),
		"long multiline string":         str(strings.Repeat("line one is quite long here\n", 4)),
		"symbol":                        NewSymbol(FromGoString("s")),
		"symbol no description":         NewSymbolNoDesc(),

		"empty object":              NewObject(),
		"one key":                   obj("a", Number(1)),
		"two keys":                  obj("a", Number(1), "b", str("x")),
		"numeric and odd keys":      obj("a-b", Number(1), "2", Number(3), "valid_id", Number(4)),
		"dollar key":                obj("$a", Number(1)),
		"undefined and null values": obj("a", Undefined, "b", Null),
		"seven short keys": obj("a", Number(1), "b", Number(2), "c", Number(3), "d", Number(4),
			"e", Number(5), "f", Number(6), "g", Number(7)),
		"three long keys": obj(
			"longKeyName1", str("aaaaaaaaaaaaaaaaaaaa"),
			"longKeyName2", str("bbbbbbbbbbbbbbbbbbbb"),
			"longKeyName3", str("cccccccccccccccccccc")),
		"nested past depth":        obj("a", obj("b", obj("c", obj("d", Number(1))))),
		"nested at depth":          obj("a", obj("b", obj("c", Number(1)))),
		"null prototype":           ObjectCreate(Null),
		"null prototype with keys": nullProtoWithKeys,
		"constructor named":        classInstance,
		"symbol keyed":             symbolKeyed,
		"nested string quoting":    obj("s", str("he'llo")),
		"nested newline string":    obj("s", str("a\nb")),
		"bigint value":             obj("a", bigOf("1")),
		"negative zero value":      obj("n", Number(negZero())),
		"proto key":                obj("__proto__", Number(1)),

		"empty array":   NewArrayValue(nil),
		"three numbers": NewArrayValue([]Value{Number(1), Number(2), Number(3)}),
		"nested arrays past depth": NewArrayValue([]Value{NewArrayValue([]Value{
			Number(1), NewArrayValue([]Value{
				Number(2), NewArrayValue([]Value{
					Number(3), NewArrayValue([]Value{Number(4)})})})})}),
		"array with named property":  arrayWithProp,
		"sparse array":               NewArrayValue([]Value{Number(1), hole, Number(3)}),
		"sparse tail":                NewArrayValue([]Value{Number(1), hole, hole, hole}),
		"eight numbers":              nums(8),
		"seven numbers":              nums(7),
		"twenty numbers":             nums(20),
		"hundred and twenty numbers": nums(120),
		"ten strings": NewArrayValue([]Value{
			str("item0"), str("item1"), str("item2"), str("item3"), str("item4"),
			str("item5"), str("item6"), str("item7"), str("item8"), str("item9")}),
		"array of objects": NewArrayValue([]Value{obj("a", Number(1)), obj("b", Number(2))}),
		"mixed array":      NewArrayValue([]Value{Number(1), str("two"), True, Null, Undefined}),

		"anonymous function": NewFunc(func([]Value) Value { return Undefined }),
		"named function":     WithName(NewFunc(func([]Value) Value { return Undefined }), "foo"),
		"function with property": func() Value {
			f := WithName(NewFunc(func([]Value) Value { return Undefined }), "bar")
			f.Set(FromGoString("x"), Number(1))
			return f
		}(),
		"regexp":           RegExpValue(NewRegExpLiteral("ab+c", "gi")),
		"empty regexp":     RegExpValue(NewRegExpLiteral("", "")),
		"regexp in object": obj("r", RegExpValue(NewRegExpLiteral("a", ""))),

		"error":                 NewError(FromGoString("x")).ToValue(),
		"error no message":      NewError(FromGoString("")).ToValue(),
		"type error with code":  errWithCode.ToValue(),
		"error in array":        NewArrayValue([]Value{NewError(FromGoString("x")).ToValue()}),
		"error with properties": errWithProps,

		"empty map":               mapOf(),
		"map of numbers":          mapOf(Number(1), Number(2), Number(3), Number(4)),
		"map of strings":          mapOf(str("a"), str("x"), str("b"), str("y")),
		"map with object value":   mapOf(str("a"), obj("b", Number(1))),
		"map with object key":     mapOf(obj("a", Number(1)), str("v")),
		"map with named property": mapWithProp,
		"nested maps past depth": mapOf(str("a"), mapOf(str("b"),
			mapOf(str("c"), mapOf(str("d"), Number(1))))),
		"long map": mapOf(
			str("alpha"), str("aaaaaaaaaaaaaaa"),
			str("beta"), str("bbbbbbbbbbbbbbb"),
			str("gamma"), str("ccccccccccccccc")),
		"map in object":    obj("m", mapOf(Number(1), Number(2))),
		"map in array":     NewArrayValue([]Value{mapOf(Number(1), Number(2))}),
		"twenty entry map": mapOf(countedPairs(20)...),

		"empty set":               setOf(),
		"set of numbers":          setOf(Number(1), Number(2), Number(3)),
		"set of strings":          setOf(str("a"), str("b")),
		"set of objects":          setOf(obj("a", Number(1)), obj("b", Number(2))),
		"set with named property": setWithProp,
		"nested sets past depth":  setOf(setOf(setOf(setOf(Number(1))))),
		"thirty number set":       setOf(counted(30)...),
		"set in object":           obj("s", setOf(Number(1))),
		"map of sets":             mapOf(str("a"), setOf(Number(1), Number(2))),

		"circular object":              circularObject,
		"circular map":                 circularMapValue,
		"circular set":                 circularSetValue,
		"circular array":               circularArray,
		"deep circular":                deepCircular,
		"two references to one object": obj("a", sharedNested, "b", sharedNested),
	}
}

// TestInspectMatchesNode is the whole point: every case renders byte for byte the
// way Node v24.18.0 rendered it.
func TestInspectMatchesNode(t *testing.T) {
	raw, err := os.ReadFile("testdata/nodeinspect_node24.json")
	if err != nil {
		t.Fatalf("reading the reference: %v", err)
	}
	var want map[string]string
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("parsing the reference: %v", err)
	}
	got := inspectCases(t)

	for name, expected := range want {
		v, ok := got[name]
		if !ok {
			t.Errorf("the reference has case %q but the Go side does not build it", name)
			continue
		}
		if actual := NodeInspect(v).ToGoString(); actual != expected {
			t.Errorf("case %q:\n got %q\nwant %q", name, actual, expected)
		}
	}
	for name := range got {
		if _, ok := want[name]; !ok {
			t.Errorf("the Go side builds case %q but the reference has no entry; regenerate testdata", name)
		}
	}
}

// TestConsoleValueLeavesAStringAlone pins the one argument console.log does not
// inspect. A logged string is its own text: console.log("hi") is hi, not 'hi', and
// a build that quoted it would put quotes around every log line a program writes.
func TestConsoleValueLeavesAStringAlone(t *testing.T) {
	if got := ConsoleValue(StringValue(FromGoString("hi"))).ToGoString(); got != "hi" {
		t.Errorf("ConsoleValue(\"hi\") = %q, want hi", got)
	}
	// Nested in a container the same string quotes, since there it has to be told
	// apart from the punctuation around it.
	if got := ConsoleValue(NewArrayValue([]Value{StringValue(FromGoString("hi"))})).ToGoString(); got != "[ 'hi' ]" {
		t.Errorf("ConsoleValue([\"hi\"]) = %q, want [ 'hi' ]", got)
	}
}

// TestConsoleValueInspectsAnObject is the reproducer for the slice. A compiled
// program logging an object printed "[object Object]", which is what a string
// coercion answers and is never what anyone logging an object wanted to see.
func TestConsoleValueInspectsAnObject(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	o.Set(FromGoString("b"), StringValue(FromGoString("x")))
	if got := ConsoleValue(o).ToGoString(); got != "{ a: 1, b: 'x' }" {
		t.Errorf("ConsoleValue({a:1,b:'x'}) = %q, want { a: 1, b: 'x' }", got)
	}
}

// TestConsoleValueRendersASymbol pins the kind that used to throw. String(sym) is a
// TypeError by design, so the old console path, which was a string coercion, could
// not print a symbol at all; Node prints one.
func TestConsoleValueRendersASymbol(t *testing.T) {
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("logging a symbol threw %v", rec)
		}
	}()
	if got := ConsoleValue(NewSymbol(FromGoString("s"))).ToGoString(); got != "Symbol(s)" {
		t.Errorf("ConsoleValue(Symbol(s)) = %q, want Symbol(s)", got)
	}
}

// TestAFunctionNameIsNotEnumerable pins the property model this slice corrected.
// Function.prototype.name is non-enumerable in JavaScript, and bento stored it as
// an ordinary write, so Object.keys of a function reported it and a logged callback
// would have read as "[Function: foo] { name: 'foo' }".
func TestAFunctionNameIsNotEnumerable(t *testing.T) {
	f := WithName(NewFunc(func([]Value) Value { return Undefined }), "foo")
	if n := f.OwnEnumerableKeys().Len(); n != 0 {
		t.Errorf("Object.keys(f) has %v entries, want none", n)
	}
	if got := f.Get(FromGoString("name")); got.Kind() != KindString || got.AsString().ToGoString() != "foo" {
		t.Errorf("f.name = %v, want foo", got.Kind())
	}
}

// TestInspectingAProxyShowsItsTarget pins what a logged proxy reads as. Node's
// showProxy option is off by default, so a proxy prints as the thing it stands for
// rather than as the target-and-handler pair, which is what a program using one as
// a transparent wrapper expects to see.
func TestInspectingAProxyShowsItsTarget(t *testing.T) {
	target := NewObject()
	target.Set(FromGoString("a"), Number(1))
	p := NewProxy(target, NewObject())
	if got := NodeInspect(p).ToGoString(); got != "{ a: 1 }" {
		t.Errorf("inspecting a proxy = %q, want { a: 1 }", got)
	}
}

// TestALongStringIsCut pins maxStringLength. The cap is 10000 characters, and a
// program logging a whole file wants the head of it plus a count, not the file.
func TestALongStringIsCut(t *testing.T) {
	got := NodeInspect(StringValue(FromGoString(strings.Repeat("a", 10003)))).ToGoString()
	if !strings.HasSuffix(got, "'... 3 more characters") {
		t.Errorf("a 10003-character string ends %q, want the trailing count", got[len(got)-40:])
	}
}

// fromUnits builds a string from raw UTF-16 code units, the only way to write a
// lone surrogate: it is not valid Unicode, so no Go string literal denotes it.
// nan, inf and negZero live in json_test.go, which needs the same three values for
// the same reason: none of them is writable as a Go literal.
func fromUnits(units ...uint16) BStr { return FromUTF16(units) }
