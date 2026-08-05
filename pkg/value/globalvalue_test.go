package value

import (
	"strings"
	"testing"
)

// An ambient global named rather than called is a value, and these pin what that
// value is: one object per name for the life of the run, reporting itself as a
// function, carrying its own name, and calling what the bare name calls.

// TestGlobalValueIsOneValuePerName pins the identity the whole family rests on. A
// program comparing two references to a global, or comparing one against what it
// read off globalThis, is asking whether they are the same object, and they are.
func TestGlobalValueIsOneValuePerName(t *testing.T) {
	if !StrictEquals(GlobalValue("Symbol"), GlobalValue("Symbol")) {
		t.Error("two reads of Symbol answered different values")
	}
	if StrictEquals(GlobalValue("Symbol"), GlobalValue("Object")) {
		t.Error("Symbol and Object answered the same value")
	}
}

// TestGlobalValueLooksLikeAFunction pins the two things a program reads off a global
// before it does anything else with it: what typeof says, and what .name says.
func TestGlobalValueLooksLikeAFunction(t *testing.T) {
	for _, name := range []string{"atob", "setImmediate", "Symbol", "URL"} {
		v := GlobalValue(name)
		if got := v.TypeOf().ToGoString(); got != "function" {
			t.Errorf("typeof %s is %q, want \"function\"", name, got)
		}
		if got := ToString(v.Get(FromGoString("name"))).ToGoString(); got != name {
			t.Errorf("%s.name is %q, want %q", name, got, name)
		}
	}
}

// TestGlobalValueNameIsNotEnumerable pins that a global's name rides the same
// non-enumerable slot a function's name does, so walking a global's own keys finds
// nothing rather than an entry the language does not have there.
func TestGlobalValueNameIsNotEnumerable(t *testing.T) {
	if keys := GlobalValue("atob").OwnEnumerableKeys(); keys.Len() != 0 {
		t.Errorf("atob has %v enumerable own keys, want none", keys.Len())
	}
}

// TestGlobalValueCallsTheSameRuntime pins that reaching a global through its value
// runs what calling it by name runs, since the two forms are one function in the
// language. base64 round-trips through both codec globals.
func TestGlobalValueCallsTheSameRuntime(t *testing.T) {
	encoded := GlobalValue("btoa").Call(StringValue(FromGoString("bento")))
	if got := ToString(encoded).ToGoString(); got != "YmVudG8=" {
		t.Fatalf("btoa answered %q, want \"YmVudG8=\"", got)
	}
	decoded := GlobalValue("atob").Call(encoded)
	if got := ToString(decoded).ToGoString(); got != "bento" {
		t.Fatalf("atob answered %q, want \"bento\"", got)
	}
}

// TestGlobalValueSymbolCallIsFresh pins that Symbol through the value is the
// constructor and not a cached symbol: every call is a new unique symbol, which is
// the whole point of the name.
func TestGlobalValueSymbolCallIsFresh(t *testing.T) {
	a := GlobalValue("Symbol").Call(StringValue(FromGoString("tag")))
	b := GlobalValue("Symbol").Call(StringValue(FromGoString("tag")))
	if a.Kind() != KindSymbol {
		t.Fatalf("Symbol('tag') answered kind %v, want a symbol", a.Kind())
	}
	if StrictEquals(a, b) {
		t.Error("two Symbol('tag') calls answered the same symbol")
	}
}

// TestGlobalValueCoercionCalls pins the constructors a program is allowed to call
// without new. Each coerces rather than constructs, which is all the call form does.
func TestGlobalValueCoercionCalls(t *testing.T) {
	if got := ToString(GlobalValue("String").Call(Number(12))).ToGoString(); got != "12" {
		t.Errorf("String(12) answered %q, want \"12\"", got)
	}
	if got := ToNumber(GlobalValue("Number").Call(StringValue(FromGoString("2.5")))); got != 2.5 {
		t.Errorf("Number(\"2.5\") answered %v, want 2.5", got)
	}
	if got := ToBoolean(GlobalValue("Boolean").Call(StringValue(FromGoString("")))); got {
		t.Error("Boolean(\"\") answered true, want false")
	}
	if got := ToNumber(GlobalValue("Number").Call()); got != 0 {
		t.Errorf("Number() answered %v, want 0", got)
	}
	if got := ToString(GlobalValue("String").Call()).ToGoString(); got != "" {
		t.Errorf("String() answered %q, want the empty string", got)
	}
}

// TestGlobalValueArrayCall pins Array's two readings of its arguments: one number is
// a length and everything else is the elements themselves.
func TestGlobalValueArrayCall(t *testing.T) {
	byLength := GlobalValue("Array").Call(Number(3))
	if got := ToNumber(byLength.Get(FromGoString("length"))); got != 3 {
		t.Errorf("Array(3) has length %v, want 3", got)
	}
	if got := byLength.GetIndex(0); !got.IsUndefined() {
		t.Errorf("Array(3)[0] answered %v, want undefined", ToString(got).ToGoString())
	}
	byElems := GlobalValue("Array").Call(StringValue(FromGoString("3")))
	if got := ToNumber(byElems.Get(FromGoString("length"))); got != 1 {
		t.Errorf("Array(\"3\") has length %v, want 1", got)
	}
}

// TestGlobalValueRequiresNew pins the constructors that have no call form. Calling
// one is a TypeError in the language, so the value throws it rather than quietly
// constructing what the program did not ask for.
func TestGlobalValueRequiresNew(t *testing.T) {
	for _, name := range []string{"Map", "Set", "Promise", "URL", "TextDecoder"} {
		func() {
			defer func() {
				r := recover()
				if r == nil {
					t.Errorf("calling %s did not throw", name)
					return
				}
				thrown, ok := r.(Thrown)
				if !ok {
					t.Errorf("calling %s threw %T, want a thrown value", name, r)
					return
				}
				msg := ToString(Caught(thrown).ToValue()).ToGoString()
				if !strings.Contains(msg, "Constructor "+name+" requires 'new'") {
					t.Errorf("calling %s threw %q, want the requires-new TypeError", name, msg)
				}
			}()
			GlobalValue(name).Call()
		}()
	}
}

// TestObjectCoerceKeepsAnObject pins the case Object(x) is written for: an object is
// already one, so it comes back unchanged and compares equal to what went in.
func TestObjectCoerceKeepsAnObject(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))
	if !StrictEquals(ObjectCoerce(o), o) {
		t.Error("Object(o) answered a different object")
	}
	fresh := ObjectCoerce(Undefined)
	if fresh.Kind() != KindObject {
		t.Errorf("Object(undefined) answered kind %v, want an object", fresh.Kind())
	}
	if StrictEquals(fresh, ObjectCoerce(Null)) {
		t.Error("Object(undefined) and Object(null) answered the same object, want two")
	}
}

// TestObjectCoerceRefusesAPrimitive pins the gap rather than papering over it: the
// object form of a primitive is a wrapper object, and there is no value in this
// package that behaves like one, so the coercion says so.
func TestObjectCoerceRefusesAPrimitive(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("Object(5) did not throw")
		}
	}()
	ObjectCoerce(Number(5))
}

// TestHostsGlobalCoversOnlyWhatIsBuilt pins the rule the whole slice rests on: a
// global is hosted when the runtime has built it, and a global bento has not built
// stays unhosted so the lowerer goes on refusing it at compile time rather than
// handing a program a value that answers undefined for everything.
func TestHostsGlobalCoversOnlyWhatIsBuilt(t *testing.T) {
	for _, name := range []string{"atob", "setImmediate", "Symbol", "Object", "URL", "Proxy"} {
		if !HostsGlobal(name) {
			t.Errorf("%s is not hosted, want it hosted", name)
		}
	}
	for _, name := range []string{"crypto", "Crypto", "WebSocket", "ReadableStream", "eval", "Math", "JSON"} {
		if HostsGlobal(name) {
			t.Errorf("%s is hosted, want it left to the compile-time refusal", name)
		}
	}
}

// TestGlobalThisHoldsTheHostedGlobals pins that reaching a global through the global
// object reaches the same value the bare name does, which is what makes
// globalThis.atob === atob hold the way globalThis.process === process does.
func TestGlobalThisHoldsTheHostedGlobals(t *testing.T) {
	g := GlobalThisValue()
	for _, name := range HostedGlobalNames() {
		if !StrictEquals(g.Get(FromGoString(name)), GlobalValue(name)) {
			t.Errorf("globalThis.%s is not the value the bare name reads", name)
		}
	}
}
