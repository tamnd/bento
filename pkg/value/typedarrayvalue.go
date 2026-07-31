// This file bridges the typed-array family into the dynamic value.Value world: the
// eight numeric kinds bento stores as a TypedArray[T], the Uint8Array it keeps apart so
// it can hand its bytes to Go, and the two bigint kinds it stores as a BigIntArray[T].
// It is the same seam mapvalue.go, datevalue.go and buffervalue.go open, a brand field
// on Object plus a member switch, with one thing none of those have: elements.
//
// A typed array is the one boxed built-in whose own property table is not empty. Its
// indices are real own properties in JavaScript, so Object.keys of one lists them, the
// JSON walk writes them as an index object, and a bracket read or write reaches them.
// Everything below that touches an index goes through the backing's element accessors
// rather than through storage of the box's own, so a write through the box lands in the
// buffer every other view of it reads.
//
// The member surface is written once against the backing interface rather than per kind.
// A typed array's methods all reduce to reading an element, writing one, and allocating
// or viewing another array of the same kind, so the interface exposes those three and
// the whole prototype falls out of them. That is also what gives the Uint8Array and the
// two bigint kinds the methods their runtime structs never grew: the box does not
// delegate to a method on the struct, it does the work over the elements.

package value

import (
	"math"
	"sort"
	"strconv"
)

// typedArrayBacking is what every member of the family answers, the small set the whole
// prototype is written against. jsTypedViewOf is the one that carries the kind: it
// builds another array of the same kind over a buffer, which is how slice allocates a
// copy and subarray takes a window without either of them naming a concrete type.
type typedArrayBacking interface {
	// jsTypedName is the constructor name, which is also the Symbol.toStringTag and the
	// prefix the rendering carries.
	jsTypedName() string
	// jsTypedLen is the element count as of this access, zero for a view whose buffer has
	// been detached or shrunk out from under it.
	jsTypedLen() int
	// jsTypedAt reads one element as the value JavaScript hands out, a Number for the
	// numeric kinds and a BigInt for the two 64-bit ones. An index out of range is the
	// caller's mistake; every caller here clamps first.
	jsTypedAt(i int) Value
	// jsTypedSet writes one element, applying the kind's own store coercion, so a write of
	// 300 into an Int8Array wraps and one into a Uint8ClampedArray clamps.
	jsTypedSet(i int, v Value)
	// jsTypedBuffer and jsTypedByteOffset are the view's geometry, the two the .buffer and
	// .byteOffset accessors answer and the pair subarray needs to place its window.
	jsTypedBuffer() *ArrayBuffer
	jsTypedByteOffset() int
	// jsTypedElemBytes is BYTES_PER_ELEMENT, which the geometry arithmetic also needs.
	jsTypedElemBytes() int
	// jsTypedViewOf builds another array of this kind over the given buffer. It is the one
	// place the concrete type is named, so slice, subarray, map, filter and the rest stay
	// generic over the family.
	jsTypedViewOf(buf *ArrayBuffer, byteOffset, length int) typedArrayBacking
	// jsTypedBox is this array's own box, the value a member that answers the receiver
	// hands back and the third argument every callback is given.
	jsTypedBox() Value
}

// ToValue boxes a numeric typed array into a dynamic value. The box is built once and
// kept on the array, so every crossing hands back the same object: a typed array is a
// view over a buffer, and two boxes of one view would compare unequal under === while
// writing into the same bytes.
func (a *TypedArray[T]) ToValue() Value {
	if a.boxed == nil {
		a.boxed = &Object{kind: KindObject, jsTyped: a}
	}
	return objectValue(a.boxed)
}

// ToValue is the Uint8Array half of the same crossing.
func (a *Uint8Array) ToValue() Value {
	if a.boxed == nil {
		a.boxed = &Object{kind: KindObject, jsTyped: a}
	}
	return objectValue(a.boxed)
}

// ToValue is the bigint half of the same crossing.
func (a *BigIntArray[T]) ToValue() Value {
	if a.boxed == nil {
		a.boxed = &Object{kind: KindObject, jsTyped: a}
	}
	return objectValue(a.boxed)
}

// The backing implementations. Each is three lines of geometry, two element accessors
// and the view constructor that names the kind.

func (a *TypedArray[T]) jsTypedName() string         { return typedArrayKindName[T]() }
func (a *TypedArray[T]) jsTypedLen() int             { return a.liveLen() }
func (a *TypedArray[T]) jsTypedAt(i int) Value       { return Number(float64(a.live()[i])) }
func (a *TypedArray[T]) jsTypedSet(i int, v Value)   { a.live()[i] = a.coerce(ToNumber(v)) }
func (a *TypedArray[T]) jsTypedBuffer() *ArrayBuffer { return a.buffer }
func (a *TypedArray[T]) jsTypedByteOffset() int      { return a.byteOffset }
func (a *TypedArray[T]) jsTypedElemBytes() int       { return elemBytes[T]() }
func (a *TypedArray[T]) jsTypedBox() Value           { return a.ToValue() }

func (a *TypedArray[T]) jsTypedViewOf(buf *ArrayBuffer, byteOffset, length int) typedArrayBacking {
	return newTypedArrayView(buf, byteOffset, length, a.coerce)
}

func (a *Uint8Array) jsTypedName() string         { return "Uint8Array" }
func (a *Uint8Array) jsTypedLen() int             { return a.liveLen() }
func (a *Uint8Array) jsTypedAt(i int) Value       { return Number(float64(a.live()[i])) }
func (a *Uint8Array) jsTypedSet(i int, v Value)   { a.live()[i] = toUint8(ToNumber(v)) }
func (a *Uint8Array) jsTypedBuffer() *ArrayBuffer { return a.buffer }
func (a *Uint8Array) jsTypedByteOffset() int      { return a.byteOffset }
func (a *Uint8Array) jsTypedElemBytes() int       { return 1 }
func (a *Uint8Array) jsTypedBox() Value           { return a.ToValue() }

func (a *Uint8Array) jsTypedViewOf(buf *ArrayBuffer, byteOffset, length int) typedArrayBacking {
	return &Uint8Array{buffer: buf, byteOffset: byteOffset, length: length}
}

func (a *BigIntArray[T]) jsTypedName() string         { return bigIntArrayKindName[T]() }
func (a *BigIntArray[T]) jsTypedLen() int             { return a.liveLen() }
func (a *BigIntArray[T]) jsTypedAt(i int) Value       { return BigIntFromBig(a.load(a.live()[i])) }
func (a *BigIntArray[T]) jsTypedBuffer() *ArrayBuffer { return a.buffer }
func (a *BigIntArray[T]) jsTypedByteOffset() int      { return a.byteOffset }
func (a *BigIntArray[T]) jsTypedElemBytes() int       { return 8 }
func (a *BigIntArray[T]) jsTypedBox() Value           { return a.ToValue() }

// jsTypedSet on a bigint array runs its value through ToBigInt rather than ToNumber. A
// 64-bit slot is exactly where a silent float64 rounding would corrupt data, so the
// language makes a Number here an error rather than a conversion.
func (a *BigIntArray[T]) jsTypedSet(i int, v Value) {
	a.live()[i] = a.store(dataViewBigArg(v))
}

func (a *BigIntArray[T]) jsTypedViewOf(buf *ArrayBuffer, byteOffset, length int) typedArrayBacking {
	return newBigIntArrayView(buf, byteOffset, length, a.store, a.load)
}

// typedArrayKindName names a numeric typed array from its Go element type. The eight
// kinds are one to one with the eight element types because Uint8Array, which would
// otherwise collide with Uint8ClampedArray on uint8, is a separate struct with its own
// storage. Deriving the name here rather than carrying it in the struct keeps the eight
// constructor pairs untouched.
func typedArrayKindName[T typedElem]() string {
	var zero T
	switch any(zero).(type) {
	case int8:
		return "Int8Array"
	case uint8:
		return "Uint8ClampedArray"
	case int16:
		return "Int16Array"
	case uint16:
		return "Uint16Array"
	case int32:
		return "Int32Array"
	case uint32:
		return "Uint32Array"
	case float32:
		return "Float32Array"
	}
	return "Float64Array"
}

// bigIntArrayKindName names a bigint typed array from its Go element type, the same
// one-to-one the numeric kinds have between int64 and uint64.
func bigIntArrayKindName[T bigArrayElem]() string {
	var zero T
	if _, ok := any(zero).(int64); ok {
		return "BigInt64Array"
	}
	return "BigUint64Array"
}

// asTypedArray returns the live array a value boxes, or nil when the value is not a
// typed-array box. It is the probe the reads, the key walk, the inspector, the JSON walk
// and the deep comparison make before their ordinary object handling.
func (v Value) asTypedArray() typedArrayBacking {
	if v.kind != KindObject {
		return nil
	}
	return v.object().jsTyped
}

// typedElems reads the whole array out as boxed values, the snapshot every member that
// walks the elements more than once works from. Reading once matters: a callback can
// detach the buffer under the array, and a snapshot leaves the walk looking at what the
// array held when it started rather than at a run that has gone away mid-loop.
func typedElems(a typedArrayBacking) []Value {
	n := a.jsTypedLen()
	out := make([]Value, n)
	for i := range out {
		out[i] = a.jsTypedAt(i)
	}
	return out
}

// typedArrayBytes is the run of bytes the view can see right now, the window the deep
// comparison holds two arrays against. It is empty for a view whose buffer has been
// detached or shrunk out from under it, which is the same run such a view reads.
func typedArrayBytes(a typedArrayBacking) []byte {
	off := a.jsTypedByteOffset()
	end := off + a.jsTypedLen()*a.jsTypedElemBytes()
	data := a.jsTypedBuffer().data
	if off > len(data) || end > len(data) {
		return nil
	}
	return data[off:end]
}

// typedFrom builds a fresh array of the same kind holding the given elements, the
// allocation map, filter, slice and the two non-mutating sorts each end with.
func typedFrom(a typedArrayBacking, elems []Value) typedArrayBacking {
	out := a.jsTypedViewOf(NewArrayBuffer(float64(len(elems)*a.jsTypedElemBytes())), 0, len(elems))
	for i, e := range elems {
		out.jsTypedSet(i, e)
	}
	return out
}

// typedArrayGet reads a member off a boxed typed array: an index, one of the five
// geometry accessors, or one of the prototype methods. An index is tried first because
// it is the common read and because a canonical index is never a method name.
func typedArrayGet(a typedArrayBacking, name string) (Value, bool) {
	if i, ok := arrayIndex(name); ok {
		if i >= a.jsTypedLen() {
			// An index past the end of a typed array reads undefined and never climbs the
			// prototype chain, which is what makes it different from an ordinary array's
			// hole: the index is an integer-indexed slot, not a property that might be
			// inherited.
			return Undefined, true
		}
		return a.jsTypedAt(i), true
	}
	switch name {
	case "length":
		return Number(float64(a.jsTypedLen())), true
	case "byteLength":
		return Number(float64(a.jsTypedLen() * a.jsTypedElemBytes())), true
	case "byteOffset":
		return Number(float64(a.jsTypedByteOffset())), true
	case "buffer":
		return a.jsTypedBuffer().ToValue(), true
	case "BYTES_PER_ELEMENT":
		return Number(float64(a.jsTypedElemBytes())), true
	}
	if fn, ok := typedArrayMethod(a, name); ok {
		return fn, true
	}
	return Undefined, false
}

// typedArrayHas answers the in operator over a boxed typed array. It differs from the
// read in one place: an index past the end reads undefined but is not a property, so
// `5 in new Int32Array(3)` is false where `a[5]` is undefined.
func typedArrayHas(a typedArrayBacking, name string) bool {
	if i, ok := arrayIndex(name); ok {
		return i < a.jsTypedLen()
	}
	_, found := typedArrayGet(a, name)
	return found
}

// typedArrayMethod builds one bound prototype method. The whole prototype lives here so
// each member is written once for the family rather than once per kind, and every one of
// them reaches the elements through the backing, so a Uint8Array gets the same map and
// filter a Float64Array does even though neither struct carries such a method.
func typedArrayMethod(a typedArrayBacking, name string) (Value, bool) {
	switch name {
	case "at":
		return boundMethod("at", func(args []Value) Value {
			i, ok := typedExactIndex(ToNumber(Arg(args, 0)), a.jsTypedLen())
			if !ok {
				return Undefined
			}
			return a.jsTypedAt(i)
		}), true

	case "indexOf":
		return boundMethod("indexOf", func(args []Value) Value {
			return Number(typedSearch(a, Arg(args, 0), args, false))
		}), true

	case "lastIndexOf":
		return boundMethod("lastIndexOf", func(args []Value) Value {
			return Number(typedSearch(a, Arg(args, 0), args, true))
		}), true

	case "includes":
		return boundMethod("includes", func(args []Value) Value {
			// includes differs from indexOf in one place: it matches a NaN against a NaN,
			// where indexOf uses strict equality and a NaN is equal to nothing.
			target := Arg(args, 0)
			for i, e := range typedElems(a) {
				_ = i
				if StrictEquals(e, target) || (isNaNValue(e) && isNaNValue(target)) {
					return Bool(true)
				}
			}
			return Bool(false)
		}), true

	case "join":
		return boundMethod("join", func(args []Value) Value {
			sep := ","
			if len(args) > 0 && args[0].kind != KindUndefined {
				sep = ToString(args[0]).ToGoString()
			}
			return StringValue(FromGoString(typedJoin(a, sep)))
		}), true

	case "toString":
		return boundMethod("toString", func(args []Value) Value {
			return StringValue(FromGoString(typedJoin(a, ",")))
		}), true

	case "toLocaleString":
		return boundMethod("toLocaleString", func(args []Value) Value {
			return StringValue(FromGoString(typedJoin(a, ",")))
		}), true

	case "slice":
		return boundMethod("slice", func(args []Value) Value {
			start, end := typedBounds(a, args)
			return typedFrom(a, typedElems(a)[start:end]).jsTypedBox()
		}), true

	case "subarray":
		return boundMethod("subarray", func(args []Value) Value {
			// subarray is the one member that answers a view rather than a copy: the result
			// aliases the receiver's buffer, so a write through either shows in both.
			start, end := typedBounds(a, args)
			off := a.jsTypedByteOffset() + start*a.jsTypedElemBytes()
			return a.jsTypedViewOf(a.jsTypedBuffer(), off, end-start).jsTypedBox()
		}), true

	case "set":
		return boundMethod("set", func(args []Value) Value {
			typedSet(a, Arg(args, 0), ToNumber(Arg(args, 1)))
			return Undefined
		}), true

	case "fill":
		return boundMethod("fill", func(args []Value) Value {
			start, end := typedBounds(a, args[min(1, len(args)):])
			for i := start; i < end; i++ {
				a.jsTypedSet(i, Arg(args, 0))
			}
			return a.jsTypedBox()
		}), true

	case "copyWithin":
		return boundMethod("copyWithin", func(args []Value) Value {
			typedCopyWithin(a, args)
			return a.jsTypedBox()
		}), true

	case "reverse":
		return boundMethod("reverse", func(args []Value) Value {
			typedWriteBack(a, typedReversed(typedElems(a)))
			return a.jsTypedBox()
		}), true

	case "toReversed":
		return boundMethod("toReversed", func(args []Value) Value {
			return typedFrom(a, typedReversed(typedElems(a))).jsTypedBox()
		}), true

	case "sort":
		return boundMethod("sort", func(args []Value) Value {
			typedWriteBack(a, typedSorted(typedElems(a), Arg(args, 0)))
			return a.jsTypedBox()
		}), true

	case "toSorted":
		return boundMethod("toSorted", func(args []Value) Value {
			return typedFrom(a, typedSorted(typedElems(a), Arg(args, 0))).jsTypedBox()
		}), true

	case "with":
		return boundMethod("with", func(args []Value) Value {
			elems := typedElems(a)
			i, ok := typedExactIndex(ToNumber(Arg(args, 0)), len(elems))
			if !ok {
				Throw(NewRangeError(FromGoString("Invalid typed array index")))
			}
			out := typedFrom(a, elems)
			out.jsTypedSet(i, Arg(args, 1))
			return out.jsTypedBox()
		}), true

	case "forEach":
		return boundMethod("forEach", func(args []Value) Value {
			typedEachUntil(a, args, func(int, Value) bool { return false })
			return Undefined
		}), true

	case "map":
		return boundMethod("map", func(args []Value) Value {
			out := make([]Value, 0, a.jsTypedLen())
			typedEachUntil(a, args, func(_ int, r Value) bool {
				out = append(out, r)
				return false
			})
			return typedFrom(a, out).jsTypedBox()
		}), true

	case "filter":
		return boundMethod("filter", func(args []Value) Value {
			elems := typedElems(a)
			out := make([]Value, 0, len(elems))
			typedEachUntil(a, args, func(i int, r Value) bool {
				if ToBoolean(r) {
					out = append(out, elems[i])
				}
				return false
			})
			return typedFrom(a, out).jsTypedBox()
		}), true

	case "every":
		return boundMethod("every", func(args []Value) Value {
			return Bool(!typedEachUntil(a, args, func(_ int, r Value) bool { return !ToBoolean(r) }))
		}), true

	case "some":
		return boundMethod("some", func(args []Value) Value {
			return Bool(typedEachUntil(a, args, func(_ int, r Value) bool { return ToBoolean(r) }))
		}), true

	case "find", "findIndex", "findLast", "findLastIndex":
		return typedFinder(a, name), true

	case "reduce":
		return boundMethod("reduce", func(args []Value) Value {
			return typedReduce(a, args, false)
		}), true

	case "reduceRight":
		return boundMethod("reduceRight", func(args []Value) Value {
			return typedReduce(a, args, true)
		}), true

	case "keys":
		return boundMethod("keys", func(args []Value) Value { return typedIterator(a, typedIterKeys) }), true

	case "values":
		return boundMethod("values", func(args []Value) Value { return typedIterator(a, typedIterValues) }), true

	case "entries":
		return boundMethod("entries", func(args []Value) Value { return typedIterator(a, typedIterEntries) }), true
	}
	return Undefined, false
}

// typedFinder builds the four searching members, which differ only in which end they
// walk from and whether they answer the element or its index.
func typedFinder(a typedArrayBacking, name string) Value {
	fromEnd := name == "findLast" || name == "findLastIndex"
	wantIndex := name == "findIndex" || name == "findLastIndex"
	return boundMethod(name, func(args []Value) Value {
		elems := typedElems(a)
		fn := Arg(args, 0)
		box := a.jsTypedBox()
		for k := range elems {
			i := k
			if fromEnd {
				i = len(elems) - 1 - k
			}
			if ToBoolean(fn.Call(elems[i], Number(float64(i)), box)) {
				if wantIndex {
					return Number(float64(i))
				}
				return elems[i]
			}
		}
		if wantIndex {
			return Number(-1)
		}
		return Undefined
	})
}

// typedEachUntil walks the elements calling the member's callback with the three
// arguments JavaScript gives it, the element, its index and the array itself, and stops
// when the visitor says to. It reports whether it stopped early, which is what every and
// some read to answer their booleans. The callback's second argument is the reason the
// members are written here rather than delegated to the runtime struct's own map and
// filter, which pass the element alone. An optional thisArg is dropped, the same subset
// every other callback-taking member in the value model covers, since a boxed callback
// carries its own receiver.
func typedEachUntil(a typedArrayBacking, args []Value, visit func(i int, result Value) bool) bool {
	fn := Arg(args, 0)
	box := a.jsTypedBox()
	for i, e := range typedElems(a) {
		if visit(i, fn.Call(e, Number(float64(i)), box)) {
			return true
		}
	}
	return false
}

// typedReduce is the shared body of reduce and reduceRight. An absent initial value
// takes the first element walked as the accumulator and starts from the next one, and an
// empty array with no initial value is a TypeError, which is the one edge that makes
// reduce different from a fold with a zero.
func typedReduce(a typedArrayBacking, args []Value, fromEnd bool) Value {
	elems := typedElems(a)
	fn := Arg(args, 0)
	box := a.jsTypedBox()
	order := make([]int, len(elems))
	for k := range order {
		if fromEnd {
			order[k] = len(elems) - 1 - k
		} else {
			order[k] = k
		}
	}
	var acc Value
	start := 0
	if len(args) > 1 {
		acc = args[1]
	} else {
		if len(order) == 0 {
			Throw(NewTypeError(FromGoString("Reduce of empty array with no initial value")))
		}
		acc = elems[order[0]]
		start = 1
	}
	for _, i := range order[start:] {
		acc = fn.Call(acc, elems[i], Number(float64(i)), box)
	}
	return acc
}

// typedSearch is the shared body of indexOf and lastIndexOf, which compare by strict
// equality, so a NaN in the array is found by neither. An optional second argument is
// the index to start from, relative to the end when negative.
func typedSearch(a typedArrayBacking, target Value, args []Value, fromEnd bool) float64 {
	elems := typedElems(a)
	from := 0
	if fromEnd {
		from = len(elems) - 1
	}
	if len(args) > 1 {
		from = relativeSearchStart(ToNumber(args[1]), len(elems), fromEnd)
	}
	for k := range elems {
		i := from + k
		if fromEnd {
			i = from - k
		}
		if i < 0 || i >= len(elems) {
			break
		}
		if StrictEquals(elems[i], target) {
			return float64(i)
		}
	}
	return -1
}

// relativeSearchStart resolves the optional start index indexOf and lastIndexOf take. A
// negative one counts from the end, and one past either end clamps to the nearest valid
// start so the walk simply finds nothing rather than running off the array.
func relativeSearchStart(from float64, n int, fromEnd bool) int {
	i := int(math.Trunc(from))
	if from != from {
		i = 0
	}
	if i < 0 {
		i += n
	}
	if i < 0 {
		if fromEnd {
			return -1
		}
		return 0
	}
	if i >= n {
		if fromEnd {
			return n - 1
		}
		return n
	}
	return i
}

// typedExactIndex resolves the single index at and with take. It differs from the slice
// bound beside it in that it does not clamp: an index past either end is not a position
// in the array at all, so at answers undefined for one and with throws. A negative index
// counts back from the end, which is the only thing the two rules share.
// The arithmetic stays in float space until the range test has passed, since an int
// conversion of a value outside the int range is implementation-defined and would fold
// a huge index back to something inside the array.
func typedExactIndex(v float64, n int) (int, bool) {
	if v != v {
		v = 0
	}
	v = math.Trunc(v)
	if v < 0 {
		v += float64(n)
	}
	if v < 0 || v >= float64(n) {
		return 0, false
	}
	return int(v), true
}

// typedBounds resolves the optional start and end a slice, a subarray and a fill take,
// the same relative-index rule Array.prototype.slice uses, with an end before the start
// yielding an empty range rather than a negative one.
func typedBounds(a typedArrayBacking, args []Value) (int, int) {
	n := a.jsTypedLen()
	start := 0
	if len(args) > 0 && args[0].kind != KindUndefined {
		start = relativeIndex(ToNumber(args[0]), n)
	}
	end := n
	if len(args) > 1 && args[1].kind != KindUndefined {
		end = relativeIndex(ToNumber(args[1]), n)
	}
	if end < start {
		end = start
	}
	return start, end
}

// typedSet writes a source array's elements into the receiver at an offset, the
// set(source, offset) member. The source is anything iterable or index-shaped, which is
// what lets a program fill a typed array from a plain array. A source that would run
// past the end is a RangeError, the throw the spec raises rather than a silent truncation
// that would leave half a write behind.
func typedSet(a typedArrayBacking, src Value, offset float64) {
	at := 0
	if offset == offset && offset > 0 {
		at = int(math.Trunc(offset))
	}
	var elems []Value
	if s := src.asTypedArray(); s != nil {
		elems = typedElems(s)
	} else {
		n := int(ToNumber(src.Get(FromGoString("length"))))
		for i := 0; i < n; i++ {
			elems = append(elems, src.Get(FromGoString(strconv.Itoa(i))))
		}
	}
	if at+len(elems) > a.jsTypedLen() {
		Throw(NewRangeError(FromGoString("offset is out of bounds")))
	}
	for i, e := range elems {
		a.jsTypedSet(at+i, e)
	}
}

// typedCopyWithin copies a run of the array onto itself, the copyWithin member. The
// elements are read out first so an overlapping copy takes the values the array held
// before the write started, which is what makes copyWithin a move rather than a smear.
func typedCopyWithin(a typedArrayBacking, args []Value) {
	elems := typedElems(a)
	n := len(elems)
	target := 0
	if len(args) > 0 {
		target = relativeIndex(ToNumber(args[0]), n)
	}
	start, end := typedBounds(a, args[min(1, len(args)):])
	for i := 0; i < end-start && target+i < n; i++ {
		a.jsTypedSet(target+i, elems[start+i])
	}
}

// typedWriteBack stores a walk's result back into the receiver, the last step of the two
// members that reorder in place.
func typedWriteBack(a typedArrayBacking, elems []Value) {
	for i, e := range elems {
		a.jsTypedSet(i, e)
	}
}

// typedReversed is the reversal both reverse and toReversed share, taken over the
// snapshot so neither reads an element it has already overwritten.
func typedReversed(elems []Value) []Value {
	out := make([]Value, len(elems))
	for i, e := range elems {
		out[len(elems)-1-i] = e
	}
	return out
}

// typedSorted sorts a snapshot, either with the comparator the call gave or, with none,
// in ascending numeric order. The numeric default is what makes a typed array's sort
// different from an ordinary array's, which compares string forms and so puts 10 before
// 9. The sort is stable, which the spec requires of both.
func typedSorted(elems []Value, cmp Value) []Value {
	out := make([]Value, len(elems))
	copy(out, elems)
	if cmp.kind == KindFunc {
		sort.SliceStable(out, func(i, j int) bool {
			return ToNumber(cmp.Call(out[i], out[j])) < 0
		})
		return out
	}
	sort.SliceStable(out, func(i, j int) bool { return typedLess(out[i], out[j]) })
	return out
}

// typedLess is the default ordering, ascending by value with the two edges the spec
// pins: a NaN sorts to the end, and a negative zero sorts before a positive one. A
// bigint element compares as a bigint, since its value does not survive a float64.
func typedLess(a, b Value) bool {
	if a.kind == KindBigInt && b.kind == KindBigInt {
		return a.bigint().Int().Cmp(b.bigint().Int()) < 0
	}
	x, y := ToNumber(a), ToNumber(b)
	if x != x {
		return false
	}
	if y != y {
		return true
	}
	if x == y {
		return math.Signbit(x) && !math.Signbit(y)
	}
	return x < y
}

// typedJoin renders the elements separated by the given string, the body join, toString
// and the string coercion share. An element is rendered the way String() renders it,
// which for a bigint element is its digits with no n suffix.
func typedJoin(a typedArrayBacking, sep string) string {
	out := ""
	for i, e := range typedElems(a) {
		if i > 0 {
			out += sep
		}
		out += ToString(e).ToGoString()
	}
	return out
}

// isNaNValue reports whether a value is the Number NaN, the one element includes matches
// against itself where indexOf does not.
func isNaNValue(v Value) bool {
	return v.kind == KindNumber && v.AsNumber() != v.AsNumber()
}

// The three iteration modes keys, values and entries walk in.
const (
	typedIterKeys = iota
	typedIterValues
	typedIterEntries
)

// typedIterator builds the iterator keys(), values(), entries() and a for...of over a
// boxed typed array walk. It snapshots the elements for the reason a boxed set's iterator
// does: a body that resizes or detaches the buffer would otherwise leave the walk reading
// a run that has gone away.
func typedIterator(a typedArrayBacking, mode int) Value {
	elems := typedElems(a)
	i := 0
	return iterObject(func() IterResult {
		if i >= len(elems) {
			return IterResult{Value: Undefined, Done: true}
		}
		k, e := Number(float64(i)), elems[i]
		i++
		switch mode {
		case typedIterKeys:
			return IterResult{Value: k}
		case typedIterEntries:
			return IterResult{Value: NewArrayValue([]Value{k, e})}
		}
		return IterResult{Value: e}
	})
}

// typedArraySymGet reads a symbol-keyed member off a boxed typed array. Like a buffer's
// and unlike a Date's, the toStringTag is a real property in JavaScript, carried on the
// prototype, so both a plain read of it and Object.prototype.toString answer the kind's
// name.
func typedArraySymGet(a typedArrayBacking, key *Symbol) (Value, bool) {
	switch key {
	case symbolIterator:
		return boundMethod("[Symbol.iterator]", func(args []Value) Value {
			return typedIterator(a, typedIterValues)
		}), true
	case symbolToStringTag:
		return StringValue(FromGoString(a.jsTypedName())), true
	}
	return Undefined, false
}

// typedArraySet writes a member on a boxed typed array and reports whether it took the
// write. Only an index writes: a typed array's integer-indexed slots are its own
// properties, and a name that is not one is dropped rather than added, which is what an
// engine does with `a.foo = 1` on a typed array in sloppy mode.
func typedArraySet(a typedArrayBacking, name string, val Value) bool {
	i, ok := arrayIndex(name)
	if !ok {
		return false
	}
	// An index past the end is not an error and does not grow the array: a typed array's
	// length is its view's, so the write simply has nowhere to land and is dropped.
	if i < a.jsTypedLen() {
		a.jsTypedSet(i, val)
	}
	return true
}

// typedArrayKeys is the array's own property names, its indices as decimal strings. It
// is the one boxed built-in whose key walk is not empty, which is what makes
// Object.keys, Object.values and the JSON walk answer the elements rather than nothing.
func typedArrayKeys(a typedArrayBacking) []BStr {
	n := a.jsTypedLen()
	out := make([]BStr, n)
	for i := range out {
		out[i] = FromGoString(strconv.Itoa(i))
	}
	return out
}
