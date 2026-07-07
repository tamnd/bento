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
)

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
	if o.kind == KindArray {
		return Value{kind: KindArray, ref: unsafe.Pointer(o)}
	}
	return Value{kind: KindObject, ref: unsafe.Pointer(o)}
}

// Kind reports the value's runtime tag.
func (v Value) Kind() Kind { return v.kind }

// IsUndefined and the other predicates ask the tag directly, the cheap check the
// dynamic path makes before it commits to a kind-specific operation.
func (v Value) IsUndefined() bool { return v.kind == KindUndefined }
func (v Value) IsNull() bool      { return v.kind == KindNull }
func (v Value) IsNullish() bool   { return v.kind == KindUndefined || v.kind == KindNull }

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

// Get implements a dynamic property read, o[key], for the kinds the AOT path
// produces. A string reports its length and indexes to a one-character string; an
// array reports its length and indexes into its elements; an object looks the key
// up in its property map. A read that finds nothing is undefined, the JavaScript
// result for a missing property, so the caller never faults. The other kinds have
// no own properties the dynamic path reads yet and return undefined too.
func (v Value) Get(key BStr) Value {
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
			if idx < len(o.elems) {
				return o.elems[idx]
			}
			return Undefined
		}
		return o.getOwn(key)
	case KindObject:
		return v.object().getOwn(key)
	case KindFunc:
		// A function is an object too, so a named read finds its own properties: the
		// name a built-in error constructor carries is the read the caught-error tests
		// make. A function box with no own properties still answers undefined for a
		// miss through the same getOwn scan.
		return v.object().getOwn(key)
	default:
		return Undefined
	}
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

// ClassTag implements Object.prototype.toString.call(v), the idiom test262 and
// much library code reaches for to read a value's internal class as a string of
// the form "[object Type]". The mapping is the spec's Object.prototype.toString:
// undefined and null carry their own tags, an array is "[object Array]", a
// callable is "[object Function]", and every primitive and plain object reports
// the tag for its type. It is called only where the AOT path proved the borrow is
// Object.prototype.toString.call, so the receiver kind alone decides the tag.
//
// Two spec cases bento does not model yet: an object with a Symbol.toStringTag
// property reports that tag instead of "[object Object]", and an Error, Date, or
// RegExp built with the corresponding internal slot reports "[object Error]" and
// the like. Those wait on the runtime carrying the slots; a plain object reaches
// the object case and reports "[object Object]".
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
		return FromGoString("[object Object]")
	}
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

// toPrimitiveDefault, toPrimitiveNumber, and toPrimitiveString apply the
// ToPrimitive abstract operation at the three hints. A primitive is already
// primitive and returns unchanged. An object has no user valueOf or toString on
// the dynamic path yet, so it takes the default ordinary result: an array joins
// its elements and any other object is "[object Object]", which is what the
// default valueOf/toString pair produces for a plain object. The number and
// default hints agree here because neither object case has a numeric valueOf.
func toPrimitiveDefault(v Value) Value {
	if v.kind != KindObject && v.kind != KindArray && v.kind != KindFunc {
		return v
	}
	return StringValue(ordinaryToString(v))
}

func toPrimitiveNumber(v Value) Value { return toPrimitiveDefault(v) }
func toPrimitiveString(v Value) BStr  { return ordinaryToString(v) }

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
		if e.kind == KindNull || e.kind == KindUndefined {
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
