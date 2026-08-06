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
	return StructToValue(x)
}

// classInstance returns the Go instance a class view reads through, and whether this
// value is one. A boxed collection reaches for this to turn a value handed to
// map.set or map.get back into the typed element it holds, which is the whole reason
// the view carries a back-pointer: a bag of copied fields could be recognized as an
// instance of the class but never as the instance.
func (v Value) classInstance() (any, bool) {
	if v.kind != KindObject {
		return nil, false
	}
	o := v.object()
	if o.jsClass == nil {
		return nil, false
	}
	return o.jsClass, true
}

// classLiveFields defines one live data property per exported field of the instance,
// visiting the struct the way jsonStructFields does so the two agree on which fields
// an instance has and in what order: an unexported field is machinery and skipped, an
// anonymous field flattens its own fields into the same object so a derived class's
// inherited fields sit first, and an absent optional property contributes no key.
//
// The struct value here is reached through the instance pointer, so its fields are
// addressable and a closure over one can write to it. That is what separates this from
// the fixed-shape box next door, which copies: an instance keeps living on the typed
// side after it is boxed, so a read through the view has to answer what the field
// holds now rather than what it held when the view was made.
func classLiveFields(obj Value, rv reflect.Value) {
	t := rv.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if f.PkgPath != "" {
			continue
		}
		if f.Anonymous {
			classLiveFields(obj, rv.Field(i))
			continue
		}
		fv := rv.Field(i)
		if opt, ok := fv.Interface().(jsonOptional); ok {
			// An optional property absent when the view is made contributes no key, the same
			// choice the copying walk makes. The value of a present one stays live; only its
			// presence is read once, so a field that becomes present later is not shown until
			// the instance is boxed again.
			if _, present := opt.jsonOptField(); !present {
				continue
			}
		}
		key, omitUndef := jsonFieldKey(f)
		// An optional any or unknown member holds a bare Value, so an omitted one reads
		// as undefined and contributes no key, the same choice the copying walk makes.
		if omitUndef && jsonUndefinedGo(fv.Interface()) {
			continue
		}
		name := key
		obj.object().defineOwn(FromGoString(key), liveProperty(
			func() Value { return classFieldRead(fv) },
			func(val Value) { classFieldWrite(fv, name, val) },
		))
	}
}

// classFieldRead boxes what a field holds now, through the same lift every other
// boxing of a Go value goes through, so a field holding a date, a map or another
// instance reads as that rather than as a bag of its storage.
func classFieldRead(fv reflect.Value) Value {
	v := fv.Interface()
	if opt, ok := v.(jsonOptional); ok {
		inner, present := opt.jsonOptField()
		if !present {
			return Undefined
		}
		v = inner
	}
	return jsonToValue(v)
}

// classFieldWrite lands a write made through the view in the field it views. It
// raises on the two ways that can fail rather than storing the value in the view,
// where it would read back correctly once and be gone from the instance: a field
// whose Go type has no inbound conversion, and a value of a type the field cannot
// hold. Both are what dynUnboxOrThrow refuses for a collection element and for the
// same reason, that a dropped write cannot be told from a write that happened.
func classFieldWrite(fv reflect.Value, name string, val Value) {
	if !classFieldSettable(fv.Type()) {
		Throw(NewTypeError(FromGoString("bento: writing to ." + name + " through a boxed instance is a later slice, its type has no dynamic form")))
		return
	}
	if !classFieldStore(fv, val) {
		Throw(NewTypeError(FromGoString("bento: ." + name + " of this instance cannot hold a value of this type")))
	}
}

// classFieldSettable reports whether a field's Go type has an inbound conversion at
// all, which is what tells a write that could never work from a write of the wrong
// value. The list is the one dynUnbox converts a collection element back through,
// plus another class instance, which comes back through its view's back-pointer.
func classFieldSettable(t reflect.Type) bool {
	switch reflect.Zero(t).Interface().(type) {
	case Value, float64, BStr, bool, *Date, *RegExp, *ArrayBuffer, *SharedArrayBuffer, *DataView:
		return true
	}
	if t.Kind() != reflect.Pointer && t.Kind() != reflect.Interface {
		return false
	}
	for _, iface := range []reflect.Type{typedArrayBackingType, mapBackingType, setBackingType} {
		if t.Implements(iface) {
			return true
		}
	}
	return t.Kind() == reflect.Pointer && classPrototypeFor(t.Elem()) != nil
}

// classFieldStore performs the write, reporting false when the value is not of the
// field's kind. A reference field takes the very object the value boxes rather than a
// copy of it, so a date or a map written through a view is the one the writer holds
// and the two sides do not drift apart.
func classFieldStore(fv reflect.Value, val Value) bool {
	switch fv.Interface().(type) {
	case Value:
		fv.Set(reflect.ValueOf(val))
		return true
	case float64:
		if val.kind == KindNumber {
			fv.SetFloat(val.AsNumber())
			return true
		}
		return false
	case BStr:
		if val.kind == KindString {
			fv.Set(reflect.ValueOf(val.AsString()))
			return true
		}
		return false
	case bool:
		if val.kind == KindBool {
			fv.SetBool(val.AsBool())
			return true
		}
		return false
	}
	if d := val.asDate(); d != nil && classFieldStoreRef(fv, d) {
		return true
	}
	if r := val.asRegExp(); r != nil && classFieldStoreRef(fv, r) {
		return true
	}
	if b := val.asBuffer(); b != nil && classFieldStoreRef(fv, b) {
		return true
	}
	if w := val.asDataView(); w != nil && classFieldStoreRef(fv, w) {
		return true
	}
	if a := val.asTypedArray(); a != nil && classFieldStoreRef(fv, a) {
		return true
	}
	if m := val.asMap(); m != nil && classFieldStoreRef(fv, m) {
		return true
	}
	if s := val.asSet(); s != nil && classFieldStoreRef(fv, s) {
		return true
	}
	if p, ok := val.classInstance(); ok && classFieldStoreRef(fv, p) {
		return true
	}
	return false
}

// classFieldStoreRef assigns a reference the value boxes when the field can hold it.
// The assignability check is what keeps an Int32Array out of a Float64Array field and
// an instance of one class out of a field declared for another, since both arrive
// here as the interface the family shares.
func classFieldStoreRef(fv reflect.Value, x any) bool {
	rx := reflect.ValueOf(x)
	if !rx.IsValid() || !rx.Type().AssignableTo(fv.Type()) {
		return false
	}
	fv.Set(rx)
	return true
}

var (
	typedArrayBackingType = reflect.TypeOf((*typedArrayBacking)(nil)).Elem()
	mapBackingType        = reflect.TypeOf((*mapBacking)(nil)).Elem()
	setBackingType        = reflect.TypeOf((*setBacking)(nil)).Elem()
)

// classCoercions holds the two members a boxed instance answers by running the class's
// own code rather than by reading a copied field: toString and valueOf. A box carries
// the fields and the class name, so without these an instance that writes its own
// toString would read as "[object Object]" the moment it crossed into a dynamic value,
// which is a wrong string rather than a missing one.
//
// Only these two are here because they are the two the language calls on its own. Every
// other method is reached by a call the program writes, which has a typed call site to
// run it at; ToPrimitive has no such site, so it has to find the method through the box.
type classCoercions struct {
	toString func(any) Value
	valueOf  func(any) Value
}

// classCoercionRegistry maps a generated class struct's Go type to the coercions its
// instances answer. It is separate from classRegistry rather than a field on the
// prototype so the two registrations are independent: each rides its own package-level
// var and neither has to be initialized before the other.
var classCoercionRegistry sync.Map // reflect.Type -> *classCoercions

// RegisterClassCoercion records that instances of the struct sample points at answer
// the named coercion by calling fn, and returns true so the registration can ride a
// package-level var the way RegisterClass does. The lowerer emits one of these per
// class that writes a toString or a valueOf, with fn closing over the typed call and
// the boxing of what it returns, so nothing here has to know the class's Go signature.
//
// A name that is neither is ignored rather than stored, which keeps the read path a
// two-way switch instead of a map lookup on every property miss.
func RegisterClassCoercion(sample any, name string, fn func(any) Value) bool {
	t := reflect.TypeOf(sample)
	if t == nil || t.Kind() != reflect.Pointer {
		return true
	}
	entry, _ := classCoercionRegistry.LoadOrStore(t.Elem(), &classCoercions{})
	c := entry.(*classCoercions)
	switch name {
	case "toString":
		c.toString = fn
	case "valueOf":
		c.valueOf = fn
	}
	return true
}

// classCoercionGet answers a coercion read off a boxed instance, bound to the instance
// the view reads through, or reports false when the class registered none by that name.
// The method is bound here rather than installed on the interned prototype because a
// Call carries no receiver in this value model: every other box binds its members at the
// read for the same reason, which is why a boxed Map's get knows which map it came from.
//
// A class that writes neither reports false here and falls through to the chain walk,
// which ends at Object.prototype and answers both: the class tag for toString and the
// object itself for valueOf. That fallback is not a courtesy, it is what keeps the two
// hints apart. The string hint asks for toString first and takes its answer, so a class
// writing only a valueOf whose box answered only valueOf would read String(v) as "7"
// where the engine reads "[object Object]"; and an identity valueOf is object-like, so
// the default hint falls through it to toString the way OrdinaryToPrimitive does.
func classCoercionGet(inst any, name string) (Value, bool) {
	var c *classCoercions
	if t := reflect.TypeOf(inst); t != nil && t.Kind() == reflect.Pointer {
		if entry, ok := classCoercionRegistry.Load(t.Elem()); ok {
			c = entry.(*classCoercions)
		}
	}
	if c == nil {
		return Undefined, false
	}
	switch name {
	case "toString":
		if c.toString != nil {
			return boundMethod(name, func([]Value) Value { return c.toString(inst) }), true
		}
	case "valueOf":
		if c.valueOf != nil {
			return boundMethod(name, func([]Value) Value { return c.valueOf(inst) }), true
		}
	}
	return Undefined, false
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
