package value

import "testing"

// Every ordinary object inherits Object.prototype, and until this the value model
// carried only half of it: the reflection methods answered a read, the two coercion
// methods did not, and the existence probe the in operator makes answered none of them.
// These hold the modeled default prototype: what an ordinary object inherits, what an
// object made with a null prototype inherits instead (nothing), and where a nearer
// prototype's method wins. Every expectation is Node v24's.

// TestDefaultPrototypeAnswersItsCoercions is the capability. toString and valueOf are
// the two members the language calls on its own during a coercion, so an object with no
// coercion of its own has to find them on the prototype the way every other member is
// found, rather than have the coercion invent an answer further down.
func TestDefaultPrototypeAnswersItsCoercions(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("a"), Number(1))

	toString := o.Get(FromGoString("toString"))
	if toString.kind != KindFunc {
		t.Fatalf("o.toString did not resolve to a function: kind %v", toString.kind)
	}
	if got := ToString(toString.Call()).ToGoString(); got != "[object Object]" {
		t.Errorf("o.toString() = %q, want \"[object Object]\"", got)
	}

	valueOf := o.Get(FromGoString("valueOf"))
	if valueOf.kind != KindFunc {
		t.Fatalf("o.valueOf did not resolve to a function: kind %v", valueOf.kind)
	}
	if got := valueOf.Call(); !StrictEquals(got, o) {
		t.Error("o.valueOf() did not answer the object itself")
	}
	if got := ToString(o).ToGoString(); got != "[object Object]" {
		t.Errorf("String(o) = %q, want \"[object Object]\"", got)
	}
}

// TestDefaultPrototypeAnswersTheInOperator is the other half. The read and the
// existence probe have to name the same set, or `'toString' in o` reads false for a
// name o.toString hands back a method for, which is the pair the language keeps
// together and the reason the lowerer used to decline the test outright.
func TestDefaultPrototypeAnswersTheInOperator(t *testing.T) {
	o := NewObject()
	present := []string{
		"toString", "valueOf", "toLocaleString", "hasOwnProperty", "isPrototypeOf",
		"propertyIsEnumerable", "__lookupGetter__", "__lookupSetter__",
		"__defineGetter__", "__defineSetter__",
	}
	for _, name := range present {
		if !o.HasProperty(FromGoString(name)) {
			t.Errorf("'%s' in o = false, want true", name)
		}
	}
	if o.HasProperty(FromGoString("nope")) {
		t.Error("'nope' in o = true, want false")
	}
}

// TestANullPrototypeInheritsNothing is the gate. bento leaves a nil [[Prototype]] slot
// for both an ordinary object and an Object.create(null), so the fallback that supplies
// the inherited methods had been running for both and handed a prototype-less object
// the very methods it was made to be without. The flag on the last object of the chain
// is what tells the two apart, and the walk runs to the end of the chain so an object
// standing over a prototype-less one inherits nothing either.
func TestANullPrototypeInheritsNothing(t *testing.T) {
	bare := ObjectCreate(Null)
	for _, name := range []string{"toString", "valueOf", "hasOwnProperty"} {
		if got := bare.Get(FromGoString(name)); !got.IsUndefined() {
			t.Errorf("Object.create(null).%s = %v, want undefined", name, got.Kind())
		}
		if bare.HasProperty(FromGoString(name)) {
			t.Errorf("'%s' in Object.create(null) = true, want false", name)
		}
	}

	// A chain standing on a prototype-less object ends at the same explicit null, so it
	// inherits nothing however many links are between.
	over := ObjectCreate(bare)
	if got := over.Get(FromGoString("hasOwnProperty")); !got.IsUndefined() {
		t.Errorf("an object over a null-prototype object read hasOwnProperty = %v, want undefined", got.Kind())
	}

	// With neither toString nor valueOf to reach, the string coercion runs out of rungs
	// and throws, which is what the engine does for String(Object.create(null)).
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("String(Object.create(null)) did not throw")
		}
		if got := ToString(Caught(r).ToValue()).ToGoString(); got != "TypeError: Cannot convert object to primitive value" {
			t.Errorf("String(Object.create(null)) threw %q, want a TypeError about converting an object", got)
		}
	}()
	_ = ToString(bare)
}

// TestAPrototypeSetToNullStopsInheriting covers the same gate reached the other way,
// through Object.setPrototypeOf, which is the path that flips the flag on an object
// that already existed rather than setting it as the object is made.
func TestAPrototypeSetToNullStopsInheriting(t *testing.T) {
	o := NewObject()
	if !o.HasProperty(FromGoString("toString")) {
		t.Fatal("a fresh object did not inherit toString")
	}
	o.SetPrototype(Null)
	if got := o.Get(FromGoString("toString")); !got.IsUndefined() {
		t.Errorf("after setPrototypeOf(o, null), o.toString = %v, want undefined", got.Kind())
	}
	if o.HasProperty(FromGoString("toString")) {
		t.Error("after setPrototypeOf(o, null), 'toString' in o = true, want false")
	}
}

// TestANearerPrototypeWinsOverTheDefault holds the lookup order. Object.prototype is the
// last stop, so a name a nearer prototype writes has to win: an array's own
// Array.prototype.toString joins its elements where the default would report a tag, and
// a boxed error and regexp each spell themselves because their prototypes write a
// toString bento keeps as a brand rather than as a property.
func TestANearerPrototypeWinsOverTheDefault(t *testing.T) {
	arr := NewArrayValue([]Value{Number(1), Number(2)})
	if got := ToString(arr.Get(FromGoString("toString")).Call()).ToGoString(); got != "1,2" {
		t.Errorf("[1,2].toString() = %q, want \"1,2\"", got)
	}
	if !arr.HasProperty(FromGoString("map")) {
		t.Error("'map' in [1,2] = false, want true")
	}
	if !arr.HasProperty(FromGoString("hasOwnProperty")) {
		t.Error("'hasOwnProperty' in [1,2] = false, want true: an array reaches Object.prototype past Array.prototype")
	}

	err := NewTypeError(FromGoString("bad")).ToValue()
	if got := ToString(err.Get(FromGoString("toString")).Call()).ToGoString(); got != "TypeError: bad" {
		t.Errorf("err.toString() = %q, want \"TypeError: bad\"", got)
	}

	re := RegExpValue(NewRegExpLiteral("ab+", "g"))
	if got := ToString(re.Get(FromGoString("toString")).Call()).ToGoString(); got != "/ab+/g" {
		t.Errorf("re.toString() = %q, want \"/ab+/g\"", got)
	}
}

// TestAnOwnPropertyStillShadowsTheDefault keeps the new members under the same
// own-before-inherited rule the reflection ones already followed, so a program that
// writes its own toString on an object reads its own back.
func TestAnOwnPropertyStillShadowsTheDefault(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("toString"), NewFunc(func([]Value) Value { return StringValue(FromGoString("mine")) }))
	if got := ToString(o).ToGoString(); got != "mine" {
		t.Errorf("String(o) = %q, want \"mine\"", got)
	}

	proto := NewObject()
	proto.Set(FromGoString("valueOf"), NewFunc(func([]Value) Value { return Number(3) }))
	child := ObjectCreate(proto)
	if got := ToNumber(child); got != 3 {
		t.Errorf("Number(child) = %v, want 3: a nearer prototype's valueOf wins", got)
	}
}

// TestLegacyDefineAccessorInstallsOne covers the two members the prototype was missing
// altogether. They install an accessor between them, each replacing only the half it
// names, and unlike a { get } handed to Object.defineProperty the property they make is
// enumerable and configurable, which is why JSON.stringify sees it.
func TestLegacyDefineAccessorInstallsOne(t *testing.T) {
	o := NewObject()
	o.Get(FromGoString("__defineGetter__")).Call(
		StringValue(FromGoString("x")),
		NewFunc(func([]Value) Value { return Number(42) }),
	)
	if got := ToNumber(o.Get(FromGoString("x"))); got != 42 {
		t.Errorf("o.x = %v, want 42", got)
	}
	if got := JSONStringify(o).ToGoString(); got != `{"x":42}` {
		t.Errorf("JSON.stringify(o) = %q, want {\"x\":42}", got)
	}
	if got := o.Get(FromGoString("__lookupGetter__")).Call(StringValue(FromGoString("x"))); got.Kind() != KindFunc {
		t.Errorf("o.__lookupGetter__('x') = %v, want a function", got.Kind())
	}

	// The setter half lands on the same property rather than replacing it, so the getter
	// installed above still reads.
	var wrote float64
	o.Get(FromGoString("__defineSetter__")).Call(
		StringValue(FromGoString("x")),
		NewFunc(func(args []Value) Value { wrote = ToNumber(Arg(args, 0)); return Undefined }),
	)
	o.Set(FromGoString("x"), Number(7))
	if wrote != 7 {
		t.Errorf("the setter saw %v, want 7", wrote)
	}
	if got := ToNumber(o.Get(FromGoString("x"))); got != 42 {
		t.Errorf("after __defineSetter__, o.x = %v, want 42: the getter half was kept", got)
	}

	// A non-callable second argument is the one way the call fails, and it names the
	// helper in the message the way the engine does.
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("__defineGetter__ with a non-function did not throw")
		}
		want := "TypeError: Object.prototype.__defineGetter__: Expecting function"
		if got := ToString(Caught(r).ToValue()).ToGoString(); got != want {
			t.Errorf("__defineGetter__(k, 1) threw %q, want %q", got, want)
		}
	}()
	o.Get(FromGoString("__defineGetter__")).Call(StringValue(FromGoString("y")), Number(1))
}
