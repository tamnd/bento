package value

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// This file compares the assert module against Node's own, message for message. The
// reference in testdata/nodeassert_node24.json was produced by
// testdata/gen_nodeassert_ref.js under Node v24.18.0, and every case in it is built
// here by the same name: the generator names its values and its methods, this file
// builds the same values and calls the same methods, and a name on one side and not
// the other fails rather than being skipped.
//
// The messages are compared byte for byte, because that is what a failing test prints
// and the only thing that makes an assertion diagnosable. Three cases are expected to
// differ, listed in assertKnownDivergences below with the reason; every other
// difference is a failure.

// assertRefCase is one recorded answer from Node.
type assertRefCase struct {
	Threw            bool   `json:"threw"`
	Name             string `json:"name"`
	Code             string `json:"code"`
	Message          string `json:"message"`
	GeneratedMessage bool   `json:"generatedMessage"`
	Operator         string `json:"operator"`
}

// assertKnownDivergences are the cases whose message cannot match Node's, with the
// message bento produces instead. Every one of them is assert.ok's generated message:
// Node reads the source text of the failed expression back out of the file the stack
// frame points at and quotes it, and a compiled binary has no source file to read. The
// message is the one Node itself falls back to when it cannot find the source, so a
// test that fails prints the comparison rather than nothing.
var assertKnownDivergences = map[string]string{
	"ok(false, undefined)": "false == true",
	"ok(zero, undefined)":  "0 == true",
	"ok(null, undefined)":  "null == true",
}

// assertSharedValues holds the values a case names on both sides when it needs one
// reference rather than two equal ones, the same singletons the generator keeps.
type assertSharedValues struct {
	object       Value
	nestedObject Value
	array        Value
	function     Value
	err          Value
}

// assertValue builds the value a case names. The names are the generator's.
func assertValue(t *testing.T, shared *assertSharedValues, name string) Value {
	t.Helper()
	obj := func(pairs ...any) Value {
		o := NewObject()
		for i := 0; i < len(pairs); i += 2 {
			o.Set(FromGoString(pairs[i].(string)), pairs[i+1].(Value))
		}
		return o
	}
	n := func(f float64) Value { return Number(f) }
	s := func(v string) Value { return StringValue(FromGoString(v)) }
	thrower := func(raise func()) Value {
		return NewFunc(func([]Value) Value {
			raise()
			return Undefined
		})
	}
	numbered := func(changed bool) Value {
		o := NewObject()
		for i := 0; i < 20; i++ {
			v := float64(i)
			if changed && i == 19 {
				v = 99
			}
			o.Set(FromGoString("k"+assertKeyIndex(i)), Number(v))
		}
		return o
	}

	switch name {
	case "shared object":
		return shared.object
	case "shared nested object":
		return shared.nestedObject
	case "shared array":
		return shared.array
	case "shared function":
		return shared.function
	case "shared error":
		return shared.err
	case "zero":
		return n(0)
	case "negative zero":
		return n(math.Copysign(0, -1))
	case "one":
		return n(1)
	case "two":
		return n(2)
	case "nan":
		return n(math.NaN())
	case "true":
		return Bool(true)
	case "false":
		return Bool(false)
	case "null":
		return Null
	case "undefined":
		return Undefined
	case "string a":
		return s("a")
	case "string b":
		return s("b")
	case "string abcdefghij":
		return s("abcdefghij")
	case "string abcdefghik":
		return s("abcdefghik")
	case "long number":
		return n(1234567890123)
	case "other long number":
		return n(1234567890124)
	case "empty object":
		return NewObject()
	case "object a1":
		return obj("a", n(1))
	case "object a2":
		return obj("a", n(2))
	case "object ab":
		return obj("a", n(1), "b", n(2))
	case "object abc":
		return obj("a", n(1), "b", n(2), "c", n(3))
	case "object out of order":
		return obj("b", n(2), "a", n(1))
	case "nested object":
		return obj("a", obj("b", obj("c", n(1))))
	case "other nested object":
		return obj("a", obj("b", obj("c", n(2))))
	case "empty array":
		return NewArrayValue(nil)
	case "array 123":
		return NewArrayValue([]Value{n(1), n(2), n(3)})
	case "array 124":
		return NewArrayValue([]Value{n(1), n(2), n(4)})
	case "array 1234":
		return NewArrayValue([]Value{n(1), n(2), n(3), n(4)})
	case "ten line object":
		return obj("a", n(1), "b", n(2), "c", n(3), "d", n(4), "e", n(5), "f", n(6), "g", n(7), "h", n(8))
	case "ten line object changed":
		return obj("a", n(1), "b", n(2), "c", n(3), "d", n(4), "e", n(9), "f", n(6), "g", n(7), "h", n(8))
	case "twenty line object":
		return numbered(false)
	case "twenty line object changed":
		return numbered(true)
	case "error a":
		return NewError(FromGoString("a")).ToValue()
	case "error b":
		return NewError(FromGoString("b")).ToValue()
	case "regexp":
		return RegExpValue(NewRegExpLiteral("a+", ""))
	case "function":
		return WithName(NewFunc(func([]Value) Value { return Undefined }), "foo")
	case "other function":
		return WithName(NewFunc(func([]Value) Value { return Undefined }), "bar")

	// The throwers throws and doesNotThrow are called with. A thrown non-error goes
	// through the same carriers a program's throw of one does, so what the module catches
	// is what it would catch from lowered code.
	case "thrower nothing":
		return NewFunc(func([]Value) Value { return Undefined })
	case "thrower error a":
		return thrower(func() { Throw(NewError(FromGoString("a"))) })
	case "thrower error empty":
		return thrower(func() { Throw(NewError(FromGoString(""))) })
	case "thrower type error bad":
		return thrower(func() { Throw(NewTypeError(FromGoString("bad"))) })
	case "thrower error with code":
		return thrower(func() {
			e := NewError(FromGoString("a"))
			e.SetProperty("code", s("Y"))
			Throw(e)
		})
	case "thrower number":
		return thrower(func() { Throw(NewThrownValue(n(42))) })
	case "thrower string":
		return thrower(func() { Throw(ThrownString(FromGoString("boom"))) })
	case "thrower object":
		return thrower(func() { Throw(NewThrownValue(NewObject())) })

	// The expectations.
	case "ctor Error":
		return ErrorConstructor("Error")
	case "ctor TypeError":
		return ErrorConstructor("TypeError")
	case "ctor RangeError":
		return ErrorConstructor("RangeError")
	case "regexp bad":
		return RegExpValue(NewRegExpLiteral("bad", ""))
	case "regexp error a":
		return RegExpValue(NewRegExpLiteral("^Error: a$", ""))
	case "validation true":
		return NewFunc(func([]Value) Value { return Bool(true) })
	case "validation false":
		return NewFunc(func([]Value) Value { return Bool(false) })
	case "validation one":
		return NewFunc(func([]Value) Value { return n(1) })
	case "validation named nope":
		return WithName(NewFunc(func([]Value) Value { return s("nope") }), "named")
	case "validation message a":
		return NewFunc(func(args []Value) Value {
			return Bool(StrictEquals(Arg(args, 0).Get(FromGoString("message")), s("a")))
		})
	case "expect name":
		return obj("name", s("TypeError"))
	case "expect name and message":
		return obj("name", s("TypeError"), "message", s("other"))
	case "expect message regexp":
		return obj("message", RegExpValue(NewRegExpLiteral("bad", "")))
	case "expect message regexp unmatched":
		return obj("message", RegExpValue(NewRegExpLiteral("nope", "")))
	case "expect code":
		return obj("code", s("X"))
	}
	t.Fatalf("no value named %q: the generator has one and this file does not", name)
	return Undefined
}

// assertKeyIndex is a decimal integer for the generated key names, spelled here so the file does
// not import strconv for one call.
func assertKeyIndex(i int) string {
	if i < 10 {
		return string(rune('0' + i))
	}
	return string(rune('0'+i/10)) + string(rune('0'+i%10))
}

// assertCall runs the named method with the two values, and reports what it threw.
// The methods are reached through the module rather than called directly, so what the
// test exercises is what a program reaches: the callable module, its own properties,
// and the honest-stub proxy around them.
func assertCall(t *testing.T, method string, a, b Value) (e *Error, threw bool) {
	t.Helper()
	mod := RequireBuiltin("assert")
	call := func(name string, args ...Value) {
		mod.Get(FromGoString(name)).Call(args...)
	}
	defer func() {
		if r := recover(); r != nil {
			e, threw = Caught(r), true
		}
	}()
	custom := StringValue(FromGoString("custom"))
	switch method {
	case "ok":
		mod.Call(a)
	case "ok message":
		mod.Call(a, custom)
	case "equal":
		call("equal", a, b)
	case "notEqual":
		call("notEqual", a, b)
	case "strictEqual":
		call("strictEqual", a, b)
	case "strictEqual message":
		call("strictEqual", a, b, custom)
	case "notStrictEqual":
		call("notStrictEqual", a, b)
	case "deepEqual":
		call("deepEqual", a, b)
	case "notDeepEqual":
		call("notDeepEqual", a, b)
	case "deepStrictEqual":
		call("deepStrictEqual", a, b)
	case "deepStrictEqual message":
		call("deepStrictEqual", a, b, custom)
	case "notDeepStrictEqual":
		call("notDeepStrictEqual", a, b)
	case "ifError":
		call("ifError", a)
	case "fail":
		call("fail", a)
	case "fail nothing":
		call("fail")
	case "match":
		call("match", a, b)
	case "doesNotMatch":
		call("doesNotMatch", a, b)
	case "throws":
		call("throws", a, b)
	case "throws message":
		call("throws", a, b, custom)
	case "throws error message":
		call("throws", a, b, NewRangeError(FromGoString("as message")).ToValue())
	case "doesNotThrow":
		call("doesNotThrow", a, b)
	case "doesNotThrow message":
		call("doesNotThrow", a, b, custom)
	default:
		t.Fatalf("no method named %q: the generator has one and this file does not", method)
	}
	return nil, false
}

// TestAssertMessagesMatchNode is the whole comparison: every case Node recorded, run
// through bento's module.
func TestAssertMessagesMatchNode(t *testing.T) {
	raw, err := os.ReadFile("testdata/nodeassert_node24.json")
	if err != nil {
		t.Fatalf("read reference: %v", err)
	}
	var ref map[string]assertRefCase
	if err := json.Unmarshal(raw, &ref); err != nil {
		t.Fatalf("parse reference: %v", err)
	}
	if len(ref) == 0 {
		t.Fatal("reference is empty")
	}

	shared := &assertSharedValues{
		object:       NewObject(),
		nestedObject: NewObject(),
		array:        NewArrayValue([]Value{Number(1), Number(2), Number(3)}),
		function:     WithName(NewFunc(func([]Value) Value { return Undefined }), "foo"),
		err:          NewError(FromGoString("a")).ToValue(),
	}
	shared.object.Set(FromGoString("a"), Number(1))
	inner := NewObject()
	innermost := NewObject()
	innermost.Set(FromGoString("c"), Number(1))
	inner.Set(FromGoString("b"), innermost)
	shared.nestedObject.Set(FromGoString("a"), inner)

	for key, want := range ref {
		method, actualName, expectedName := splitCaseKey(t, key)
		a := assertValue(t, shared, actualName)
		b := assertValue(t, shared, expectedName)

		e, threw := assertCall(t, method, a, b)
		if threw != want.Threw {
			if threw {
				t.Errorf("%s: threw %q, Node did not throw", key, e.ErrorMessage())
			} else {
				t.Errorf("%s: did not throw, Node threw %q", key, want.Message)
			}
			continue
		}
		if !threw {
			continue
		}

		wantMessage := want.Message
		if divergent, ok := assertKnownDivergences[key]; ok {
			wantMessage = divergent
		}
		if got := e.ErrorMessage(); got != wantMessage {
			t.Errorf("%s message:\ngot:\n%s\nwant:\n%s", key, got, wantMessage)
		}
		if got := e.ErrorName(); got != want.Name {
			t.Errorf("%s name = %q, want %q", key, got, want.Name)
		}
		if code, _ := e.Code(); code.ToGoString() != want.Code {
			t.Errorf("%s code = %q, want %q", key, code.ToGoString(), want.Code)
		}
		boxed := e.ToValue()
		if got := ToBoolean(boxed.Get(FromGoString("generatedMessage"))); got != want.GeneratedMessage {
			t.Errorf("%s generatedMessage = %v, want %v", key, got, want.GeneratedMessage)
		}
		if got := ToString(boxed.Get(FromGoString("operator"))).ToGoString(); got != want.Operator {
			t.Errorf("%s operator = %q, want %q", key, got, want.Operator)
		}
	}
}

// splitCaseKey reads a case key back into its three names, the "method(actual,
// expected)" the generator spelled it as.
func splitCaseKey(t *testing.T, key string) (method, actual, expected string) {
	t.Helper()
	open := -1
	for i := 0; i < len(key); i++ {
		if key[i] == '(' {
			open = i
			break
		}
	}
	if open == -1 || key[len(key)-1] != ')' {
		t.Fatalf("case key %q is not method(actual, expected)", key)
	}
	args := key[open+1 : len(key)-1]
	sep := -1
	for i := 0; i+1 < len(args); i++ {
		if args[i] == ',' && args[i+1] == ' ' {
			sep = i
			break
		}
	}
	if sep == -1 {
		t.Fatalf("case key %q has no argument separator", key)
	}
	return key[:open], args[:sep], args[sep+2:]
}

// TestAssertModuleShape pins the identities Node's module has, which a program relies
// on more than it looks: a test that destructures { ok } from the module gets a
// callable, and one that requires assert/strict gets the very object assert.strict is.
func TestAssertModuleShape(t *testing.T) {
	plain := RequireBuiltin("assert")
	strict := RequireBuiltin("assert/strict")

	if !StrictEquals(plain, RequireBuiltin("node:assert")) {
		t.Error("require('assert') and require('node:assert') are different values")
	}
	if !StrictEquals(strict, RequireBuiltin("node:assert/strict")) {
		t.Error("require('assert/strict') and require('node:assert/strict') are different values")
	}
	if !StrictEquals(plain.Get(FromGoString("strict")), strict) {
		t.Error("assert.strict is not require('assert/strict')")
	}
	if !StrictEquals(strict.Get(FromGoString("strict")), strict) {
		t.Error("assert.strict.strict is not assert.strict")
	}
	if !StrictEquals(plain.Get(FromGoString("ok")), plain) {
		t.Error("assert.ok is not assert")
	}
	if !StrictEquals(strict.Get(FromGoString("ok")), plain) {
		t.Error("assert.strict.ok is not assert")
	}
	if !StrictEquals(strict.Get(FromGoString("equal")), plain.Get(FromGoString("strictEqual"))) {
		t.Error("assert.strict.equal is not assert.strictEqual")
	}
	if !StrictEquals(strict.Get(FromGoString("deepEqual")), plain.Get(FromGoString("deepStrictEqual"))) {
		t.Error("assert.strict.deepEqual is not assert.deepStrictEqual")
	}
	if plain.Kind() != KindFunc {
		t.Errorf("the assert module is %v, not a function", plain.Kind())
	}
}

// TestAssertStrictModuleUsesStrictComparisons checks the one behavioral difference
// between the two modules: the loose names compare strictly.
func TestAssertStrictModuleUsesStrictComparisons(t *testing.T) {
	strict := RequireBuiltin("assert/strict")
	cases := []struct {
		method string
		a, b   Value
	}{
		{"equal", Number(1), StringValue(FromGoString("1"))},
		{"deepEqual", Number(0), Bool(false)},
	}
	for _, c := range cases {
		threw := func() (threw bool) {
			defer func() {
				if recover() != nil {
					threw = true
				}
			}()
			strict.Get(FromGoString(c.method)).Call(c.a, c.b)
			return false
		}()
		if !threw {
			t.Errorf("assert.strict.%s(%s, %s) passed, want a failure", c.method,
				NodeInspect(c.a).ToGoString(), NodeInspect(c.b).ToGoString())
		}
	}
}

// TestAssertUnimplementedMembersThrow keeps the honest-stub rule over a module that is
// mostly implemented: a member this port does not carry says so rather than answering
// undefined, which a program would only find out about a few lines later.
func TestAssertUnimplementedMembersThrow(t *testing.T) {
	for _, member := range []string{"rejects", "doesNotReject", "partialDeepStrictEqual", "AssertionError", "Assert", "CallTracker"} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("assert.%s answered instead of throwing", member)
					return
				}
				msg := Caught(r).ErrorMessage()
				want := "The built-in module 'assert' is registered but not implemented in bento yet (reading '" + member + "')"
				if msg != want {
					t.Errorf("assert.%s threw %q, want %q", member, msg, want)
				}
			}()
			RequireBuiltin("assert").Get(FromGoString(member))
		}()
	}
}

// TestAssertThrowsErrorClassAgainstNonError pins the one throws case that is not in the
// reference, because Node's answer to it is a stack from the machine that ran it.
//
// assert.throws(fn, Error) where fn threw something that is not an error: Node decides
// an expectation is an error class with Error.isPrototypeOf(expected), which is false for
// Error itself, so it falls through to the validation-function branch, calls Error as a
// predicate, and reports the error that call returned, stack included. Every other class
// (TypeError, RangeError) takes the class branch and says what it expected. bento models
// a class as a name rather than a prototype and has no such gap, so Error reads as the
// class the caller wrote, which is the message Node gives for every other class.
func TestAssertThrowsErrorClassAgainstNonError(t *testing.T) {
	e, threw := assertCall(t, "throws",
		NewFunc(func([]Value) Value {
			Throw(NewThrownValue(NewObject()))
			return Undefined
		}), ErrorConstructor("Error"))
	if !threw {
		t.Fatal("assert.throws(thrower, Error) passed for a thrown object")
	}
	want := `The error is expected to be an instance of "Error". Received "{}"`
	if got := e.ErrorMessage(); got != want {
		t.Errorf("message:\ngot:\n%s\nwant:\n%s", got, want)
	}
}

// TestAssertThrowsRethrowsIdentity pins that doesNotThrow rethrows the error it caught
// rather than a copy of it. A test that lets an unexpected error through and catches it
// higher up compares it by identity, and an assertion library that rebuilt it would
// break that.
func TestAssertThrowsRethrowsIdentity(t *testing.T) {
	raised := NewError(FromGoString("a"))
	e, threw := assertCall(t, "doesNotThrow",
		NewFunc(func([]Value) Value {
			Throw(raised)
			return Undefined
		}), ErrorConstructor("TypeError"))
	if !threw {
		t.Fatal("assert.doesNotThrow rethrew nothing")
	}
	if e != raised {
		t.Errorf("rethrew %v, want the error the function raised", e)
	}
}
