package value

import (
	"bytes"
	"math"
	"unsafe"
)

// This file is a port of node's lib/internal/util/comparisons.js, the deep equality
// that answers util.isDeepStrictEqual and, in a later slice, assert.deepStrictEqual
// and assert.deepEqual. The reason to port rather than to write a recursive compare
// is that almost none of the interesting behavior is in the recursion: it is in which
// keys count, which tag mismatch rejects a pair before its keys are looked at, and how
// a cycle is decided. Each of those has a rule no one would guess, and every one of
// them is what a program that uses assert.deepStrictEqual depends on.
//
// Three of node's modes are here. kLoose and kStrict are the two a program asks for,
// and kStrictWithoutPrototypes is the strict comparison with the constructor check
// dropped, which assert uses when it compares two errors it built itself. kPartial,
// the mode behind assert.partialDeepStrictEqual, is not: it is a mode of its own on
// top of these, and it is a later slice.
//
// The branches for the kinds that do not box into a Value are left out rather than
// written and left unreachable: Date, the typed arrays, ArrayBuffer, DataView,
// Promise, WeakMap, WeakSet, a boxed primitive, a URL, and a crypto key. What
// survives is every branch a boxed value can reach: an array, a plain object, a
// regexp, an error, a Set, a Map, and the tag comparisons that keep those apart. The
// Set and Map walks live in nodedeepcollections.go. As the rest of the boxing wall
// falls, the missing branches come back one kind at a time, and the shape they slot
// into is node's, unchanged.
//
// One divergence is deliberate. Node runs the whole comparison once with no cycle
// tracking at all and catches the stack overflow a cyclic value raises, then retries
// with the memo set, because tracking costs a Set insertion per object pair and a
// cycle is rare. Go cannot recover from a stack overflow, so the memo set is carried
// from the start. The set is allocated on first use, so a comparison of two
// primitives or two flat objects still allocates nothing.

// deepMode is the comparison mode, node's kLoose, kStrict and
// kStrictWithoutPrototypes. It decides three things: whether two primitives compare
// with == or with ===, whether the pair must share a constructor, and whether a
// symbol-keyed property is part of the comparison at all.
type deepMode int

const (
	deepLoose deepMode = iota
	deepStrict
	deepStrictNoProto
)

// deepIter is node's iteration type, which says what a pair carries beyond its named
// properties: nothing, an array's elements, a set's members, or a map's entries.
type deepIter int

const (
	deepIterNone deepIter = iota
	deepIterArray
	deepIterSet
	deepIterMap
)

// deepMemos holds the object pairs the comparison is already inside, which is how a
// cyclic value terminates: a pair reached for the second time is taken to be equal,
// since the only way to disprove it would be to walk the cycle again. Node keeps the
// first two pairs in named fields and materializes a Set on the third, an
// optimization for the common shallow case; one set from the start answers the same,
// because node seeds its set with exactly those first two pairs.
type deepMemos struct {
	seen map[unsafe.Pointer]struct{}
}

// DeepStrictEqual reports whether two values are deeply strictly equal, the
// comparison behind util.isDeepStrictEqual and assert.deepStrictEqual. Strictly
// means every primitive compares with ===, with NaN equal to itself and 0 not equal
// to -0, and two objects must carry the same prototype as well as the same
// properties.
func DeepStrictEqual(a, b Value) bool {
	return deepInnerEqual(a, b, deepStrict, &deepMemos{})
}

// DeepEqual reports whether two values are deeply loosely equal, the comparison
// behind assert.deepEqual. Loosely means a primitive compares with ==, so 1 and "1"
// are equal, a null element matches an undefined one, and neither the prototype nor a
// symbol-keyed property is looked at.
func DeepEqual(a, b Value) bool {
	return deepInnerEqual(a, b, deepLoose, &deepMemos{})
}

// DeepStrictEqualSkipPrototype is the strict comparison with the constructor check
// dropped, node's kStrictWithoutPrototypes. A program reaches it through assert's
// skipPrototype option, and assert itself uses it to compare two errors it built, so
// an error it raised from one place matches one raised from another. Everything else
// is the strict comparison: a primitive still compares with ===, and a symbol-keyed
// property still counts.
func DeepStrictEqualSkipPrototype(a, b Value) bool {
	return deepInnerEqual(a, b, deepStrictNoProto, &deepMemos{})
}

// NodeIsDeepStrictEqual is util.isDeepStrictEqual called with its own argument list.
// It is variadic for the reason the other util entry points are: the lowerer emits
// one boxed argument per source argument, and a call with a missing argument compares
// against undefined rather than failing to compile.
func NodeIsDeepStrictEqual(args ...Value) bool {
	return DeepStrictEqual(Arg(args, 0), Arg(args, 1))
}

// deepInnerEqual is node's innerDeepEqual, the entry every level of the comparison
// goes through. It settles the primitives itself and hands a pair of objects to
// deepObjectComparisonStart.
func deepInnerEqual(a, b Value, mode deepMode, memos *deepMemos) bool {
	// All identical values are equivalent, as determined by ===, with one exception:
	// 0 === -0 holds, and they are not the same value, so a strict comparison has to
	// look at the sign of the zero it just proved equal.
	if StrictEquals(a, b) {
		if a.kind == KindNumber && a.AsNumber() == 0 && mode != deepLoose {
			return math.Signbit(a.AsNumber()) == math.Signbit(b.AsNumber())
		}
		return true
	}

	if mode != deepLoose {
		if a.kind == KindNumber {
			// A number that is not === to the other side can only still be equal by both
			// being NaN, which is the one value that is deeply equal to itself and not
			// strictly equal to itself.
			return math.IsNaN(a.AsNumber()) && b.kind == KindNumber && math.IsNaN(b.AsNumber())
		}
		if !deepIsObject(a) || !deepIsObject(b) {
			return false
		}
	} else {
		if !deepIsObject(a) {
			return !deepIsObject(b) && (LooseEquals(a, b) || deepBothNaN(a, b))
		}
		if !deepIsObject(b) {
			return false
		}
	}
	return deepObjectComparisonStart(a, b, mode, memos)
}

// deepIsObject is node's `val !== null && typeof val === 'object'`, the test that
// decides whether a value is compared by its contents or by ===. A function is not
// one of them, since typeof reports "function", so two distinct functions are never
// deeply equal however identical their properties are. That is node's answer and not
// a gap here, and it holds for a proxy over a function too, which is callable and so
// reports "function" as well.
func deepIsObject(v Value) bool {
	return v.kind == KindObject || v.kind == KindArray
}

// deepBothNaN is node's `val1 !== val1 && val2 !== val2` in the loose branch, where
// == has already said no. Loose equality is the only comparison NaN escapes through
// this way, since == and === agree that it matches nothing.
func deepBothNaN(a, b Value) bool {
	return a.kind == KindNumber && math.IsNaN(a.AsNumber()) &&
		b.kind == KindNumber && math.IsNaN(b.AsNumber())
}

// deepObjectComparisonStart is node's objectComparisonStart, the branch that decides
// what kind of thing the pair is before their properties are compared at all. The
// order of the branches is load-bearing: the array test comes before the tag tests
// because an array is compared as its elements, and the plain-object test comes
// before every remaining kind because that is the case worth reaching first.
func deepObjectComparisonStart(a, b Value, mode deepMode, memos *deepMemos) bool {
	if mode == deepStrict && !deepSameConstructor(a, b) {
		return false
	}

	if IsArray(a) {
		if !IsArray(b) || deepArrayLen(a) != deepArrayLen(b) || deepHasUnequalTag(a, b) {
			return false
		}
		// An array's named properties are compared as keys and its elements as elements,
		// so the key list is the own enumerable properties that are not indices. Both
		// sides are counted here because the key walk below only walks the second one.
		keys2 := deepNonIndexKeys(b, mode)
		if len(keys2) != len(deepNonIndexKeys(a, mode)) {
			return false
		}
		return deepKeyCheck(a, b, mode, memos, deepIterArray, keys2, true)
	}

	// The tag is what keeps two objects of different kinds apart before anything reads
	// their properties, which is why an error and a plain object carrying a name and a
	// message are not deeply equal.
	tag1 := deepClassTag(a)
	if deepToStringTag(a).kind == KindUndefined && tag1 == "[object Object]" {
		if deepSlowHasUnequalTag(tag1, a, b) {
			return false
		}
		return deepKeyCheck(a, b, mode, memos, deepIterNone, nil, false)
	}

	switch {
	case a.asRegExp() != nil:
		if b.asRegExp() == nil || !deepSimilarRegExps(a.asRegExp(), b.asRegExp()) || deepHasUnequalTag(a, b) {
			return false
		}

	case deepSlowHasUnequalTag(tag1, a, b) || IsArray(b):
		return false

	case a.asDate() != nil:
		// A date is its instant: two dates are equal when they name the same moment,
		// whatever properties either of them was given afterwards, which the key walk
		// below still compares. The tag test above has already rejected a date against
		// anything that names itself differently, but a plain object can carry the Date
		// toStringTag, so the second value is still asked whether it is really a date.
		if b.asDate() == nil || !dateSameInstant(a.asDate(), b.asDate()) {
			return false
		}

	case b.asDate() != nil:
		return false

	case a.asBuffer() != nil:
		// A buffer is its bytes: two buffers are equal when they hold the same run,
		// whatever else was done to either of them, and a detached one holds none. The tag
		// test above has already kept an ArrayBuffer and a SharedArrayBuffer apart, since
		// each names itself, so this only ever compares two of a kind.
		if b.asBuffer() == nil || !bytes.Equal(a.asBuffer().jsBufferBytes(), b.asBuffer().jsBufferBytes()) {
			return false
		}

	case b.asBuffer() != nil:
		return false

	case a.asDataView() != nil:
		// A view is compared by the bytes it can see rather than by the buffer under it or
		// where in that buffer its window starts, so two views onto different buffers at
		// different offsets are equal when they read the same run.
		if b.asDataView() == nil || !bytes.Equal(dataViewWindow(a.asDataView()), dataViewWindow(b.asDataView())) {
			return false
		}

	case b.asDataView() != nil:
		return false

	case a.asSet() != nil:
		// Two sets of different sizes cannot match however their members compare, which
		// is the one cheap test before the pairwise hunt setEquiv does. The tag test
		// above has already rejected a set against anything that names itself
		// differently, but a plain object can carry the Set toStringTag, so the second
		// value is still asked whether it is really a set.
		if b.asSet() == nil || a.asSet().jsSize() != b.asSet().jsSize() {
			return false
		}
		return deepKeyCheck(a, b, mode, memos, deepIterSet, nil, false)

	case a.asMap() != nil:
		if b.asMap() == nil || a.asMap().jsSize() != b.asMap().jsSize() {
			return false
		}
		return deepKeyCheck(a, b, mode, memos, deepIterMap, nil, false)

	case b.asSet() != nil || b.asMap() != nil:
		return false

	case a.object().err != nil:
		// The stack is not compared: two errors thrown from two places are the same
		// error as far as a test is concerned, and node leaves the stack out for exactly
		// that reason. What is compared is the four properties an error carries, each of
		// them only when the second error does not carry it as an enumerable own
		// property, since an enumerable one is compared by the key walk below anyway.
		if b.object().err == nil ||
			!deepEnumerableOrIdentical(a, b, "message", mode, memos) ||
			!deepEnumerableOrIdentical(a, b, "name", mode, memos) ||
			!deepEnumerableOrIdentical(a, b, "cause", mode, memos) ||
			!deepEnumerableOrIdentical(a, b, "errors", mode, memos) {
			return false
		}
		// A cause of undefined and no cause at all are different errors, which the value
		// comparison above cannot tell apart.
		if deepHasOwn(a, deepStrKey("cause")) != deepHasOwn(b, deepStrKey("cause")) {
			return false
		}

	case b.object().err != nil:
		return false
	}

	return deepKeyCheck(a, b, mode, memos, deepIterNone, nil, false)
}

// deepSameConstructor is the strict mode's prototype check. Node compares the two
// constructors when the first value's constructor is a built-in one or is inherited,
// and falls back to comparing the prototypes themselves; the built-in half of that
// test is not portable, because bento has no Array or Object constructor object for a
// value to point at, so the inherited-constructor half decides it. A class instance
// therefore compares against another instance by its constructor, and a plain object
// against a plain object by its prototype.
func deepSameConstructor(a, b Value) bool {
	ctor := a.Get(FromGoString("constructor"))
	if ctor.kind != KindUndefined && !deepHasOwn(a, deepStrKey("constructor")) {
		return StrictEquals(ctor, b.Get(FromGoString("constructor")))
	}
	return deepSamePrototype(a, b)
}

// deepSamePrototype compares two objects' [[Prototype]] slots. It reads the slots
// rather than calling GetPrototype because GetPrototype reports null both for the
// ordinary prototype, which bento models as no pointer at all, and for the null one
// Object.create(null) gives: node says {} and Object.create(null) are not deeply
// strictly equal, and the flattened form cannot see the difference. The cost is that a
// proxy's getPrototypeOf trap is not consulted, since the slot read is the proxy
// object's own.
func deepSamePrototype(a, b Value) bool {
	oa, ob := a.object(), b.object()
	return oa.proto == ob.proto && oa.protoNull == ob.protoNull
}

// deepClassTag is Object.prototype.toString for the comparison. It is ClassTag plus
// the two brands ClassTag does not model yet: bento stores an error and a regexp as an
// object carrying a marker rather than with the internal slot the spec reads, so
// ClassTag calls both "[object Object]" and the comparison would take an error for a
// plain object and then find a plain object with the same name and message deeply
// equal to it.
func deepClassTag(v Value) string {
	if v.kind == KindObject && v.object().proxy == nil {
		if v.object().err != nil {
			return "[object Error]"
		}
		if v.asRegExp() != nil {
			return "[object RegExp]"
		}
	}
	return ClassTag(v).ToGoString()
}

// deepToStringTag reads a value's Symbol.toStringTag, the hook a class uses to name
// its own instances. The read climbs the prototype chain the way any property read
// does, since a class that names its instances puts the tag on the prototype.
func deepToStringTag(v Value) Value {
	return v.GetElem(SymbolToStringTag())
}

// deepHasUnequalTag is node's hasUnequalTag: two values of a kind the comparison has
// already matched can still be different if one of them renamed itself.
func deepHasUnequalTag(a, b Value) bool {
	return !StrictEquals(deepToStringTag(a), deepToStringTag(b))
}

// deepSlowHasUnequalTag is node's slowHasUnequalTag, the tag comparison for the case
// where the first value's tag has already been computed. Two values that both name
// themselves are compared by those names; otherwise the already-computed tag is
// compared against the second value's.
func deepSlowHasUnequalTag(tag1 string, a, b Value) bool {
	ta, tb := deepToStringTag(a), deepToStringTag(b)
	if ta.kind != KindUndefined && tb.kind != KindUndefined {
		return !StrictEquals(ta, tb)
	}
	return tag1 != deepClassTag(b)
}

// deepSimilarRegExps is node's areSimilarRegExps. A regexp is its pattern, its flags
// and its position: lastIndex is part of the comparison because a global regexp that
// has matched once is not interchangeable with a fresh one.
func deepSimilarRegExps(a, b *RegExp) bool {
	return a.Source().Equal(b.Source()) &&
		a.Flags().Equal(b.Flags()) &&
		a.LastIndex() == b.LastIndex()
}

// deepEnumerableOrIdentical is node's isEnumerableOrIdentical, the test the error
// branch uses for each of the four error properties. An enumerable own property on
// the second value is left to the key walk, which compares it against the first
// value's; anything else is compared here, which is how a message that is own and
// non-enumerable on both sides gets compared at all.
func deepEnumerableOrIdentical(a, b Value, prop string, mode deepMode, memos *deepMemos) bool {
	k := deepStrKey(prop)
	return deepHasEnumerable(b, k) || deepInnerEqual(deepGet(a, k), deepGet(b, k), mode, memos)
}

// deepKeyCheck is node's keyCheck. For every remaining pair, equality is having the
// same number of own enumerable properties, the same set of keys whatever their
// order, an equal value under each of them, and, for an array, equal elements.
//
// keysGiven says the caller already built the key list, which the array branch does
// because an array's index properties are compared as elements instead. Node spells
// that as keys2 being passed at all; the flag is the same test written so an array
// with no named properties cannot be mistaken for a value whose keys were not built.
func deepKeyCheck(a, b Value, mode deepMode, memos *deepMemos, iter deepIter, keys2 []inspectKey, keysGiven bool) bool {
	var keys1 []inspectKey
	if !keysGiven {
		keys2 = deepOwnEnumKeys(b)
		keys1 = deepOwnEnumKeys(a)
		// The pair must have the same number of own properties, which is the cheap half
		// of comparing the two key sets: the walk below proves the keys are the same
		// ones, and this proves neither side has any the other lacks.
		if len(keys1) != len(keys2) {
			return false
		}
		if mode == deepStrict || mode == deepStrictNoProto {
			keys1 = append(keys1, deepOwnEnumSymbols(a)...)
			keys2 = append(keys2, deepOwnEnumSymbols(b)...)
			if len(keys1) != len(keys2) {
				return false
			}
		}
	}

	if len(keys2) == 0 && (iter == deepIterNone || (iter == deepIterArray && deepArrayLen(b) == 0)) {
		return true
	}
	return deepHandleCycles(a, b, mode, keys1, keys2, memos, iter)
}

// deepHandleCycles is node's handleCycles: it records the pair, compares them, and
// takes the pair back out. A pair that is already recorded is a cycle and is taken to
// be equal; a value recorded against a different partner is not, which is what keeps
// two differently shaped cycles apart.
func deepHandleCycles(a, b Value, mode deepMode, keys1, keys2 []inspectKey, memos *deepMemos, iter deepIter) bool {
	if memos.seen == nil {
		memos.seen = map[unsafe.Pointer]struct{}{}
	}
	size := len(memos.seen)
	memos.seen[a.ref] = struct{}{}
	memos.seen[b.ref] = struct{}{}
	if size != len(memos.seen)-2 {
		// At least one of the two was already inside the comparison. Both being inside
		// it is the cycle that closes, and only one being inside it is a cycle on one
		// side that the other side does not have.
		return size == len(memos.seen)
	}

	equal := deepObjEquiv(a, b, mode, keys1, keys2, memos, iter)

	delete(memos.seen, a.ref)
	delete(memos.seen, b.ref)
	return equal
}

// deepObjEquiv is node's objEquiv, the walk that compares the values behind the keys
// and then, for an array, the elements. The key walk has two halves: while the two key
// lists agree position by position, which is the ordinary case for two objects built
// the same way, a key is read straight off both sides; once they diverge, each
// remaining key is looked up on the first value as a descriptor, so a key the second
// value has and the first one only inherits fails there.
func deepObjEquiv(a, b Value, mode deepMode, keys1, keys2 []inspectKey, memos *deepMemos, iter deepIter) bool {
	if len(keys2) > 0 {
		i := 0
		// Ordered keys.
		if keys1 != nil {
			for ; i < len(keys2); i++ {
				k := keys2[i]
				if !deepSameKey(keys1[i], k) {
					break
				}
				if !deepInnerEqual(deepGet(a, k), deepGet(b, k), mode, memos) {
					return false
				}
			}
		}
		// Unordered keys.
		for ; i < len(keys2); i++ {
			k := keys2[i]
			val, ok := deepOwnEnumValue(a, k)
			if !ok {
				return false
			}
			if !deepInnerEqual(val, deepGet(b, k), mode, memos) {
				return false
			}
		}
	}

	if iter == deepIterSet {
		return deepSetEquiv(a.asSet(), b.asSet(), mode, memos)
	}
	if iter == deepIterMap {
		return deepMapEquiv(a.asMap(), b.asMap(), mode, memos)
	}

	if iter == deepIterArray {
		for i, n := 0, deepArrayLen(a); i < n; i++ {
			bi := deepIndex(b, i)
			if bi.kind == KindUndefined {
				if !deepHasIndex(b, i) {
					// The second array has a hole here, so from this point on the two are
					// compared as the sparse arrays they are: by their present indices.
					return deepSparseArrayEquiv(a, b, mode, memos, i)
				}
				ai := deepIndex(a, i)
				if (ai.kind != KindUndefined || !deepHasIndex(a, i)) && (mode != deepLoose || ai.kind != KindNull) {
					return false
				}
			} else if ai := deepIndex(a, i); (ai.kind == KindUndefined || !deepInnerEqual(ai, bi, mode, memos)) &&
				(mode != deepLoose || bi.kind != KindNull) {
				return false
			}
		}
	}

	return true
}

// deepSparseArrayEquiv is node's sparseArrayEquiv, the comparison two arrays fall
// into once a hole is found in one of them. A hole is not an element with an undefined
// value: it is the absence of a property, so the two arrays are compared as the key
// sets they are, and a hole on one side against a written undefined on the other is a
// difference.
func deepSparseArrayEquiv(a, b Value, mode deepMode, memos *deepMemos, i int) bool {
	keysA := deepOwnEnumKeys(a)
	keysB := deepOwnEnumKeys(b)
	if len(keysA) != len(keysB) {
		return false
	}
	for ; i < len(keysB); i++ {
		k := keysB[i]
		av := deepGet(a, k)
		if (av.kind == KindUndefined && !deepHasOwn(a, k)) || !deepInnerEqual(av, deepGet(b, k), mode, memos) {
			return false
		}
	}
	return true
}

// deepStrKey builds a string key for the walk. The key type is inspectKey, the
// string-or-symbol pair the inspector's key walk already needed, since deep equality
// walks the same mixed list of own keys.
func deepStrKey(name string) inspectKey {
	return inspectKey{str: FromGoString(name)}
}

// deepKeyValue is a key as the boxed property key a dynamic read takes, which is how
// a symbol key reaches a read or a descriptor lookup without being coerced to a string.
func deepKeyValue(k inspectKey) Value {
	if k.sym != nil {
		return symbolValue(k.sym)
	}
	return StringValue(k.str)
}

// deepSameKey reports whether two keys name the same property. A symbol is compared
// by identity, since two symbols with the same description are two properties.
func deepSameKey(a, b inspectKey) bool {
	if a.sym != nil || b.sym != nil {
		return a.sym == b.sym
	}
	return a.str.Equal(b.str)
}

// deepGet reads a value's property under a key, node's val[key]. It goes through the
// ordinary read, so an accessor runs and a proxy's get trap answers.
func deepGet(v Value, k inspectKey) Value {
	if k.sym != nil {
		return v.GetElem(symbolValue(k.sym))
	}
	return v.Get(k.str)
}

// deepIndex reads an array element, node's val[i].
func deepIndex(v Value, i int) Value {
	return v.GetIndex(float64(i))
}

// deepHasIndex reports whether an array carries an index as an own property, which is
// what tells a written undefined from a hole.
func deepHasIndex(v Value, i int) bool {
	return deepHasOwn(v, inspectKey{str: NumberToString(float64(i))})
}

// deepHasOwn is node's ObjectPrototypeHasOwnProperty. It asks for the descriptor
// rather than probing the property bag directly, which is what makes it answer for an
// array's elements, report a hole as absent, and route through a proxy's trap.
func deepHasOwn(v Value, k inspectKey) bool {
	return v.GetOwnPropertyDescriptor(deepKeyValue(k)).kind != KindUndefined
}

// deepHasEnumerable is node's ObjectPrototypePropertyIsEnumerable: own and
// enumerable, the pair of conditions that decides whether a property is one the key
// walk will compare. It asks about the property without reading it, so a value whose
// message is an accessor is not called here only to find out that the key walk will
// call it.
func deepHasEnumerable(v Value, k inspectKey) bool {
	if v.asProxy() != nil {
		desc := v.GetOwnPropertyDescriptor(deepKeyValue(k))
		return desc.kind != KindUndefined && ToBoolean(desc.Get(FromGoString("enumerable")))
	}
	desc, ok := inspectOwnDesc(v.object(), k)
	return ok && desc.enumerable
}

// deepOwnEnumValue reads the value behind an own enumerable property and reports
// whether the property is one. It is node's descriptor read in objEquiv, which takes
// the value out of a data descriptor and calls the getter of an accessor: a data
// property is read without running any code, and an accessor is read the way the
// language reads it.
func deepOwnEnumValue(v Value, k inspectKey) (Value, bool) {
	if v.asProxy() != nil {
		// A proxy's only descriptor is the one its trap returns, so the answer comes from
		// the boxed descriptor object rather than from storage the proxy does not have.
		desc := v.GetOwnPropertyDescriptor(deepKeyValue(k))
		if desc.kind == KindUndefined || !ToBoolean(desc.Get(FromGoString("enumerable"))) {
			return Undefined, false
		}
		if desc.Get(FromGoString("writable")).kind == KindUndefined {
			return deepGet(v, k), true
		}
		return desc.Get(FromGoString("value")), true
	}
	desc, ok := inspectOwnDesc(v.object(), k)
	if !ok || !desc.enumerable {
		return Undefined, false
	}
	if desc.accessor {
		return deepGet(v, k), true
	}
	return desc.value, true
}

// deepOwnEnumKeys is node's ObjectKeys: the own enumerable string keys in the order
// the language enumerates them, which for an array includes its indices.
func deepOwnEnumKeys(v Value) []inspectKey {
	names := v.OwnEnumerableKeys().Elems()
	keys := make([]inspectKey, 0, len(names))
	for _, n := range names {
		keys = append(keys, inspectKey{str: n})
	}
	return keys
}

// deepOwnEnumSymbols is the own enumerable symbol keys, which the strict modes count
// as properties and the loose one ignores. A proxy reports none of them: its symbol
// keys are its target's, and the ownKeys trap that would answer for them is not read
// here.
func deepOwnEnumSymbols(v Value) []inspectKey {
	o := v.object()
	var keys []inspectKey
	for i, s := range o.symKeys {
		if o.symDescs[i].enumerable {
			keys = append(keys, inspectKey{sym: s})
		}
	}
	return keys
}

// deepNonIndexKeys is node's getOwnNonIndexProperties with the ONLY_ENUMERABLE
// filter: an array's own enumerable properties minus the indices, which are compared
// as elements. An array's length is not one of them, since it is not enumerable.
func deepNonIndexKeys(v Value, mode deepMode) []inspectKey {
	var keys []inspectKey
	for _, n := range v.OwnEnumerableKeys().Elems() {
		if _, isIndex := arrayIndex(n.ToGoString()); isIndex {
			continue
		}
		keys = append(keys, inspectKey{str: n})
	}
	if mode != deepLoose {
		keys = append(keys, deepOwnEnumSymbols(v)...)
	}
	return keys
}

// deepArrayLen is an array's length. It reads the element slice directly for a real
// array and goes through the property read for a proxy over one, whose length is
// whatever its trap answers.
func deepArrayLen(v Value) int {
	if v.kind == KindArray {
		return len(v.object().elems)
	}
	return int(ToNumber(v.Get(FromGoString("length"))))
}
