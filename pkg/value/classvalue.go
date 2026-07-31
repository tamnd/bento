// This file gives a class instance a dynamic form. A class lowers to a Go struct
// with methods, so an instance flowing into a dynamic slot (console.log, a member of
// a boxed container, assert) has to present itself as an object carrying its fields
// under a name: Node prints "P { x: 1 }", not "{ x: 1 }", and its deep comparison
// holds two instances of one class apart from two of another and from a plain object
// with the same fields.
//
// Both of those hang off one thing, the prototype. Node reads the constructor name by
// climbing the prototype chain, and its strict comparison decides two objects have the
// same constructor by comparing prototypes. So a boxed instance is an ordinary object
// carrying its fields plus a prototype interned per class, and the renderer and the
// comparison then need nothing of their own: they already do the right thing with a
// prototype that names a constructor.
//
// The generated struct knows its class name at compile time and nothing else does, so
// each class registers its Go type here at package initialization. That is what lets a
// class instance reached through reflection be named too, which is every position but
// the top one: a class field holding another instance, an instance in an array, an
// instance stored in a boxed collection.

package value

import (
	"reflect"
	"sync"
)

// classRegistry maps a generated class struct's Go type to the interned prototype
// object every instance of that class is boxed with. It is written once per class
// before main runs and read on every boxing after that, so it is a sync.Map rather
// than a plain map with a lock: the read path is the hot one and it never contends.
var classRegistry sync.Map // reflect.Type -> *Object

// RegisterClass records that instances of the generated struct sample points at are
// instances of the JavaScript class called name, and returns true so the registration
// can ride a package-level var declaration rather than an init function. The lowerer
// emits one of these per class it renders.
//
// The prototype is built here rather than on the first boxing so every instance of one
// class shares one prototype object however it is reached. That identity is what the
// strict deep comparison reads: two instances of P carry the same prototype pointer and
// compare as having the same constructor, while an instance of Q and a plain object,
// which carries no prototype at all, do not.
func RegisterClass(name string, sample any) bool {
	t := reflect.TypeOf(sample)
	if t == nil || t.Kind() != reflect.Pointer {
		return true
	}
	classRegistry.LoadOrStore(t.Elem(), newClassPrototype(name))
	return true
}

// newClassPrototype builds the prototype object a class's instances carry. It holds
// one property, the constructor, which is what Node's getConstructorName climbs the
// chain looking for and reads the name off. The property is not enumerable, matching
// the language: Object.keys of an instance lists its own fields and nothing from the
// prototype, and a for-in over one does not reach the constructor either.
//
// The constructor is a callable that raises rather than a working constructor. A
// program that reads p.constructor to compare it or to read its name gets what it
// asked for; one that calls it is asking to construct an instance from the dynamic
// side, which needs the class's lowered constructor reachable as a value, and that is
// not this slice. Raising is honest where handing back a half-built object would not be.
func newClassPrototype(name string) *Object {
	ctor := WithName(NewFunc(func([]Value) Value {
		Throw(NewTypeError(FromGoString("bento: calling a class constructor through its boxed instance is a later slice")))
		return Undefined
	}), name)
	proto := &Object{kind: KindObject}
	proto.defineOwn(FromGoString("constructor"), dataProperty(ctor, true, false, true))
	return proto
}

// ClassToValue boxes one class instance, the generic form an element boxer has to have.
// ObjectFromStruct takes an any, which is the right shape for a boxing site that names
// the value directly, but ArrayValueOf wants a func(T) Value it can apply down a typed
// slice, and a func(any) Value does not fit that however compatible the call would be.
// This is that function with the type parameter spelled, so an array of instances boxes
// with the element boxer inferred from the slice and no closure emitted per class.
func ClassToValue[T any](x T) Value {
	return ObjectFromStruct(x)
}

// classPrototypeFor returns the interned prototype for a boxed struct's type, or nil
// when the type is not a registered class. A plain fixed-shape object struct is not
// registered, so it boxes with no prototype and reads as a plain object, which is what
// Node prints for an object literal.
func classPrototypeFor(t reflect.Type) *Object {
	if proto, ok := classRegistry.Load(t); ok {
		return proto.(*Object)
	}
	return nil
}
