package value

// arrayProtoBuiltin supplies the Array.prototype methods a dynamic property read
// resolves off an array when nothing on its own chain overrode them. It is the array
// twin of objectProtoBuiltin and exists for the same reason: bento's object model
// carries no synthetic Array.prototype object, so a chain walk that reached the end
// answered undefined, and `require('os').cpus().map(fn)` then invoked undefined and
// threw "undefined is not a function" at run time.
//
// That was the sharpest gap left in the value model. An array that came back from the
// runtime answered length, an index, Object.keys and JSON.stringify, so it looked like
// an array right up to the first method call, and the failure landed after the build
// had already succeeded.
//
// Every method delegates to the generic-receiver implementation in arraylike.go or
// arraylikemutate.go, the same code a borrowed Array.prototype.<m>.call(arrayLike)
// runs. So there is one implementation of each method rather than a dynamic copy that
// could drift from the static one, and a method reached this way behaves the same on a
// real array and on an array-like.
//
// The returned function closes over recv because a dynamic call site threads no
// separate this: arr.map(f) always calls with this === arr, which is exactly recv.
// A receiver that declares its own property of the same name never reaches here, since
// getChained returns the own or nearer-prototype slot first, so a user override still
// wins the way own-before-inherited lookup requires.
func arrayProtoBuiltin(recv Value, key BStr) (Value, bool) {
	fn, ok := arrayProtoMethods[key.ToGoString()]
	if !ok {
		return Undefined, false
	}
	return NewFunc(func(args []Value) Value { return fn(recv, args) }), true
}

// tail returns the arguments after the first n, the optional trailing arguments a
// method takes past the one it always reads: the thisArg of a callback method, the
// fromIndex of a search. A call that passed fewer arguments yields an empty slice, so
// the method sees the argument as absent rather than as an undefined it was handed.
func tail(args []Value, n int) []Value {
	if len(args) <= n {
		return nil
	}
	return args[n:]
}

// arrayProtoMethods is every named Array.prototype method, keyed by its property name.
// A map rather than a switch because the same table answers the presence question a
// key probe asks, and because the count is the point: this is the whole prototype, not
// a popular subset, so a program cannot find the one method that was left out.
//
// The absentees are deliberate and are not methods. Symbol.iterator is symbol-keyed
// and installed on the symbol path rather than here, Symbol.unscopables is a data
// property with no runtime behaviour bento observes, and constructor would hand back a
// function object the value model has no Array constructor for.
// The table is filled in init rather than in its own initializer because the two
// refer to each other through a long chain: a method coerces an argument, a coercion
// may call a user toPrimitive, a call may go through a proxy trap, and a trap reads a
// property, which is the getChained that consults this table. Go reads that as an
// initialization cycle even though nothing runs at init time, and filling the map in
// init breaks it without a lock or a lazy check on the hot path.
var arrayProtoMethods map[string]func(recv Value, args []Value) Value

func init() {
	arrayProtoMethods = map[string]func(recv Value, args []Value) Value{
		"at":             func(r Value, a []Value) Value { return GenericAt(r, Arg(a, 0)) },
		"concat":         func(r Value, a []Value) Value { return GenericConcat(r, a...) },
		"copyWithin":     func(r Value, a []Value) Value { return GenericCopyWithin(r, a...) },
		"entries":        func(r Value, a []Value) Value { return GenericEntries(r) },
		"every":          func(r Value, a []Value) Value { return GenericEvery(r, Arg(a, 0), tail(a, 1)...) },
		"fill":           func(r Value, a []Value) Value { return GenericFill(r, Arg(a, 0), tail(a, 1)...) },
		"filter":         func(r Value, a []Value) Value { return GenericFilter(r, Arg(a, 0), tail(a, 1)...) },
		"find":           func(r Value, a []Value) Value { return GenericFind(r, Arg(a, 0), tail(a, 1)...) },
		"findIndex":      func(r Value, a []Value) Value { return GenericFindIndex(r, Arg(a, 0), tail(a, 1)...) },
		"findLast":       func(r Value, a []Value) Value { return GenericFindLast(r, Arg(a, 0), tail(a, 1)...) },
		"findLastIndex":  func(r Value, a []Value) Value { return GenericFindLastIndex(r, Arg(a, 0), tail(a, 1)...) },
		"flat":           func(r Value, a []Value) Value { return GenericFlat(r, a...) },
		"flatMap":        func(r Value, a []Value) Value { return GenericFlatMap(r, Arg(a, 0), tail(a, 1)...) },
		"forEach":        func(r Value, a []Value) Value { return GenericForEach(r, Arg(a, 0), tail(a, 1)...) },
		"includes":       func(r Value, a []Value) Value { return GenericIncludes(r, Arg(a, 0), tail(a, 1)...) },
		"indexOf":        func(r Value, a []Value) Value { return GenericIndexOf(r, Arg(a, 0), tail(a, 1)...) },
		"join":           func(r Value, a []Value) Value { return GenericJoin(r, a...) },
		"keys":           func(r Value, a []Value) Value { return GenericKeys(r) },
		"lastIndexOf":    func(r Value, a []Value) Value { return GenericLastIndexOf(r, Arg(a, 0), tail(a, 1)...) },
		"map":            func(r Value, a []Value) Value { return GenericMap(r, Arg(a, 0), tail(a, 1)...) },
		"pop":            func(r Value, a []Value) Value { return GenericPop(r) },
		"push":           func(r Value, a []Value) Value { return GenericPush(r, a...) },
		"reduce":         func(r Value, a []Value) Value { return GenericReduce(r, Arg(a, 0), tail(a, 1)...) },
		"reduceRight":    func(r Value, a []Value) Value { return GenericReduceRight(r, Arg(a, 0), tail(a, 1)...) },
		"reverse":        func(r Value, a []Value) Value { return GenericReverse(r) },
		"shift":          func(r Value, a []Value) Value { return GenericShift(r) },
		"slice":          func(r Value, a []Value) Value { return GenericSlice(r, a...) },
		"some":           func(r Value, a []Value) Value { return GenericSome(r, Arg(a, 0), tail(a, 1)...) },
		"sort":           func(r Value, a []Value) Value { return GenericSort(r, a...) },
		"splice":         func(r Value, a []Value) Value { return GenericSplice(r, a...) },
		"toLocaleString": func(r Value, a []Value) Value { return GenericToLocaleString(r) },
		"toReversed":     func(r Value, a []Value) Value { return GenericToReversed(r) },
		"toSorted":       func(r Value, a []Value) Value { return GenericToSorted(r, a...) },
		"toSpliced":      func(r Value, a []Value) Value { return GenericToSpliced(r, a...) },
		"toString":       func(r Value, a []Value) Value { return GenericToString(r) },
		"unshift":        func(r Value, a []Value) Value { return GenericUnshift(r, a...) },
		"values":         func(r Value, a []Value) Value { return GenericValues(r) },
		"with":           func(r Value, a []Value) Value { return GenericWith(r, Arg(a, 0), Arg(a, 1)) },
	}
}
