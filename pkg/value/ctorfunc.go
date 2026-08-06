// This file owns the ES5 constructor function at runtime: a callable value that is
// also constructible, carrying the .prototype object every instance it builds links
// to. It is the other half of the prototype chain prototype.go owns. That file can
// already read and write an object's [[Prototype]]; this one supplies the object a
// program actually puts there, because a constructor function's .prototype is where
// nearly every real JavaScript program keeps the methods its instances share.
//
//	function ArrayStream() {}
//	Object.setPrototypeOf(ArrayStream.prototype, Stream.prototype);
//	ArrayStream.prototype.readable = true;
//	const s = new ArrayStream();
//
// Node's own test helpers are written in exactly this shape, so nothing that reaches
// through test/common runs without it.
//
// A boxed callable elsewhere in the runtime takes no receiver (see callBack in
// arraylike.go): bento's plain functions get undefined for `this`, which is the
// honest answer for a callback. A constructor is the one function whose receiver is
// not the caller's to supply, it is the fresh object [[Construct]] makes, so a
// constructor body is stored as a ctorFn, which takes that object as its first
// argument.

package value

// ctorFn is the body of a constructor function: it takes the receiver the call
// bound, the new object under `new` and undefined under a plain call, and its
// arguments already boxed. It returns whatever the body returned, which Construct
// then judges against the object it made.
type ctorFn func(this Value, args []Value) Value

// NewCtor boxes a Go closure as an ES5 constructor function value, the pair a
// `function F(...) {...}` declaration creates when a program treats it as a
// constructor: F is callable and constructible, F.prototype is a fresh object, and
// F.prototype.constructor points back at F.
//
// The three own properties are defined rather than assigned, with the attributes the
// spec gives them, so none of them shows up in Object.keys(F) or in a logged
// rendering of it. prototype is writable and non-configurable, which is what lets
// `B.prototype = new A()` replace the object wholesale; name and length are the
// non-enumerable, configurable pair every function carries.
//
// arity is the declared parameter count the function reports as .length, which is
// the count before the first default or rest parameter, the same number the lowerer
// reads off the declared signature.
func NewCtor(name string, arity int, fn ctorFn) Value {
	o := &Object{kind: KindFunc, construct: fn}
	// A constructor called without new still runs its body, with whatever receiver the
	// call bound. Nothing bento emits binds one, and every file that reaches this shape
	// is strict, so that is undefined rather than the global object.
	o.call = func(args []Value) Value { return fn(Undefined, args) }
	f := objectValue(o)

	proto := &Object{kind: KindObject}
	protoVal := objectValue(proto)
	proto.defineOwn(FromGoString("constructor"), dataProperty(f, true, false, true))

	o.defineOwn(FromGoString("prototype"), dataProperty(protoVal, true, false, false))
	o.defineOwn(FromGoString("name"), dataProperty(StringValue(FromGoString(name)), false, false, true))
	o.defineOwn(FromGoString("length"), dataProperty(Number(float64(arity)), false, false, true))
	return f
}

// NewMethod boxes a Go closure as a function value that reads its receiver, the box
// a function expression takes when it is written onto an object and then called back
// off it:
//
//	A.prototype.who = function () { return "A:" + this.tag; };
//	new A("x").who();
//
// A plain NewFunc box has no receiver slot, so a body like that one would read
// undefined for `this` and answer "A:undefined", which is a wrong answer rather than
// a refusal. A method value carries the body as a ctorFn instead, and the method-call
// path hands it the object the call selected it from.
//
// The value is otherwise an ordinary callable: WithName still names it, a property
// read still finds its own keys, and calling it with no receiver, the way a plain
// callback is invoked, binds the undefined a receiver-free call leaves.
func NewMethod(fn ctorFn) Value {
	o := &Object{kind: KindFunc, recv: fn}
	o.call = func(args []Value) Value { return fn(Undefined, args) }
	return objectValue(o)
}

// CallMethod runs obj[key](args), the receiver-preserving form of a method call on a
// dynamic value. It is one operation rather than a read followed by a call so the
// object the property came off can be bound as the callee's `this`, which is what the
// language does and what a plain `obj.Get(key).Call(args)` throws away.
//
// A callee that has no receiver slot, every boxed function that is not a method or a
// constructor, ignores the receiver and runs exactly as it did.
func CallMethod(obj Value, key BStr, args ...Value) Value {
	return CallWithThis(obj.Get(key), obj, args...)
}

// IsConstructor reports whether v is a value `new` can be applied to, which here
// means a function value built by NewCtor. A plain boxed callable is not one: it has
// a body but no [[Construct]], the same distinction JavaScript draws between an
// arrow function and a function declaration.
func IsConstructor(v Value) bool {
	return v.kind == KindFunc && v.object().construct != nil
}

// Construct runs [[Construct]] over a constructor value, the runtime behind
// `new F(args)`. It makes a fresh object whose [[Prototype]] is the constructor's
// current .prototype, runs the body with that object as its receiver, and answers
// the object unless the body returned an object of its own, which overrides it the
// way `function F() { return other; }` does.
//
// The prototype is read at construction time rather than captured when the
// constructor was made, because a program is free to replace it first, and the whole
// point of `B.prototype = new A()` is that instances built after the assignment link
// to the new object. A .prototype that is not an object leaves the instance on the
// default chain, which is what the spec's OrdinaryCreateFromConstructor falls back to.
func Construct(fn Value, args ...Value) Value {
	if p := fn.asProxy(); p != nil {
		return p.construct(args, fn)
	}
	if !IsConstructor(fn) {
		Throw(NewTypeError(fn.TypeOf().ConcatN(FromGoString(" is not a constructor"))))
		return Undefined
	}
	o := fn.object()
	var this Value
	switch proto := o.getChained(fn, FromGoString("prototype")); proto.kind {
	case KindObject, KindArray, KindFunc:
		this = ObjectCreate(proto)
	default:
		this = NewObject()
	}
	res := o.construct(this, args)
	switch res.kind {
	case KindObject, KindArray, KindFunc:
		return res
	default:
		return this
	}
}

// CallWithThis invokes a function value with an explicit receiver, the runtime
// behind F.call(obj, ...). It is the constructor-chaining idiom, where a derived
// constructor runs the base constructor's body over the object it is building:
//
//	function ArrayStream() { Stream.call(this); }
//
// A constructor value and a method value each honor the receiver, since both bodies
// take one. Any other callable does not have a receiver slot to fill, so the argument
// would set a `this` the body could never read; the lowering refuses that case rather
// than reach here, and a value that arrives anyway runs with the receiver dropped,
// which is the same undefined its body would have read.
func CallWithThis(fn, this Value, args ...Value) Value {
	if fn.kind == KindFunc {
		o := fn.object()
		if o.recv != nil {
			return o.recv(this, args)
		}
		if o.construct != nil {
			return o.construct(this, args)
		}
	}
	return fn.Call(args...)
}

// InstanceOf reports whether v was built by ctor, the runtime behind
// `v instanceof ctor`. It is the ordinary [[HasInstance]]: the constructor's current
// .prototype is looked up and v's prototype chain walked for it, so an object built
// before a prototype was replaced answers against the object it actually links to,
// which is what the language does.
//
// A right-hand side that is not callable, and one whose .prototype is not an object,
// each throw a TypeError the way the spec rejects them. A Symbol.hasInstance method
// overriding the test is a later slice; nothing in the runtime installs one.
func InstanceOf(v, ctor Value) bool {
	if ctor.kind != KindFunc {
		Throw(NewTypeError(FromGoString("Right-hand side of 'instanceof' is not callable")))
		return false
	}
	proto := ctor.object().getChained(ctor, FromGoString("prototype"))
	switch proto.kind {
	case KindObject, KindArray, KindFunc:
	default:
		Throw(NewTypeError(FromGoString("Function has non-object prototype in instanceof check")))
		return false
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
	default:
		// A primitive is never an instance of anything: instanceof does not coerce its
		// left side, it answers false for it outright.
		return false
	}
	want := proto.object()
	for o := v.object().proto; o != nil; o = o.proto {
		if o == want {
			return true
		}
	}
	return false
}
