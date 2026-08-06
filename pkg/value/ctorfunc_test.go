package value

import "testing"

// thrownName runs fn and answers the name of the error it threw, so a test that pins a
// TypeError says so in one line. It fails the test when fn completes without throwing,
// since a missing throw is the failure those tests are guarding against.
func thrownName(t *testing.T, fn func()) (name string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want a throw, got a normal return")
		}
		name = Caught(r).ToValue().Get(FromGoString("name")).AsString().ToGoString()
	}()
	fn()
	return ""
}

// TestNewCtorCarriesItsFunctionSurface pins the three own properties a constructor
// function is born with. prototype is a real object, name and length report what the
// declaration said, and none of the three is enumerable, so Object.keys of a
// constructor is empty the way it is in JavaScript.
func TestNewCtorCarriesItsFunctionSurface(t *testing.T) {
	f := NewCtor("Shape", 2, func(this Value, args []Value) Value { return Undefined })
	if got := f.TypeOf().ToGoString(); got != "function" {
		t.Fatalf("want typeof function, got %q", got)
	}
	if got := f.Get(FromGoString("name")).AsString().ToGoString(); got != "Shape" {
		t.Fatalf("want name Shape, got %q", got)
	}
	if got := ToNumber(f.Get(FromGoString("length"))); got != 2 {
		t.Fatalf("want length 2, got %v", got)
	}
	proto := f.Get(FromGoString("prototype"))
	if proto.kind != KindObject {
		t.Fatalf("want a prototype object, got kind %v", proto.kind)
	}
	if !StrictEquals(proto.Get(FromGoString("constructor")), f) {
		t.Fatal("want prototype.constructor to point back at the constructor")
	}
	if n := f.OwnEnumerableKeys().Len(); n != 0 {
		t.Fatalf("want no enumerable own keys on a constructor, got %v", n)
	}
}

// TestConstructBindsTheNewObject pins the core of [[Construct]]: the body runs with the
// fresh object as its receiver, that object links to the constructor's prototype, and a
// body that returns nothing yields the object rather than the undefined it returned.
func TestConstructBindsTheNewObject(t *testing.T) {
	f := NewCtor("Tagged", 1, func(this Value, args []Value) Value {
		this.Set(FromGoString("tag"), Arg(args, 0))
		return Undefined
	})
	v := Construct(f, StringValue(FromGoString("x")))
	if got := v.Get(FromGoString("tag")).AsString().ToGoString(); got != "x" {
		t.Fatalf("want the body to have written onto the new object, got %q", got)
	}
	if !StrictEquals(v.ProtoRead(), f.Get(FromGoString("prototype"))) {
		t.Fatal("want the instance linked to the constructor's prototype")
	}
}

// TestConstructReadsThePrototypeAtConstructionTime pins that replacing .prototype
// changes what later instances link to, which is what `B.prototype = new A()` relies on.
// An instance built before the replacement keeps the object it was linked to.
func TestConstructReadsThePrototypeAtConstructionTime(t *testing.T) {
	f := NewCtor("F", 0, func(this Value, args []Value) Value { return Undefined })
	before := Construct(f)
	replacement := NewObject()
	f.Set(FromGoString("prototype"), replacement)
	after := Construct(f)
	if !StrictEquals(after.ProtoRead(), replacement) {
		t.Fatal("want an instance built after the replacement to link to the new prototype")
	}
	if StrictEquals(before.ProtoRead(), replacement) {
		t.Fatal("want an instance built before the replacement to keep its own prototype")
	}
}

// TestConstructLetsAnObjectReturnOverrideTheInstance pins the spec's override rule: a
// constructor that returns an object hands that object to the caller, and one that
// returns a primitive has the primitive discarded.
func TestConstructLetsAnObjectReturnOverrideTheInstance(t *testing.T) {
	other := NewObject()
	other.Set(FromGoString("which"), StringValue(FromGoString("other")))
	object := NewCtor("F", 0, func(this Value, args []Value) Value { return other })
	if !StrictEquals(Construct(object), other) {
		t.Fatal("want an object return to override the instance")
	}
	primitive := NewCtor("G", 0, func(this Value, args []Value) Value {
		this.Set(FromGoString("which"), StringValue(FromGoString("instance")))
		return Number(5)
	})
	got := Construct(primitive).Get(FromGoString("which")).AsString().ToGoString()
	if got != "instance" {
		t.Fatalf("want a primitive return discarded, got %q", got)
	}
}

// TestConstructThrowsOnANonConstructor pins that `new` over a value with no
// [[Construct]] is the TypeError the language throws, not a silent undefined. A plain
// boxed callable is the case that matters: it has a body but nothing new can apply to.
func TestConstructThrowsOnANonConstructor(t *testing.T) {
	fn := NewFunc(func(args []Value) Value { return Undefined })
	if IsConstructor(fn) {
		t.Fatal("want a plain boxed callable to report as not constructible")
	}
	if got := thrownName(t, func() { Construct(fn) }); got != "TypeError" {
		t.Fatalf("want a TypeError, got %q", got)
	}
}

// TestCallWithThisRunsAConstructorBodyOverAReceiver pins constructor chaining: the
// derived constructor calls the base one over the object it is building, so the base's
// writes land on that object rather than on one of its own.
func TestCallWithThisRunsAConstructorBodyOverAReceiver(t *testing.T) {
	base := NewCtor("Base", 1, func(this Value, args []Value) Value {
		this.Set(FromGoString("kind"), Arg(args, 0))
		return Undefined
	})
	derived := NewCtor("Derived", 0, func(this Value, args []Value) Value {
		CallWithThis(base, this, StringValue(FromGoString("base")))
		this.Set(FromGoString("own"), Number(1))
		return Undefined
	})
	v := Construct(derived)
	if got := v.Get(FromGoString("kind")).AsString().ToGoString(); got != "base" {
		t.Fatalf("want the base body to have written onto the derived instance, got %q", got)
	}
	if got := ToNumber(v.Get(FromGoString("own"))); got != 1 {
		t.Fatalf("want the derived body's own write, got %v", got)
	}
}

// TestCallWithThisDropsAReceiverAValueCannotHold pins that a callable with no receiver
// slot still runs when one is supplied: it reads the undefined its body would have read
// anyway rather than failing the call.
func TestCallWithThisDropsAReceiverAValueCannotHold(t *testing.T) {
	fn := NewFunc(func(args []Value) Value { return Number(float64(len(args))) })
	if got := ToNumber(CallWithThis(fn, NewObject(), Number(1), Number(2))); got != 2 {
		t.Fatalf("want the arguments through and the receiver dropped, got %v", got)
	}
}

// TestNewMethodReadsItsReceiver pins the method box: called through CallMethod off the
// object that holds it, the body reads that object as its receiver, which is what a
// prototype method needs and what a plain boxed callable cannot do.
func TestNewMethodReadsItsReceiver(t *testing.T) {
	m := NewMethod(func(this Value, args []Value) Value {
		return this.Get(FromGoString("tag"))
	})
	o := NewObject()
	o.Set(FromGoString("tag"), StringValue(FromGoString("here")))
	o.Set(FromGoString("who"), m)
	if got := CallMethod(o, FromGoString("who")).AsString().ToGoString(); got != "here" {
		t.Fatalf("want the method to read its receiver, got %q", got)
	}
	// Called with nothing bound, the way a callback is invoked, the receiver is the
	// undefined a receiver-free call leaves.
	if got := m.Call(); !StrictEquals(got, Undefined) {
		t.Fatalf("want undefined for a receiver-free call, got %v", got)
	}
}

// TestCallMethodClimbsThePrototypeChain pins that the receiver a method reads is the
// object the call selected it from, not the prototype the method was found on. That is
// the whole point of putting a method on a prototype: one function, one body, and every
// instance reads its own fields through it.
func TestCallMethodClimbsThePrototypeChain(t *testing.T) {
	ctor := NewCtor("Tagged", 1, func(this Value, args []Value) Value {
		this.Set(FromGoString("tag"), Arg(args, 0))
		return Undefined
	})
	proto := ctor.Get(FromGoString("prototype"))
	proto.Set(FromGoString("who"), NewMethod(func(this Value, args []Value) Value {
		return this.Get(FromGoString("tag"))
	}))
	a := Construct(ctor, StringValue(FromGoString("a")))
	b := Construct(ctor, StringValue(FromGoString("b")))
	if got := CallMethod(a, FromGoString("who")).AsString().ToGoString(); got != "a" {
		t.Fatalf("want the first instance's own tag, got %q", got)
	}
	if got := CallMethod(b, FromGoString("who")).AsString().ToGoString(); got != "b" {
		t.Fatalf("want the second instance's own tag, got %q", got)
	}
}

// TestCallMethodLeavesAPlainCallableAlone pins that threading a receiver changes
// nothing for the boxed callables that fill most dynamic slots: the arguments arrive as
// they always did and no receiver reaches a body that has no slot for one.
func TestCallMethodLeavesAPlainCallableAlone(t *testing.T) {
	o := NewObject()
	o.Set(FromGoString("add"), NewFunc(func(args []Value) Value {
		return Number(ToNumber(Arg(args, 0)) + ToNumber(Arg(args, 1)))
	}))
	if got := ToNumber(CallMethod(o, FromGoString("add"), Number(2), Number(3))); got != 5 {
		t.Fatalf("want 5, got %v", got)
	}
}

// TestInstanceOfWalksThePrototypeChain pins the chain walk both ways: an instance is an
// instance of its own constructor and of every constructor above it once the prototypes
// are linked, and an unrelated object is an instance of neither.
func TestInstanceOfWalksThePrototypeChain(t *testing.T) {
	base := NewCtor("Base", 0, func(this Value, args []Value) Value { return Undefined })
	derived := NewCtor("Derived", 0, func(this Value, args []Value) Value { return Undefined })
	derived.Get(FromGoString("prototype")).SetPrototype(base.Get(FromGoString("prototype")))
	v := Construct(derived)
	if !InstanceOf(v, derived) {
		t.Fatal("want an instance of its own constructor")
	}
	if !InstanceOf(v, base) {
		t.Fatal("want an instance of the base once the prototypes are linked")
	}
	if InstanceOf(NewObject(), base) {
		t.Fatal("want a plain object to be an instance of neither")
	}
}

// TestInstanceOfIsFalseForAPrimitive pins that instanceof does not coerce its left
// side: a primitive answers false outright rather than being boxed into a wrapper whose
// chain would then be walked.
func TestInstanceOfIsFalseForAPrimitive(t *testing.T) {
	f := NewCtor("F", 0, func(this Value, args []Value) Value { return Undefined })
	for _, v := range []Value{Number(1), StringValue(FromGoString("s")), True, Undefined, Null} {
		if InstanceOf(v, f) {
			t.Fatalf("want false for the primitive %v", v)
		}
	}
}

// TestInstanceOfThrowsOnANonCallableRightSide pins the TypeError the spec gives when
// the right side of instanceof is not callable, which is the failure a program gets for
// `x instanceof {}`.
func TestInstanceOfThrowsOnANonCallableRightSide(t *testing.T) {
	if got := thrownName(t, func() { InstanceOf(NewObject(), NewObject()) }); got != "TypeError" {
		t.Fatalf("want a TypeError, got %q", got)
	}
}
