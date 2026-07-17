// This file owns the boxed dynamic Value, the self-describing representation the
// dynamic world uses when the checker could not prove a static shape (10_value_model
// sections 3 and 4). The typed world never touches it on the hot path; it appears
// only where the program is genuinely dynamic, which for the AOT path today is the
// result of JSON.parse and the coercions an any-typed expression forces.
//
// The box is a tagged struct, not a NaN-boxed word, because Go's precise garbage
// collector must see the reference payload as a real pointer to scan it. The
// layout follows the spec: a one-byte tag, a scalar payload for the immediate
// kinds, and a pointer payload for the reference kinds. The reference storage is
// an ordered property map rather than the spec's hidden-class shapes; shapes are a
// performance lever the dynamic path will grow later, and an ordered map is
// behaviorally identical, only slower.

package value

import (
	"math"
	"unsafe"
)

// Kind is the runtime tag of a boxed Value, one case per JavaScript type plus the
// flat array and function cases the spec splits out so their fast paths do not go
// through the generic object case.
type Kind uint8

const (
	KindUndefined Kind = iota
	KindNull
	KindBool
	KindNumber
	KindBigInt
	KindString
	KindSymbol
	KindObject
	KindArray
	KindFunc
	// KindHole is the internal tag of an array hole, an index below length that
	// carries no own property. It fills a gap in an array's dense element storage so
	// a hole is distinct from a stored undefined: a read sees undefined either way,
	// but the in operator, hasOwnProperty, and enumeration treat a hole as absent. It
	// never escapes to user code, so typeof and the coercions never see it.
	KindHole
)

// hole is the singleton stored in an array's element slice for an index that has no
// own property, the gap delete a[i] leaves and the padding a[5] = x on a shorter
// array creates. It is compared by kind, never read as a value, so it needs no
// scalar or reference payload.
var hole = Value{kind: KindHole}

// isHole reports whether an array element slot is a hole rather than a present
// value, the presence test the hole-sensitive reads and enumeration make before
// treating an index as an own property.
func isHole(v Value) bool { return v.kind == KindHole }

// Value is the boxed, self-describing dynamic value. It is three machine words:
// the tag, a scalar for immediates (a bool in the low bit or a number's raw
// float64 bits), and a pointer the collector scans for the reference kinds. It is
// passed by value, so a Value in a local or a slice lives inline with no separate
// allocation; only the reference kinds put anything on the heap.
type Value struct {
	kind   Kind
	scalar uint64
	ref    unsafe.Pointer
}

// The singletons. undefined, null, and the two booleans carry no reference, so
// they are cheap to return and to compare.
var (
	Undefined = Value{kind: KindUndefined}
	Null      = Value{kind: KindNull}
	True      = Value{kind: KindBool, scalar: 1}
	False     = Value{kind: KindBool, scalar: 0}
)

// Bool boxes a Go bool as one of the two singletons.
func Bool(b bool) Value {
	if b {
		return True
	}
	return False
}

// Number boxes a float64. The raw bits are stored, so a NaN payload and a
// negative zero round-trip unchanged, which the number-to-string and equality
// paths rely on.
func Number(f float64) Value {
	return Value{kind: KindNumber, scalar: math.Float64bits(f)}
}

// StringValue boxes a BStr. The string is a value type, so it is copied to the
// heap and the box holds a pointer to that copy, which the collector scans as the
// reference payload.
func StringValue(s BStr) Value {
	return Value{kind: KindString, ref: unsafe.Pointer(&s)}
}

// objectValue boxes an *Object as either the object or the array kind, so an
// array keeps its flat tag for the length and index fast paths while sharing the
// one reference type.
func objectValue(o *Object) Value {
	switch o.kind {
	case KindArray:
		return Value{kind: KindArray, ref: unsafe.Pointer(o)}
	case KindFunc:
		// A function is an object too, so it shares Object storage, but its box keeps
		// the func tag so typeof reports "function" and its callable body stays
		// reachable through the same reference.
		return Value{kind: KindFunc, ref: unsafe.Pointer(o)}
	default:
		return Value{kind: KindObject, ref: unsafe.Pointer(o)}
	}
}

// Kind reports the value's runtime tag.
func (v Value) Kind() Kind { return v.kind }

// IsUndefined and the other predicates ask the tag directly, the cheap check the
// dynamic path makes before it commits to a kind-specific operation.
func (v Value) IsUndefined() bool { return v.kind == KindUndefined }
func (v Value) IsNull() bool      { return v.kind == KindNull }
func (v Value) IsNullish() bool   { return v.kind == KindUndefined || v.kind == KindNull }

// IsArray reports whether v is a real array, the runtime brand check Array.isArray
// makes. It asks the tag, so it says true only for an array value and false for an
// array-like object, a typed array box, a string, or any other value, matching the
// exotic-array brand the spec tests rather than a duck-typed length probe.
func IsArray(v Value) bool { return v.Kind() == KindArray }

// TypeOf returns the JavaScript typeof string for the boxed value, the lowering
// of typeof x when the operand is dynamic and its kind is only known at runtime.
// The mapping is the language's, not Go's: null reports "object" (the historical
// wart), an array is an "object" like any other, and only a callable is
// "function". A static operand never reaches here; the lowerer folds typeof to a
// string constant when the checker already knows the kind, and emits this call
// only when the operand is any or unknown.
func (v Value) TypeOf() BStr {
	switch v.kind {
	case KindUndefined:
		return FromGoString("undefined")
	case KindBool:
		return FromGoString("boolean")
	case KindNumber:
		return FromGoString("number")
	case KindBigInt:
		return FromGoString("bigint")
	case KindString:
		return FromGoString("string")
	case KindSymbol:
		return FromGoString("symbol")
	case KindFunc:
		return FromGoString("function")
	default:
		// null, object, and array all report "object".
		return FromGoString("object")
	}
}

// AsNumber returns the double a number box holds, decoding the raw bits. It is
// only valid on a KindNumber value; the caller checks the kind first, or reaches
// for ToNumber when the kind is not known.
func (v Value) AsNumber() float64 { return math.Float64frombits(v.scalar) }

// AsBool returns the bool a boolean box holds.
func (v Value) AsBool() bool { return v.scalar != 0 }

// AsString returns the BStr a string box holds. Like AsNumber it is only valid
// on a KindString value: lowered code calls it where the checker proved the
// kind, past a typeof guard, and reaches for ToString when the kind is open.
func (v Value) AsString() BStr { return v.str() }

// str returns the BStr a string box holds, dereferencing the heap copy. It is
// unexported because only this package's coercions read it; a caller outside gets
// a string through ToString.
func (v Value) str() BStr { return *(*BStr)(v.ref) }

// object returns the *Object an object, array, or function box holds.
func (v Value) object() *Object { return (*Object)(v.ref) }

// GetIndex reads v[i] for a numeric index, the bracket read a[i] takes when the
// receiver is a dynamic value and the index is a number. The index becomes a
// property key its canonical string the way JavaScript's a[3] reads the "3"
// property, then the read dispatches by the receiver's kind through Get, so an
// array element, a string code unit, and an object numeric property all resolve
// the same way a static read would.
func (v Value) GetIndex(i float64) Value {
	return v.Get(NumberToString(i))
}

// GetElem reads v[key] for a dynamic index whose own type is not known to be a
// number, the bracket read a[k] takes when both the receiver and the key are
// dynamic values. The key is coerced to a property key the way JavaScript does, a
// string used as is and any other value taken through ToString, then the read
// dispatches through Get. A number key round-trips to its canonical string, so a
// dynamic index reads the same element GetIndex would.
func (v Value) GetElem(key Value) Value {
	if key.kind == KindSymbol {
		return v.getSymKey(key.symbol())
	}
	if key.kind == KindString {
		return v.Get(key.str())
	}
	return v.Get(ToString(key))
}

// getSymKey reads a symbol-keyed property off an object, array, or function
// receiver, the symbol branch of a dynamic bracket read o[s]. A symbol key never
// coerces to a string, so it is looked up by identity in the symbol bag; a
// primitive receiver carries no such property and reads undefined.
func (v Value) getSymKey(key *Symbol) Value {
	if p := v.asProxy(); p != nil {
		return p.getSym(v, key)
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().getSymChained(v, key)
	default:
		return Undefined
	}
}

// SetIndex writes v[i] = val for a numeric index, the bracket write a[i] = val
// takes when the receiver is a dynamic value and the index is a number. It mirrors
// GetIndex: the index becomes a property key its canonical string, then the write
// dispatches by the receiver's kind, so an array element lands in dense storage and
// an object numeric property lands in the property map the way a[3] = x does. It
// returns the assigned value so the write reads the same as JavaScript's assignment
// expression, which evaluates to its right-hand side.
func (v Value) SetIndex(i float64, val Value) Value {
	return v.SetKey(NumberToString(i), val)
}

// SetElem writes v[key] = val for a dynamic index whose own type is not known to
// be a number, the mirror of GetElem. The key is coerced to a property key the way
// JavaScript does, a string used as is and any other value taken through ToString,
// then the write dispatches through the same kind-aware path SetIndex uses, so a
// numeric string key round-trips to the same array element GetIndex would read.
func (v Value) SetElem(key, val Value) Value {
	if key.kind == KindSymbol {
		return v.setSymKey(key.symbol(), val)
	}
	if key.kind == KindString {
		return v.SetKey(key.str(), val)
	}
	return v.SetKey(ToString(key), val)
}

// SetKeyed writes a property whose key is a boxed value, resolving it to a symbol,
// string, or numeric-string property the way SetElem does, and returns the receiver
// so a boxed object literal can chain a computed member `{ [k]: v }` in one
// expression the way Set chains a named one. It differs from SetElem, whose
// assignment semantics return the assigned value, because literal construction
// needs the object back to keep building.
func (v Value) SetKeyed(key, val Value) Value {
	v.SetElem(key, val)
	return v
}

// setSymKey writes a symbol-keyed property onto an object, array, or function
// receiver, the symbol branch of a dynamic bracket write o[s] = val. It returns
// val so the write reads as JavaScript's assignment expression; a primitive
// receiver has no writable symbol storage and drops the write, returning val.
func (v Value) setSymKey(key *Symbol, val Value) Value {
	if p := v.asProxy(); p != nil {
		p.setSym(v, key, val)
		return val
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		v.object().setSym(v, key, val)
	}
	return val
}

// DeleteIndex removes v[i] for a numeric index, the delete a[i] takes when the
// receiver is a dynamic value and the index is a number. It mirrors GetIndex: the
// index becomes a property key its canonical string, then the removal dispatches
// by the receiver's kind through Delete, so an array element clears to a hole and
// an object numeric property drops from the map the way delete a[3] does.
func (v Value) DeleteIndex(i float64) bool {
	return v.Delete(NumberToString(i))
}

// DeleteElem removes v[key] for a dynamic index whose own type is not known to be
// a number, the mirror of GetElem. The key is coerced to a property key the way
// JavaScript does, a string used as is and any other value taken through
// ToString, then the removal dispatches through the same kind-aware Delete, so a
// numeric string key round-trips to the same array element DeleteIndex would.
func (v Value) DeleteElem(key Value) bool {
	if key.kind == KindSymbol {
		return v.deleteSymKey(key.symbol())
	}
	if key.kind == KindString {
		return v.Delete(key.str())
	}
	return v.Delete(ToString(key))
}

// deleteSymKey removes a symbol-keyed property from an object, array, or function
// receiver, the symbol branch of delete o[s]. Every property this model creates is
// configurable, so a removal never fails and a primitive receiver has nothing to
// remove, both reporting true the way delete does for a configurable or absent
// property.
func (v Value) deleteSymKey(key *Symbol) bool {
	if p := v.asProxy(); p != nil {
		return p.deleteSym(key)
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		return v.object().deleteSym(key)
	default:
		return true
	}
}

// MissingProperty is the value of a property read whose receiver's fixed shape
// does not declare the property. A shape interns to a Go struct that carries
// exactly its declared fields, so such a read is a provable miss and the language
// answers undefined. The receiver is passed and dropped rather than ignored at
// the call site so its evaluation still happens, keeping any effect a receiver
// expression like getObj().foo carries, and so the read references the receiver
// the Go compiler would otherwise flag as unused. It takes any because the
// receiver is a static Go value of the shape's struct type, not a boxed value.
func MissingProperty(recv any) Value {
	_ = recv
	return Undefined
}

// Get implements a dynamic property read, o[key], for the kinds the AOT path
// produces. A string reports its length and indexes to a one-character string; an
// array reports its length and indexes into its elements; an object looks the key
// up in its property map. A read that finds nothing is undefined, the JavaScript
// result for a missing property, so the caller never faults. The other kinds have
// no own properties the dynamic path reads yet and return undefined too.
func (v Value) Get(key BStr) Value {
	if p := v.asProxy(); p != nil {
		return p.get(v, key)
	}
	name := key.ToGoString()
	switch v.kind {
	case KindString:
		s := v.str()
		if name == "length" {
			return Number(s.Length())
		}
		if idx, ok := arrayIndex(name); ok {
			ch := s.CharAt(float64(idx))
			if ch.Length() == 0 {
				return Undefined
			}
			return StringValue(ch)
		}
		return Undefined
	case KindArray:
		o := v.object()
		if name == "length" {
			return Number(float64(len(o.elems)))
		}
		if idx, ok := arrayIndex(name); ok {
			if idx < len(o.elems) && !isHole(o.elems[idx]) {
				return o.elems[idx]
			}
			// A hole or an out-of-range index is not an own property, so the read climbs
			// the prototype chain the way a missing property does and ends at undefined.
			return o.getChained(v, key)
		}
		return o.getChained(v, key)
	case KindObject:
		return v.object().getChained(v, key)
	case KindFunc:
		// A function is an object too, so a named read finds its own properties: the
		// name a built-in error constructor carries is the read the caught-error tests
		// make. A function box with no own properties still climbs its prototype chain
		// and answers undefined for a miss at the end of it.
		return v.object().getChained(v, key)
	case KindSymbol:
		// A symbol's only own readable property is its description; every other named
		// read reaches Symbol.prototype and answers undefined here. A dynamic symbol
		// binding routes s.description through this path, matching the dedicated
		// SymbolDescription the statically-typed symbol path emits.
		if name == "description" {
			return v.SymbolDescription()
		}
		return Undefined
	default:
		return Undefined
	}
}

// HasProperty implements the in operator, key in v: whether v carries the named
// property, own or built in, for the kinds the AOT path produces. A string has a
// length and its in-range character indices; an array has a length and its in-range
// element indices as well as any own named property; an object or a function probes
// its own keys. JavaScript throws a TypeError when the right operand of in is not an
// object, so a primitive receiver raises rather than answering false.
func (v Value) HasProperty(key BStr) bool {
	if p := v.asProxy(); p != nil {
		return p.has(key)
	}
	name := key.ToGoString()
	switch v.kind {
	case KindString:
		if name == "length" {
			return true
		}
		if idx, ok := arrayIndex(name); ok {
			return v.str().CharAt(float64(idx)).Length() != 0
		}
		return false
	case KindArray:
		o := v.object()
		if name == "length" {
			return true
		}
		if idx, ok := arrayIndex(name); ok {
			return idx < len(o.elems) && !isHole(o.elems[idx])
		}
		return o.hasChained(key)
	case KindObject, KindFunc:
		return v.object().hasChained(key)
	default:
		Throw(NewTypeError(FromGoString("Cannot use 'in' operator to search for '" + name + "' in a non-object")))
		return false
	}
}

// InOperator implements the general in operator, key in obj, the property-existence
// check distinct from the discriminated-union tag test the lowerer folds a narrowing
// in to. The right operand must be an object: a string primitive carries length and
// index properties HasProperty would answer, but the language treats it as a non-object
// and throws, so only KindObject, KindArray, and KindFunc (a proxy is backed by one of
// these) pass. The key is coerced through ToPropertyKey: a symbol key is probed by
// identity along the prototype chain, and every other key by its property-key string,
// so a numeric key like 1 reads the "1" slot and a dynamic key reaches the same check a
// string key does. The existence probe climbs the prototype chain and sees a
// non-enumerable property, since HasProperty and hasSymChained both walk every own key.
func InOperator(key, obj Value) bool {
	switch obj.kind {
	case KindObject, KindArray, KindFunc:
		if key.kind == KindSymbol {
			return obj.object().hasSymChained(key.symbol())
		}
		return obj.HasProperty(ToString(key))
	}
	name := "a Symbol"
	if key.kind != KindSymbol {
		name = "'" + ToString(key).ToGoString() + "'"
	}
	Throw(NewTypeError(FromGoString("Cannot use 'in' operator to search for " + name + " in a non-object")))
	return false
}

// ToBoolean implements the ToBoolean abstract operation, JavaScript truthiness:
// undefined, null, false, +0, -0, NaN, and the empty string are falsy, and every
// object, every nonempty string, and every other number is truthy.
func ToBoolean(v Value) bool {
	switch v.kind {
	case KindUndefined, KindNull:
		return false
	case KindBool:
		return v.AsBool()
	case KindNumber:
		f := v.AsNumber()
		return f != 0 && !math.IsNaN(f)
	case KindBigInt:
		return !v.bigint().IsZero()
	case KindString:
		return v.str().Length() != 0
	default:
		return true
	}
}

// ToNumber implements the ToNumber abstract operation, the coercion arithmetic on
// a maybe-non-number reaches for. It follows the spec cases: undefined is NaN,
// null and false are 0, true is 1, a number is itself, and a string parses
// through the same StringToNumber the Number(s) coercion uses. An object coerces
// through its primitive first, so [1] becomes 1 and [] becomes 0, matching the
// engine.
func ToNumber(v Value) float64 {
	switch v.kind {
	case KindUndefined:
		return math.NaN()
	case KindNull:
		return 0
	case KindBool:
		return BoolToNumber(v.AsBool())
	case KindNumber:
		return v.AsNumber()
	case KindBigInt:
		// The abstract ToNumber throws on a bigint: arithmetic never silently
		// coerces a bigint to a number, so 10n * 2 is a TypeError, not 20. An
		// explicit Number(b) conversion goes through its own helper, not this path.
		Throw(NewTypeError(FromGoString("Cannot convert a BigInt value to a number")))
		return 0
	case KindString:
		return StringToNumber(v.str())
	default:
		return ToNumber(toPrimitiveNumber(v))
	}
}

// ToString implements the ToString abstract operation. undefined and null spell
// their names, a boolean and a number go through the same stringify String(x)
// uses so the two agree, a string is itself, and an object stringifies through its
// primitive: an array joins its elements with commas and a plain object is
// "[object Object]", which is what the engine prints.
func ToString(v Value) BStr {
	switch v.kind {
	case KindUndefined:
		return FromGoString("undefined")
	case KindNull:
		return FromGoString("null")
	case KindBool:
		return BoolToString(v.AsBool())
	case KindNumber:
		return NumberToString(v.AsNumber())
	case KindBigInt:
		return FromGoString(v.bigint().String())
	case KindString:
		return v.str()
	default:
		return toPrimitiveString(v)
	}
}

// JoinString converts one element the way Array.prototype.join does: undefined
// and null contribute the empty string rather than their names, so
// [1, null, 3].join() is "1,,3", and every other value goes through the abstract
// ToString. The lowerer passes this as the per-element string closure when the
// array's element type is dynamic and it cannot pick a fixed element ToString
// (NumberToString, BoolToString, or the identity for a string).
func JoinString(v Value) BStr {
	if v.IsNullish() {
		return FromGoString("")
	}
	return ToString(v)
}

// MapCallString implements the borrowed idiom Array.prototype.map.call(arrayLike,
// String): it reads the array-like's length, coerces each element through the same
// abstract ToString the String built-in applies, and returns a new array of those
// strings. The test262 assert prelude formats a failed comparison exactly this
// way, compareArray.format, so lowering the borrow is what lets the prelude reach
// the interpreter's build path rather than hand back. The length coerces the way
// ToLength does, a NaN or negative length yielding a zero count, and each element
// reads positionally through the dynamic index so a dense array maps element for
// element.
func MapCallString(arrayLike Value) *Array[Value] {
	lenF := ToNumber(arrayLike.Get(FromGoString("length")))
	n := 0
	if lenF > 0 {
		n = int(lenF)
	}
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		out[i] = StringValue(ToString(arrayLike.GetIndex(float64(i))))
	}
	return NewArray(out...)
}

// ToStringMethod implements a dynamic x.toString() call, the method each
// prototype installs rather than the abstract ToString the operators use. A
// number spells its digits, a boolean spells true or false, a string is itself,
// a bigint spells its digits, an array joins its elements, and any other object
// reports the "[object Object]" tag. undefined and null carry no prototype, so
// reading toString off them throws a TypeError the way JavaScript does. The
// result is boxed because the receiver is dynamic and the call site is typed any.
func (v Value) ToStringMethod() Value {
	switch v.kind {
	case KindUndefined:
		Throw(NewTypeError(FromGoString("Cannot read properties of undefined (reading 'toString')")))
	case KindNull:
		Throw(NewTypeError(FromGoString("Cannot read properties of null (reading 'toString')")))
	case KindString:
		return v
	case KindSymbol:
		// A symbol has no abstract ToString (that throws), but Symbol.prototype.toString
		// renders "Symbol(desc)", so the method form answers that descriptive string
		// rather than routing through ToString the way the other kinds do.
		return StringValue(v.SymbolDescriptiveString())
	}
	return StringValue(ToString(v))
}

// ValueOfMethod implements a dynamic x.valueOf() call, the method each prototype
// installs. Object.prototype.valueOf returns the receiver itself, and the primitive
// wrappers return the primitive they box, so for every kind that carries a prototype
// the answer is the receiver value unchanged: a number, string, boolean, bigint, or
// symbol is its own primitive value, and an object, array, or function is returned by
// identity. undefined and null carry no prototype, so reading valueOf off them throws a
// TypeError the way JavaScript does. The result is boxed because the receiver is dynamic
// and the call site is typed any.
func (v Value) ValueOfMethod() Value {
	switch v.kind {
	case KindUndefined:
		Throw(NewTypeError(FromGoString("Cannot read properties of undefined (reading 'valueOf')")))
	case KindNull:
		Throw(NewTypeError(FromGoString("Cannot read properties of null (reading 'valueOf')")))
	}
	return v
}

// ClassTag implements Object.prototype.toString.call(v), the idiom test262 and
// much library code reaches for to read a value's internal class as a string of
// the form "[object Type]". The mapping is the spec's Object.prototype.toString:
// undefined and null carry their own tags, an array is "[object Array]", a
// callable is "[object Function]", and every primitive and plain object reports
// the tag for its type. It is called only where the AOT path proved the borrow is
// Object.prototype.toString.call, so the receiver kind alone decides the tag.
//
// One spec case bento does not model yet: an Error, Date, or RegExp built with the
// corresponding internal slot reports "[object Error]" and the like, which waits on
// the runtime carrying the slots. The other, an object whose Symbol.toStringTag
// property is a string, is honored here: such an object reports "[object <tag>]"
// with that string, the hook a library uses to name its own instances. A plain
// object with no such property reaches the object case and reports "[object Object]".
func ClassTag(v Value) BStr {
	switch v.kind {
	case KindUndefined:
		return FromGoString("[object Undefined]")
	case KindNull:
		return FromGoString("[object Null]")
	case KindBool:
		return FromGoString("[object Boolean]")
	case KindNumber:
		return FromGoString("[object Number]")
	case KindBigInt:
		return FromGoString("[object BigInt]")
	case KindString:
		return FromGoString("[object String]")
	case KindSymbol:
		return FromGoString("[object Symbol]")
	case KindArray:
		return FromGoString("[object Array]")
	case KindFunc:
		return FromGoString("[object Function]")
	default:
		if tag, ok := toStringTagOf(v); ok {
			return FromGoString("[object ").ConcatN(tag, FromGoString("]"))
		}
		return FromGoString("[object Object]")
	}
}

// toStringTagOf reads an object's Symbol.toStringTag property, the hook
// Object.prototype.toString honors to name an instance. It reports the tag string
// and true only when the receiver is an object carrying that well-known symbol key
// with a string value, the case the specification uses to override the default
// tag; a non-object receiver, a missing property, or a non-string tag reports
// false so ClassTag falls back to "[object Object]".
func toStringTagOf(v Value) (BStr, bool) {
	if v.kind != KindObject {
		return BStr{}, false
	}
	tag := v.getSymKey(symbolToStringTag)
	if tag.kind != KindString {
		return BStr{}, false
	}
	return tag.str(), true
}

// NamedClassTag returns the "[object <Name>]" tag Object.prototype.toString.call
// reads off a receiver whose class name the compiler knows statically but whose Go
// representation does not box into a Value the runtime ClassTag could read: a typed
// array, a Map, or a Set. The receiver is taken and discarded so the borrowed
// toString evaluates its argument the way the language does and the caller's binding
// reads as a use, while the tag comes from the compiler-known name.
func NamedClassTag(_ any, name string) BStr {
	return FromGoString("[object " + name + "]")
}

// Add implements the JavaScript + operator over two dynamic values, the one
// operator whose result kind depends on its operands: if either side becomes a
// string after ToPrimitive, the result is the concatenation, and otherwise both
// coerce to numbers and add. This is the operator the dynamic path hits when an
// any-typed expression is added to anything.
func Add(a, b Value) Value {
	pa := toPrimitiveDefault(a)
	pb := toPrimitiveDefault(b)
	if pa.kind == KindString || pb.kind == KindString {
		return StringValue(Concat(ToString(pa), ToString(pb)))
	}
	// A bigint adds to a bigint and produces a bigint. Mixing a bigint with a
	// number is a TypeError, the same rule that makes 1n + 1 throw, so + never
	// silently narrows a bigint to a double or widens a double to a bigint.
	if pa.kind == KindBigInt || pb.kind == KindBigInt {
		if pa.kind != KindBigInt || pb.kind != KindBigInt {
			Throw(NewTypeError(FromGoString("Cannot mix BigInt and other types, use explicit conversions")))
		}
		sum := &BigInt{}
		sum.i.Add(&pa.bigint().i, &pb.bigint().i)
		return BigIntValue(sum)
	}
	return Number(ToNumber(pa) + ToNumber(pb))
}

// StrictEquals implements the === operator over two dynamic values, the Strict
// Equality Comparison: different types are never equal, numbers compare as
// doubles (so NaN equals nothing and +0 equals -0, which Go's float64 == already
// does), strings compare by code unit, bigints by mathematical value, and the
// reference kinds by identity. undefined equals undefined and null equals null,
// each only itself.
func StrictEquals(a, b Value) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case KindUndefined, KindNull:
		return true
	case KindBool:
		return a.scalar == b.scalar
	case KindNumber:
		return a.AsNumber() == b.AsNumber()
	case KindString:
		return a.str().Equal(b.str())
	case KindBigInt:
		return a.bigint().i.Cmp(&b.bigint().i) == 0
	default:
		// Symbols, objects, arrays, and functions compare by identity.
		return a.ref == b.ref
	}
}

// Or implements the value-returning a || b over dynamic values: the left operand
// when it is truthy, the right otherwise. Both arguments arrive evaluated, so the
// lowering only takes this form when the right operand has no side effect to
// short-circuit away; a right operand with an effect keeps its hand-back until
// the lazy form lands.
func Or(a, b Value) Value {
	if ToBoolean(a) {
		return a
	}
	return b
}

// And implements the value-returning a && b over dynamic values: the left operand
// when it is falsy, the right otherwise. The same eager-argument caveat as Or
// applies, so the lowering gates on an effect-free right operand.
func And(a, b Value) Value {
	if ToBoolean(a) {
		return b
	}
	return a
}

// Coalesce implements the value-returning a ?? b over dynamic values: the left
// operand when it is neither null nor undefined and the right otherwise. Unlike
// Or it tests presence, not truthiness, so a zero or an empty string on the left
// is kept. The same eager-argument caveat as Or applies, so the lowering gates on
// an effect-free right operand.
func Coalesce(a, b Value) Value {
	if a.IsNullish() {
		return b
	}
	return a
}

// primHint is the ToPrimitive hint: the preferred type a coercion asks an object
// to become, which selects the order valueOf and toString are tried and the string
// an exotic Symbol.toPrimitive method receives.
type primHint uint8

const (
	hintDefault primHint = iota
	hintNumber
	hintString
)

// hintName spells a hint the way the spec passes it to a Symbol.toPrimitive
// method: the string argument the exotic coercion reads to decide what to return.
func hintName(hint primHint) Value {
	switch hint {
	case hintNumber:
		return StringValue(FromGoString("number"))
	case hintString:
		return StringValue(FromGoString("string"))
	default:
		return StringValue(FromGoString("default"))
	}
}

// toPrimitive applies the ToPrimitive abstract operation at the given hint. A
// value that is already primitive returns unchanged. An object is first asked for
// its Symbol.toPrimitive method: when present it is called with the hint's name and
// a primitive result is taken, while an object result throws the TypeError the spec
// raises for a coercion that will not converge. With no such method the object runs
// OrdinaryToPrimitive, reading valueOf and toString in the hint's order and taking
// the first that returns a primitive when called with the object as this. When
// neither yields a primitive the value falls back to its ordinary string form, the
// "[object Object]" or comma-joined-array spelling the default prototype methods
// produce, so a plain dynamic object with no user coercion behaves exactly as it
// did before this path looked for one.
func toPrimitive(v Value, hint primHint) Value {
	if !isObjectLike(v) {
		return v
	}
	exotic := v.getSymKey(symbolToPrimitive)
	if !exotic.IsNullish() {
		if exotic.kind != KindFunc {
			Throw(NewTypeError(FromGoString("Symbol.toPrimitive is not a function")))
			return Undefined
		}
		res := exotic.Call(v, hintName(hint))
		if isObjectLike(res) {
			Throw(NewTypeError(FromGoString("Cannot convert object to primitive value")))
			return Undefined
		}
		return res
	}
	if res, ok := ordinaryToPrimitive(v, hint == hintString); ok {
		return res
	}
	return StringValue(ordinaryToString(v))
}

// ordinaryToPrimitive is the OrdinaryToPrimitive abstract operation: it reads
// valueOf and toString off the object, in the order the hint asks (toString first
// for a string hint, valueOf first otherwise), calls the first callable one with
// the object as its this receiver, and returns its result the moment it comes back
// primitive. A method that is absent or not callable is skipped, and a method that
// returns another object is rejected so the other method still gets its turn. The
// second return reports whether any method produced a primitive, so the caller can
// fall back to the ordinary string form when neither did.
func ordinaryToPrimitive(v Value, stringFirst bool) (Value, bool) {
	names := [2]BStr{FromGoString("valueOf"), FromGoString("toString")}
	if stringFirst {
		names = [2]BStr{FromGoString("toString"), FromGoString("valueOf")}
	}
	for _, name := range names {
		m := v.Get(name)
		if m.kind != KindFunc {
			continue
		}
		res := m.Call(v)
		if !isObjectLike(res) {
			return res, true
		}
	}
	return Undefined, false
}

func toPrimitiveDefault(v Value) Value { return toPrimitive(v, hintDefault) }
func toPrimitiveNumber(v Value) Value  { return toPrimitive(v, hintNumber) }
func toPrimitiveString(v Value) BStr   { return ToString(toPrimitive(v, hintString)) }

// ordinaryToString spells an object the way the default Object.prototype.toString
// and Array.prototype.toString do: an array is its elements joined by commas with
// null and undefined rendered empty, and any other object is the "[object Object]"
// tag.
func ordinaryToString(v Value) BStr {
	if v.kind != KindArray {
		return FromGoString("[object Object]")
	}
	o := v.object()
	var b []uint16
	for i, e := range o.elems {
		if i > 0 {
			b = append(b, ',')
		}
		if e.kind == KindNull || e.kind == KindUndefined || e.kind == KindHole {
			continue
		}
		b = ToString(e).appendUnits(b)
	}
	return FromUTF16(b)
}

// arrayIndex reports whether a property name is a canonical array index (a
// nonnegative integer with no leading zeros or sign) and returns it. Only such a
// name reads through the dense element storage; anything else is a named property.
func arrayIndex(name string) (int, bool) {
	if name == "" {
		return 0, false
	}
	if name == "0" {
		return 0, true
	}
	if name[0] < '1' || name[0] > '9' {
		return 0, false
	}
	n := 0
	for i := 0; i < len(name); i++ {
		c := name[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}
