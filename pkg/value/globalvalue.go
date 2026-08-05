package value

import (
	"math"
	"sort"
)

// This file is the value-model side of an ambient global named rather than called:
// atob passed to a helper, Symbol compared for identity, URL read off globalThis.
// A global is a first-class object in JavaScript, so the dynamic world needs a value
// for it that reports typeof "function", answers .name, compares equal to itself
// wherever the program reaches it, and calls the same way the bare name calls.
//
// Node's own suite reaches for these constantly, and almost always for identity
// rather than behavior:
//
//	assert.strictEqual(globalThis.atob, buffer.atob)
//	assert.notStrictEqual(sandbox.Symbol, Symbol)
//	assert.strictEqual(Object.getOwnPropertyDescriptor(globalThis, 'URL').value, URL)
//
// The family is modeled the way the built-in error constructors are (errorctor.go):
// one interned value per name, built once at package initialization and never
// written after, so two references to Symbol are the same object pointer and
// globalThis.URL is the URL the bare name reads.
//
// What is here is what bento hosts. A global whose behavior the runtime has not
// built (crypto, WebSocket, ReadableStream) is deliberately absent, so the lowerer
// goes on refusing it at compile time rather than handing the program a value that
// answers undefined for everything it asks. That is the same rule the global object
// follows (globalthis.go), and this table is what both read.
//
// The ceiling is statics. A hosted global carries its name and its call and nothing
// else, so an alias reads no further: `const O = Object; O.keys(x)` finds no keys on
// the value and fails at run time where the direct `Object.keys(x)` lowers to a
// helper. The direct forms are what a program writes and what the lowerer claims
// before a receiver would ever box, so the gap is narrow; it is the same one the
// error constructors have carried since they were modeled, and it closes when the
// statics move onto the value.

// hostedGlobals interns one value per ambient global bento hosts. It is built once
// at package initialization and never written after, so the concurrent reads a
// running program makes need no lock, and the identity a program compares is the
// identity every reference to the name reaches.
var hostedGlobals = buildHostedGlobals()

func buildHostedGlobals() map[string]Value {
	calls := hostedGlobalCalls()
	m := make(map[string]Value, len(calls))
	for name, call := range calls {
		m[name] = WithName(NewFunc(call), name)
	}
	return m
}

// GlobalValue returns the value form of an ambient global, the lowering of naming
// one rather than calling it. The name is one the lowerer only emits after asking
// HostsGlobal, so an unknown name here is a bug in that pairing rather than
// something a program can reach; it answers undefined rather than panicking, since
// a compiled program crashing inside its own prelude is the worse of the two.
func GlobalValue(name string) Value {
	return hostedGlobals[name]
}

// HostsGlobal reports whether bento has a value form for an ambient global. The
// lowerer asks before it emits GlobalValue, so the set of names the compiler will
// hand a program and the set the runtime can answer for are one list rather than
// two that drift.
func HostsGlobal(name string) bool {
	_, ok := hostedGlobals[name]
	return ok
}

// HostedGlobalNames returns the hosted names sorted, for the global object to
// install and for a test to walk. Sorted rather than in map order because the
// install order is the order the names sit on the global object, and a program
// reading them back through getOwnPropertyNames should see the same list on every
// run of the same binary.
func HostedGlobalNames() []string {
	out := make([]string, 0, len(hostedGlobals))
	for name := range hostedGlobals {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// hostedGlobalCalls is the table: one entry per hosted global, holding what the
// name does when it is called. Every entry routes to the same runtime the direct
// call lowers to, so atob reached through a value and atob called by name are one
// implementation.
//
// A constructor that JavaScript refuses to run without new answers that refusal
// rather than doing something approximate, which is what Node does and what a
// program testing the refusal expects to catch.
func hostedGlobalCalls() map[string]callFn {
	m := map[string]callFn{
		// The codec globals take one string and answer one string.
		"atob":               func(a []Value) Value { return StringValue(Atob(ToString(Arg(a, 0)))) },
		"btoa":               func(a []Value) Value { return StringValue(Btoa(ToString(Arg(a, 0)))) },
		"encodeURI":          func(a []Value) Value { return StringValue(EncodeURI(ToString(Arg(a, 0)))) },
		"decodeURI":          func(a []Value) Value { return StringValue(DecodeURI(ToString(Arg(a, 0)))) },
		"encodeURIComponent": func(a []Value) Value { return StringValue(EncodeURIComponent(ToString(Arg(a, 0)))) },
		"decodeURIComponent": func(a []Value) Value { return StringValue(DecodeURIComponent(ToString(Arg(a, 0)))) },

		// The number globals read a number out of whatever they are handed. parseInt's
		// radix is read through ToNumber, so an omitted one arrives as NaN and the
		// runtime's own default (base 10, or 16 behind an 0x prefix) applies.
		"parseInt":   func(a []Value) Value { return Number(ParseInt(ToString(Arg(a, 0)), ToNumber(Arg(a, 1)))) },
		"parseFloat": func(a []Value) Value { return Number(ParseFloat(ToString(Arg(a, 0)))) },
		"isNaN":      func(a []Value) Value { return Bool(math.IsNaN(ToNumber(Arg(a, 0)))) },
		"isFinite": func(a []Value) Value {
			return Bool(!math.IsNaN(ToNumber(Arg(a, 0))) && !math.IsInf(ToNumber(Arg(a, 0)), 0))
		},

		// The scheduling globals hand a callback to the event loop and answer the handle
		// that cancels it. A handle is a number here, which is what the ambient
		// declarations say and what clearTimeout reads back.
		"queueMicrotask":  func(a []Value) Value { QueueMicrotask(Arg(a, 0)); return Undefined },
		"structuredClone": func(a []Value) Value { return StructuredClone(Arg(a, 0)) },
		"setTimeout":      func(a []Value) Value { return Number(SetTimeout(Arg(a, 0), Arg(a, 1), restArgs(a, 2)...)) },
		"setInterval":     func(a []Value) Value { return Number(SetInterval(Arg(a, 0), Arg(a, 1), restArgs(a, 2)...)) },
		"setImmediate":    func(a []Value) Value { return Number(SetImmediate(Arg(a, 0), restArgs(a, 1)...)) },
		"clearTimeout":    func(a []Value) Value { ClearTimer(Arg(a, 0)); return Undefined },
		"clearInterval":   func(a []Value) Value { ClearTimer(Arg(a, 0)); return Undefined },
		"clearImmediate":  func(a []Value) Value { ClearTimer(Arg(a, 0)); return Undefined },

		// The constructors JavaScript also lets a program call without new. Each one
		// coerces rather than constructs, which is the whole of what the call form does.
		"Boolean": func(a []Value) Value { return Bool(ToBoolean(Arg(a, 0))) },
		"Number":  numberGlobalCall,
		"String":  stringGlobalCall,
		"Symbol":  symbolGlobalCall,
		"Object":  func(a []Value) Value { return ObjectCoerce(Arg(a, 0)) },
		"Array":   arrayGlobalCall,
		// Date called without new answers the current time as a string, ignoring every
		// argument, which is the one place in the language where the call and the
		// construction do unrelated things.
		"Date": func([]Value) Value { return StringValue(NewDate().ToString()) },
	}
	// The constructors that have no call form at all. Calling one is a TypeError in
	// JavaScript rather than a construction, so the value says exactly that instead of
	// quietly constructing what the program did not ask for.
	for _, name := range []string{
		"Map", "Set", "WeakMap", "WeakSet", "Promise", "Proxy",
		"URL", "URLSearchParams", "TextEncoder", "TextDecoder",
	} {
		m[name] = requiresNewCall(name)
	}
	return m
}

// requiresNewCall builds the call body of a constructor that cannot be called: it
// throws the TypeError the language throws, naming the constructor the way V8 names
// it, so a program catching the refusal reads the message it expects.
func requiresNewCall(name string) callFn {
	return func([]Value) Value {
		Throw(NewTypeError(FromGoString("Constructor " + name + " requires 'new'")))
		return Undefined
	}
}

// restArgs returns the arguments from i on, the extra arguments a timer forwards to
// its callback. A call with fewer arguments than i has none to forward, which is the
// ordinary case, so the nil slice is the answer rather than a bounds failure.
func restArgs(args []Value, i int) []Value {
	if len(args) <= i {
		return nil
	}
	return args[i:]
}

// numberGlobalCall is Number(x): the numeric coercion, and zero for no argument at
// all, which is the one case ToNumber cannot answer since undefined coerces to NaN.
func numberGlobalCall(args []Value) Value {
	if len(args) == 0 {
		return Number(0)
	}
	return Number(ToNumber(args[0]))
}

// stringGlobalCall is String(x): the string coercion, and the empty string for no
// argument, which is again the case the coercion of undefined would get wrong.
func stringGlobalCall(args []Value) Value {
	if len(args) == 0 {
		return StringValue(FromGoString(""))
	}
	return StringValue(ToString(args[0]))
}

// symbolGlobalCall is Symbol(desc): a fresh unique symbol every call, described by
// the argument coerced to a string. An absent description is not the empty string,
// it is no description, which is what a logged symbol renders as Symbol() rather
// than Symbol().
func symbolGlobalCall(args []Value) Value {
	if len(args) == 0 || args[0].IsUndefined() {
		return NewSymbolNoDesc()
	}
	return NewSymbol(ToString(args[0]))
}

// arrayGlobalCall is Array(...): one numeric argument is a length and every other
// shape is the elements themselves, the rule that makes Array(3) three holes and
// Array('3') a one-element array. A length that is not a whole number in range is
// a RangeError, since there is no array of that many elements to build.
func arrayGlobalCall(args []Value) Value {
	if len(args) == 1 && args[0].Kind() == KindNumber {
		n := ToNumber(args[0])
		if n != math.Trunc(n) || n < 0 || n > math.MaxUint32-1 {
			Throw(NewRangeError(FromGoString("Invalid array length")))
			return Undefined
		}
		elems := make([]Value, int(n))
		for i := range elems {
			elems[i] = hole
		}
		return NewArrayValue(elems)
	}
	return NewArrayValue(append([]Value(nil), args...))
}

// ObjectCoerce is Object(x): the coercion that answers the object form of a value.
// An object is already one and comes back unchanged, which is what makes
// Object(process.config) === process.config hold, and null or undefined answer a
// fresh empty object.
//
// A primitive has no answer here. The object form of one is a wrapper object, a
// Number or a String holding a primitive inside it, and bento does not model those:
// there is no value in this package a program could get back that would behave like
// one. Throwing names that gap rather than handing back something that is not what
// was asked for.
func ObjectCoerce(v Value) Value {
	switch v.Kind() {
	case KindObject, KindArray, KindFunc:
		return v
	case KindUndefined, KindNull:
		return NewObject()
	}
	Throw(NewTypeError(FromGoString("Object() of a primitive needs a wrapper object, which bento does not model yet")))
	return Undefined
}
