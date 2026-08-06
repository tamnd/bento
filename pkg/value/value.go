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
// exotic-array brand the spec tests rather than a duck-typed length probe. A Proxy
// is an array when its target is: the spec's IsArray reads through the
// [[ProxyTarget]] slot rather than the proxy's own kind, so a proxy over an array,
// or a proxy over such a proxy, brands as an array. A revoked proxy has no target
// to read and throws a TypeError, the way IsArray rejects it.
func IsArray(v Value) bool {
	if v.Kind() == KindArray {
		return true
	}
	if p := v.asProxy(); p != nil {
		p.checkRevoked("isArray")
		return IsArray(p.target)
	}
	return false
}

// StaticBool returns result and ignores operand, the lowering of a call whose
// answer the checker already knows at compile time but whose operand must still be
// evaluated and referenced. Array.isArray(x) on a statically typed x folds to true
// or false, yet dropping x would discard its side effects (Array.isArray(f()) must
// still call f) and leave a Go binding that x was the only use of unreferenced, so
// the emit passes x through here to keep it live while yielding the known result.
func StaticBool[T any](_ T, result bool) bool { return result }

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
		// A boxed collection answers the two symbol-keyed members a Map and a Set carry,
		// the default iterator and the toStringTag Object.prototype.toString reads, off
		// the live collection. They are answered here rather than installed as own symbol
		// properties so the key walks that back console.log and Object.keys stay empty,
		// the way a real Map's and Set's do.
		if m := v.object().jsMap; m != nil {
			if val, ok := mapSymGet(m, key); ok {
				return val
			}
		}
		if s := v.object().jsSet; s != nil {
			if val, ok := setSymGet(s, key); ok {
				return val
			}
		}
		// A boxed date answers Symbol.toPrimitive, the hook that makes it the one built-in
		// whose default coercion is its string form rather than its number.
		if d := v.object().jsDate; d != nil {
			if val, ok := dateSymGet(d, key); ok {
				return val
			}
		}
		// A byte buffer and a data view each name themselves through Symbol.toStringTag,
		// which they carry for real rather than through an internal slot, so a plain read
		// of it answers "ArrayBuffer" the way node's does.
		if b := v.object().jsBuffer; b != nil {
			if val, ok := bufferSymGet(b, key); ok {
				return val
			}
		}
		if v.object().jsView != nil {
			if val, ok := dataViewSymGet(key); ok {
				return val
			}
		}
		// A typed array names itself the same way, and unlike a buffer it also answers the
		// default iterator, so a spread or a for...of over a boxed one walks its elements.
		if t := v.object().jsTyped; t != nil {
			if val, ok := typedArraySymGet(t, key); ok {
				return val
			}
		}
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

// SetIndexStrict is the strict-mode form of SetIndex, the numeric bracket write
// a[i] = val the lowerer emits under a "use strict" program. It routes through the
// throwing string store so a write blocked by a frozen or non-extensible receiver
// raises the TypeError a strict element assignment raises instead of dropping.
func (v Value) SetIndexStrict(i float64, val Value) Value {
	return v.SetKeyStrict(NumberToString(i), val)
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

// setSymKeyStrict is the strict-mode form of setSymKey, the symbol branch of a
// dynamic bracket write o[s] = val under a "use strict" program. Where setSymKey
// silently drops a write to a frozen or getter-only symbol property or a new key on
// a non-extensible object, setSymKeyStrict throws the TypeError a strict assignment
// raises, the same escalation SetStrict applies to a named store.
func (v Value) setSymKeyStrict(key *Symbol, val Value) Value {
	if p := v.asProxy(); p != nil {
		p.setSym(v, key, val)
		return val
	}
	switch v.kind {
	case KindObject, KindArray, KindFunc:
		v.object().setSymStrict(v, key, val)
	}
	return val
}

// SetElemStrict is the strict-mode form of SetElem, the dynamic bracket write
// o[key] = val the lowerer emits in place of SetElem under a "use strict" program.
// A symbol key routes through the throwing symbol store, a string key through the
// throwing named store, and any other key is coerced to a property key the same way
// before the throwing named store, so a strict computed write drops nothing silently
// the way its sloppy counterpart would.
func (v Value) SetElemStrict(key, val Value) Value {
	if key.kind == KindSymbol {
		return v.setSymKeyStrict(key.symbol(), val)
	}
	if key.kind == KindString {
		v.SetStrict(key.str(), val)
		return val
	}
	v.SetStrict(ToString(key), val)
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

// DeleteIndexStrict is the strict-mode form of DeleteIndex: a refused element
// removal (a sealed array slot or a non-configurable numeric property) throws
// rather than reporting false, routed through DeleteStrict like the named path.
func (v Value) DeleteIndexStrict(i float64) bool {
	return v.DeleteStrict(NumberToString(i))
}

// DeleteElemStrict is the strict-mode form of DeleteElem: a symbol key refused by
// a non-configurable property throws, and every other key routes through
// DeleteStrict so a refused string or coerced key throws too. A successful removal
// returns true unchanged, so an ordinary strict delete matches the sloppy one.
func (v Value) DeleteElemStrict(key Value) bool {
	if key.kind == KindSymbol {
		return v.deleteSymKeyStrict(key.symbol())
	}
	if key.kind == KindString {
		return v.DeleteStrict(key.str())
	}
	return v.DeleteStrict(ToString(key))
}

// deleteSymKeyStrict is the strict-mode form of deleteSymKey. A removal refused by
// a non-configurable symbol property is a TypeError in a strict program, the
// symbol sibling of DeleteStrict's throw; a successful or absent removal returns
// true unchanged.
func (v Value) deleteSymKeyStrict(key *Symbol) bool {
	if v.deleteSymKey(key) {
		return true
	}
	Throw(NewTypeError(FromGoString("Cannot delete property '" + symKeyLabel(key) + "' of #<Object>")))
	return false
}

// deleteSymKey removes a symbol-keyed property from an object, array, or function
// receiver, the symbol branch of delete o[s]. A non-configurable symbol property
// (a sealed or frozen object's key) refuses removal and reports false, while a
// configurable or absent property reports true and a primitive receiver has
// nothing to remove, matching what delete yields.
func (v Value) deleteSymKey(key *Symbol) bool {
	switch v.kind {
	case KindUndefined, KindNull:
		// delete base[sym] coerces base through ToObject first, which throws on a
		// nullish base, so the symbol branch throws the same TypeError the string
		// branch does rather than report a boolean.
		Throw(NewTypeError(FromGoString("Cannot convert undefined or null to object")))
	}
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

// OptionalMember is a?.b on a boxed receiver: undefined when the receiver is null or
// undefined, and the ordinary property read otherwise. The short circuit is the whole
// point of the optional chain, and a box is the one receiver that can carry either
// answer without the lowerer having to prove which, so the question is asked here at
// run time. A longer chain composes by feeding this call's result back in as the next
// receiver, which is what makes a?.b?.c stop at the first nullish link.
func OptionalMember(v Value, key BStr) Value {
	if v.IsNullish() {
		return Undefined
	}
	return v.Get(key)
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
		if re := v.object().regexp; re != nil {
			// A boxed regexp answers its own accessors, .source and .flags and the flag
			// booleans and .lastIndex, from the live regexp rather than an empty property
			// bag, so a dynamic read off an any-typed regexp reads the same value the
			// concrete accessor would. A name that is not a regexp own property climbs the
			// ordinary chain and ends at undefined.
			if val, ok := regexpGet(re, name); ok {
				return val
			}
		}
		if m := v.object().jsMap; m != nil {
			// A boxed Map answers its own members, .size and the methods, off the live map
			// rather than an empty property bag, so a dynamic read reaches the same entries
			// the typed path holds. A name that is not a Map member climbs the ordinary
			// chain and ends at undefined, which is what such a read does in JavaScript.
			if val, ok := mapGet(m, name); ok {
				return val
			}
		}
		if s := v.object().jsSet; s != nil {
			if val, ok := setGet(s, name); ok {
				return val
			}
		}
		if w := v.object().jsWeak; w != nil {
			// A boxed weak collection answers its own methods off the live one, so a dynamic
			// wm.get(o) reads the entry the typed wm.get(o) reads. There is no .size here,
			// unlike the two above: a weak collection has none, so the read climbs the chain
			// and ends at undefined the way it does in JavaScript.
			if val, ok := w.jsWeakMember(name); ok {
				return val
			}
		}
		if d := v.object().jsDate; d != nil {
			// A boxed date answers its own methods, the getters and the setters and the
			// formats, off the live date rather than an empty property bag, so a dynamic
			// d.getTime() reads the same instant the typed path holds and a dynamic setter
			// moves it. A name that is not a Date member climbs the ordinary chain and ends
			// at undefined, which is what such a read does in JavaScript.
			if val, ok := dateGet(d, name); ok {
				return val
			}
		}
		if b := v.object().jsBuffer; b != nil {
			// A boxed buffer answers its own accessors and methods off the live bytes, so a
			// dynamic b.byteLength reads what the typed side holds and a dynamic b.resize
			// reshapes the storage every view over it aliases.
			if val, ok := bufferGet(b, name); ok {
				return val
			}
		}
		if d := v.object().jsView; d != nil {
			if val, ok := dataViewGet(d, name); ok {
				return val
			}
		}
		if t := v.object().jsTyped; t != nil {
			// A boxed typed array answers an index off the live elements as well as its
			// geometry and its methods, so a dynamic a[1] reads the same slot the typed path
			// writes and a dynamic a.map walks the same run.
			if val, ok := typedArrayGet(t, name); ok {
				return val
			}
		}
		if inst := v.object().jsClass; inst != nil && !v.object().hasOwn(key) {
			// A boxed instance answers toString and valueOf, the two the language calls on
			// its own, by running the class's own code when it writes one. Without this a
			// box carries an instance's fields and its class name and none of its methods,
			// so String([q]) would print a class tag where the program has an answer. A
			// class that writes neither reports nothing here and falls through to the chain
			// walk, which ends at Object.prototype and answers both. The own-property check
			// comes first
			// because a field named toString is a real own property and shadows the method,
			// the way it does on a live instance.
			if val, ok := classCoercionGet(inst, name); ok {
				return val
			}
		}
		return v.object().getChained(v, key)
	case KindFunc:
		// A function is an object too, so a named read finds its own properties: the
		// name a built-in error constructor carries is the read the caught-error tests
		// make. A function box with no own properties still climbs its prototype chain
		// and answers undefined for a miss at the end of it.
		return v.object().getChained(v, key)
	case KindNumber:
		// A number's prototype members are reachable on the dynamic path too, so
		// n.toString(16) and n.toFixed(2) work off a receiver the checker typed any. The
		// statically typed receiver never gets here: calls.go lowers those two to the same
		// formatters this delegates to (primitivemember.go).
		if val, ok := numberGet(v.AsNumber(), name); ok {
			return val
		}
		return Undefined
	case KindBool:
		if val, ok := boolGet(v.AsBool(), name); ok {
			return val
		}
		return Undefined
	case KindBigInt:
		if val, ok := bigIntGet(v, name); ok {
			return val
		}
		return Undefined
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
		return o.hasChained(v, key)
	case KindObject, KindFunc:
		// A boxed collection carries its members the way a real Map and Set carry theirs
		// on their prototype, so `'size' in map` and `'add' in set` answer true, which
		// the property read above already agrees with. A boxed date carries its own the
		// same way, so `'getTime' in d` is true and `'size' in d` is false.
		if m := v.object().jsMap; m != nil {
			if _, ok := mapGet(m, name); ok {
				return true
			}
		}
		if s := v.object().jsSet; s != nil {
			if _, ok := setGet(s, name); ok {
				return true
			}
		}
		if w := v.object().jsWeak; w != nil {
			if _, ok := w.jsWeakMember(name); ok {
				return true
			}
		}
		if d := v.object().jsDate; d != nil {
			if _, ok := dateGet(d, name); ok {
				return true
			}
		}
		if b := v.object().jsBuffer; b != nil {
			if _, ok := bufferGet(b, name); ok {
				return true
			}
		}
		if d := v.object().jsView; d != nil {
			if _, ok := dataViewGet(d, name); ok {
				return true
			}
		}
		if t := v.object().jsTyped; t != nil {
			if typedArrayHas(t, name) {
				return true
			}
		}
		if inst := v.object().jsClass; inst != nil && !v.object().hasOwn(key) {
			if _, ok := classCoercionGet(inst, name); ok {
				return true
			}
		}
		return v.object().hasChained(v, key)
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
	case KindSymbol:
		// Abstract ToString (spec 7.1.17 step 4) throws on a Symbol rather than
		// producing text: a symbol carries no string form, so a bare String
		// coercion of one is a TypeError. Only the String built-in and
		// Symbol.prototype.toString render "Symbol(desc)", and both take the
		// SymbolDescriptiveString path in ToStringMethod, not this one. Without
		// this case a symbol fell to toPrimitiveString, which returns the symbol
		// unchanged (a symbol is not object-like) and re-enters ToString forever.
		Throw(NewTypeError(FromGoString("Cannot convert a Symbol value to a string")))
		return BStr{}
	default:
		return toPrimitiveString(v)
	}
}

// PlusToString coerces an operand of the + operator's string-concatenation branch:
// ToPrimitive with the default hint (the hint + passes, per the AdditionOperator
// spec) and then ToString on the resulting primitive. It differs from ToString on
// exactly one kind, an object whose valueOf, toString, or Symbol.toPrimitive reads
// the hint: + must ask for "default", where a plain ToString asks for "string", so
// { valueOf: () => 42, toString: () => "s" } concatenates as "42", not "s". Every
// primitive is already primitive, so ToPrimitive returns it unchanged and this
// matches ToString for a number, string, boolean, bigint, null, or undefined; a
// symbol still throws in the trailing ToString the way "" + Symbol() does.
func PlusToString(v Value) BStr {
	return ToString(toPrimitiveDefault(v))
}

// StringCoerce implements the String built-in called as a function, String(v),
// which differs from abstract ToString on exactly one kind: a symbol renders as
// its descriptive string "Symbol(desc)" (SymbolDescriptiveString) rather than
// throwing, the special case the String constructor makes for a symbol argument.
// Every other value coerces through the ordinary ToString, so a template
// substitution or a bare concatenation (which take ToString directly) still throws
// on a symbol while String(sym) reports its description.
func StringCoerce(v Value) BStr {
	if v.kind == KindSymbol {
		return v.SymbolDescriptiveString()
	}
	return ToString(v)
}

// CoerceThisToString runs the two opening steps every String.prototype method
// shares before it touches its receiver: RequireObjectCoercible(this value) then
// ToString(O) (for example 22.1.3.3 steps 1-2). A null or undefined this throws a
// TypeError before any coercion, so String.prototype.codePointAt.call(null) raises
// the way Node does rather than stringifying the receiver to "null"/"undefined" and
// running the method on that text. A non-nullish receiver coerces through the
// ordinary ToString, so a number, boolean, or object this stringifies exactly as a
// direct string-method call would.
func CoerceThisToString(v Value) BStr {
	if v.IsNullish() {
		Throw(NewTypeError(FromGoString("String.prototype method called on null or undefined")))
		return BStr{}
	}
	return ToString(v)
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

// IndexRest gathers the receiver's elements from index from to the end into a fresh
// boxed array, the tail an array destructuring rest binds: `const [a, ...rest] = xs`
// fills rest with everything past the fixed slots. It reads the length through the
// dynamic length property coerced the way ToLength does, a NaN or negative length
// yielding a zero count, and reads each position through the dynamic index, so a boxed
// array yields its dense tail and an array-like yields its indexed tail. A from at or
// past the end yields an empty array, matching JavaScript's rest of a short source.
// The result is boxed because the source is dynamic and the rest target is typed any[].
func (v Value) IndexRest(from float64) Value {
	lenF := ToNumber(v.Get(FromGoString("length")))
	n := 0
	if lenF > 0 {
		n = int(lenF)
	}
	start := 0
	if from > 0 {
		start = int(from)
	}
	if start > n {
		start = n
	}
	out := make([]Value, 0, n-start)
	for i := start; i < n; i++ {
		out = append(out, v.GetIndex(float64(i)))
	}
	return NewArrayValue(out)
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
	case KindObject:
		// A boxed class instance runs the toString its class writes, and answers exactly
		// what that method returns rather than a string: a class whose toString answers a
		// number answers a number here, since this is the method call and not the abstract
		// coercion, which is the one place the two part. An own field of that name shadows
		// the method the way it does on a live instance.
		key := FromGoString("toString")
		if inst := v.object().jsClass; inst != nil && !v.object().hasOwn(key) {
			if m, ok := classCoercionGet(inst, "toString"); ok {
				return m.Call(v)
			}
		}
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
//
// A boxed class instance is the one object that does not answer by identity: a class
// writing its own valueOf shadows Object.prototype's, so the call runs the class's code
// and answers the primitive it returns. An own field of that name shadows the method
// again, the way it does on a live instance, so the read is checked first.
func (v Value) ValueOfMethod() Value {
	switch v.kind {
	case KindUndefined:
		Throw(NewTypeError(FromGoString("Cannot read properties of undefined (reading 'valueOf')")))
	case KindNull:
		Throw(NewTypeError(FromGoString("Cannot read properties of null (reading 'valueOf')")))
	case KindObject:
		key := FromGoString("valueOf")
		if inst := v.object().jsClass; inst != nil && !v.object().hasOwn(key) {
			if m, ok := classCoercionGet(inst, "valueOf"); ok {
				return m.Call(v)
			}
		}
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
// An object whose Symbol.toStringTag property is a string reports "[object <tag>]"
// with that string, the hook a library uses to name its own instances, and it is read
// first because the specification has it override the internal class. A boxed Date,
// Error or RegExp reports its own name next: bento brands those on the object's
// storage rather than in the spec's internal slot, so the brand is what the tag is
// read from. A plain object with neither reaches the object case and reports
// "[object Object]".
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
		if tag, ok := brandedClassTag(v); ok {
			return tag
		}
		// A Proxy over an array carries kind KindObject, not KindArray, so the array
		// case above does not catch it, yet the spec's Object.prototype.toString brands
		// it "[object Array]" because step 4 runs IsArray, which reads through the proxy
		// target. IsArray recurses through a chain of array proxies and stays false for a
		// proxy over a plain object, so only a genuinely array-backed proxy takes the tag.
		if IsArray(v) {
			return FromGoString("[object Array]")
		}
		return FromGoString("[object Object]")
	}
}

// brandedClassTag reports the tag a value carries because of what it is rather than
// because of a property it holds: a Date, an Error and a RegExp each have their own
// "[object Type]" in the specification, read from an internal slot. bento keeps that
// slot as a brand on the object's storage, so the brand is what answers here. A proxy
// is not branded itself, so it falls through and is named by what it wraps.
func brandedClassTag(v Value) (BStr, bool) {
	if v.kind != KindObject {
		return BStr{}, false
	}
	o := v.object()
	switch {
	case o.jsDate != nil:
		return FromGoString("[object Date]"), true
	case o.err != nil:
		return FromGoString("[object Error]"), true
	case o.regexp != nil:
		return FromGoString("[object RegExp]"), true
	case o.jsWeak != nil:
		// Each of the four weakly-holding kinds brands itself, so String(wm) reads
		// "[object WeakMap]" rather than the plain-object tag, and the deep comparison
		// reaches the reference-only branch instead of walking two empty property tables.
		return FromGoString("[object " + o.jsWeak.jsWeakName() + "]"), true
	}
	return BStr{}, false
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

// SameValueZero implements the SameValueZero comparison, the key and member
// identity a Map and a Set use. It is Strict Equality with one difference: NaN is
// the same value as NaN, so a collection holds one NaN key however many times it is
// offered. The zeroes stay a single value under both, which float64 == already
// gives. Every other kind compares exactly as === does.
func SameValueZero(a, b Value) bool {
	if a.kind == KindNumber && b.kind == KindNumber {
		x, y := a.AsNumber(), b.AsNumber()
		return x == y || (x != x && y != y)
	}
	return StrictEquals(a, b)
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
	if re := v.asRegExp(); re != nil {
		// A boxed regexp has no Symbol.toPrimitive and its valueOf returns itself, so
		// OrdinaryToPrimitive would fall to its toString either way: the literal form
		// "/" + source + "/" + flags. Short-circuiting to that here makes String(box), a
		// template substitution, and "" + box all render the pattern the same way the
		// concrete RegExp.prototype.toString does.
		return StringValue(re.ToStringBStr())
	}
	if v.kind == KindObject {
		if e := v.object().err; e != nil {
			// A boxed error is the regexp case again: no Symbol.toPrimitive, a valueOf that
			// returns itself, and a toString on the prototype the box does not carry, so
			// OrdinaryToPrimitive would fall through to "[object Object]" where JavaScript
			// spells "TypeError: bad". Both hints take the same form, since Error.prototype
			// has no valueOf of its own for the number hint to find. This is what makes
			// String(err), `${err}` and "" + err read as the error in the dynamic world the
			// way they do on a typed caught error, which coerces through ToBStr.
			return StringValue(e.ToBStr())
		}
	}
	exotic := v.getSymKey(symbolToPrimitive)
	if !exotic.IsNullish() {
		if exotic.kind != KindFunc {
			Throw(NewTypeError(FromGoString("Symbol.toPrimitive is not a function")))
			return Undefined
		}
		// The exotic is invoked with the hint as its first argument, the spec's
		// Call(O, «hint»). The receiver O is the method's this, which the AOT calling
		// convention binds lexically rather than as a positional argument, so a body that
		// reads this hands back at compile time and never reaches here; passing the hint
		// alone lands it in the method's first declared parameter, where the spec puts it.
		res := exotic.Call(hintName(hint))
		if isObjectLike(res) {
			Throw(NewTypeError(FromGoString("Cannot convert object to primitive value")))
			return Undefined
		}
		return res
	}
	if res, ok := ordinaryToPrimitive(v, hint == hintString); ok {
		return res
	}
	// Neither valueOf nor toString produced a primitive. The fallback to the ordinary
	// string form models the inherited Object.prototype.toString, which a plain object
	// with no coercion of its own still has. But an object that carries an own
	// "toString" property has shadowed that inherited method: reaching here means its
	// own toString was absent-as-callable or returned an object, so the built-in is
	// unavailable and, with valueOf already exhausted, OrdinaryToPrimitive throws a
	// TypeError (7.1.1.1 step 5) rather than inventing "[object Object]". This is the
	// throw String.prototype.slice.call({toString: undefined, valueOf: undefined})
	// raises in its ToString step.
	//
	// An object whose chain ends at an explicit null throws for the other half of the
	// same reason: it never inherited the built-in at all, so String(Object.create(null))
	// raises rather than reading "[object Object]", which is what the engine does.
	if v.kind == KindObject && (v.object().hasOwn(FromGoString("toString")) || !v.object().inheritsObjectProto()) {
		Throw(NewTypeError(FromGoString("Cannot convert object to primitive value")))
		return Undefined
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
		// The object is the receiver, not the first argument. A plain Call(v) would put it
		// in the argument slot, which nothing noticed while every toString the runtime
		// boxes ignored its arguments, and which breaks the moment one does not: Buffer's
		// toString reads its first argument as an encoding, so String(buf) would coerce the
		// buffer to name its own encoding and recur until the stack ran out.
		res := CallWithThis(m, v)
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
// null and undefined rendered empty, and any other object is its class tag. The tag
// is ClassTag rather than a flat "[object Object]" because Object.prototype.toString
// honors Symbol.toStringTag, so String(new Map()) reads "[object Map]" and an object
// that names itself reads by that name, both of which the engine prints.
func ordinaryToString(v Value) BStr {
	if v.kind != KindArray {
		return ClassTag(v)
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
