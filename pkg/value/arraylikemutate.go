// This file completes the generic-receiver Array.prototype set that arraylike.go
// started. The methods here are the ones that move elements around rather than only
// read them (push, pop, shift, unshift, splice, sort), the ones that copy instead of
// mutate (toReversed, toSorted, toSpliced, with), and the remaining readers the
// first file did not carry (at, flat, flatMap, findLast, findLastIndex, keys, values,
// entries, toString, toLocaleString).
//
// They follow arraylike.go's shape exactly: read length off the receiver, read and
// write each element as the property named by its index, and write length back when
// the operation changes it. That is what makes them work on a boxed array, on an
// array-like object carrying a length and integer keys, and on a real array alike,
// which is what arrayproto.go needs to hand a dynamic receiver its methods.

package value

import (
	"math"
	"sort"
)

// arrayLikeSetLen writes the receiver's length back after an operation changed it.
// On a real array the write resizes the backing store, dropping elements past the new
// end; on an array-like object it stores the property. Both are what the spec's
// Set(O, "length", n, true) step does.
func arrayLikeSetLen(recv Value, n int) {
	recv.Set(FromGoString("length"), Number(float64(n)))
}

// arrayLikeDelete removes the property named by index k, the DeletePropertyOrThrow
// step a shrinking method takes at the indices it vacated. A real array leaves a hole
// there, which is what the spec asks for and what distinguishes `delete a[0]` from
// writing undefined.
func arrayLikeDelete(recv Value, k int) {
	recv.DeleteElem(Number(float64(k)))
}

// relativeIndex resolves a start or end argument against a length the way the spec's
// relative-index steps do: a negative value counts back from the end and clamps at 0,
// a positive one clamps at the length, and an absent one takes the supplied default.
func relativeBound(arg Value, n, def int) int {
	if arg.IsUndefined() {
		return def
	}
	f := toIntegerValue(arg)
	switch {
	case math.IsInf(f, -1):
		return 0
	case f < 0:
		if k := n + int(f); k > 0 {
			return k
		}
		return 0
	case f > float64(n):
		return n
	default:
		return int(f)
	}
}

// GenericAt runs Array.prototype.at on a generic receiver: a non-negative index reads
// that element and a negative one counts back from the end, with an index outside the
// range reading undefined rather than clamping.
func GenericAt(recv, index Value) Value {
	requireArrayLikeThis(recv, "at")
	n := arrayLikeLen(recv)
	k := toIntegerValue(index)
	if k < 0 {
		k += float64(n)
	}
	if k < 0 || k >= float64(n) {
		return Undefined
	}
	return arrayLikeGet(recv, int(k))
}

// GenericPush runs Array.prototype.push on a generic receiver, appending each item at
// the current length in order and returning the new length. It writes length back
// explicitly, so an array-like object that carries a plain length property ends up
// with the right one rather than only the new keys.
func GenericPush(recv Value, items ...Value) Value {
	requireArrayLikeThis(recv, "push")
	n := arrayLikeLen(recv)
	for _, item := range items {
		arrayLikeSet(recv, n, item)
		n++
	}
	arrayLikeSetLen(recv, n)
	return Number(float64(n))
}

// GenericPop runs Array.prototype.pop on a generic receiver, removing and returning
// the last element. An empty receiver still has its length written back as 0, the way
// the spec does, and answers undefined.
func GenericPop(recv Value) Value {
	requireArrayLikeThis(recv, "pop")
	n := arrayLikeLen(recv)
	if n == 0 {
		arrayLikeSetLen(recv, 0)
		return Undefined
	}
	last := arrayLikeGet(recv, n-1)
	arrayLikeDelete(recv, n-1)
	arrayLikeSetLen(recv, n-1)
	return last
}

// GenericShift runs Array.prototype.shift on a generic receiver, removing and
// returning the first element and moving every later element down one index. A hole
// moves as a hole, which is why the loop deletes rather than writes undefined when
// the source index is absent.
func GenericShift(recv Value) Value {
	requireArrayLikeThis(recv, "shift")
	n := arrayLikeLen(recv)
	if n == 0 {
		arrayLikeSetLen(recv, 0)
		return Undefined
	}
	first := arrayLikeGet(recv, 0)
	for k := 1; k < n; k++ {
		if arrayLikeHas(recv, k) {
			arrayLikeSet(recv, k-1, arrayLikeGet(recv, k))
		} else {
			arrayLikeDelete(recv, k-1)
		}
	}
	arrayLikeDelete(recv, n-1)
	arrayLikeSetLen(recv, n-1)
	return first
}

// GenericUnshift runs Array.prototype.unshift on a generic receiver, inserting the
// items at the front and returning the new length. The existing elements move up by
// the number of items, walked from the top down so a move never overwrites an element
// it has not read yet.
func GenericUnshift(recv Value, items ...Value) Value {
	requireArrayLikeThis(recv, "unshift")
	n := arrayLikeLen(recv)
	c := len(items)
	if c > 0 {
		for k := n - 1; k >= 0; k-- {
			if arrayLikeHas(recv, k) {
				arrayLikeSet(recv, k+c, arrayLikeGet(recv, k))
			} else {
				arrayLikeDelete(recv, k+c)
			}
		}
		for i, item := range items {
			arrayLikeSet(recv, i, item)
		}
	}
	arrayLikeSetLen(recv, n+c)
	return Number(float64(n + c))
}

// spliceBounds resolves splice's start and deleteCount against a length, the reading
// both splice and toSpliced share. With no arguments nothing is removed; with only a
// start every element from there to the end is, which is the one-argument form's rule
// rather than a default of zero.
func spliceBounds(n int, args []Value) (start, count int) {
	if len(args) == 0 {
		return 0, 0
	}
	start = relativeBound(Arg(args, 0), n, 0)
	if len(args) == 1 {
		return start, n - start
	}
	d := toIntegerValue(Arg(args, 1))
	switch {
	case d < 0:
		return start, 0
	case d > float64(n-start):
		return start, n - start
	default:
		return start, int(d)
	}
}

// GenericSplice runs Array.prototype.splice on a generic receiver: it removes count
// elements at start, inserts the items in their place, and returns what it removed.
// The shift that closes or opens the gap runs in whichever direction keeps a move from
// overwriting an element it has not read.
func GenericSplice(recv Value, args ...Value) Value {
	requireArrayLikeThis(recv, "splice")
	n := arrayLikeLen(recv)
	start, count := spliceBounds(n, args)
	var items []Value
	if len(args) > 2 {
		items = args[2:]
	}
	removed := make([]Value, 0, count)
	for k := 0; k < count; k++ {
		removed = append(removed, arrayLikeGet(recv, start+k))
	}
	switch c := len(items); {
	case c < count:
		for k := start; k < n-count; k++ {
			if from := k + count; arrayLikeHas(recv, from) {
				arrayLikeSet(recv, k+c, arrayLikeGet(recv, from))
			} else {
				arrayLikeDelete(recv, k+c)
			}
		}
		for k := n; k > n-count+c; k-- {
			arrayLikeDelete(recv, k-1)
		}
	case c > count:
		for k := n - count; k > start; k-- {
			if from := k + count - 1; arrayLikeHas(recv, from) {
				arrayLikeSet(recv, k+c-1, arrayLikeGet(recv, from))
			} else {
				arrayLikeDelete(recv, k+c-1)
			}
		}
	}
	for i, item := range items {
		arrayLikeSet(recv, start+i, item)
	}
	arrayLikeSetLen(recv, n-count+len(items))
	return NewArrayValue(removed)
}

// GenericToSpliced runs Array.prototype.toSpliced, the copying form of splice: it
// builds a new dense array with count elements replaced by the items and leaves the
// receiver alone. A hole in the source reads as undefined here, since the result is
// dense the way the copying methods all are.
func GenericToSpliced(recv Value, args ...Value) Value {
	requireArrayLikeThis(recv, "toSpliced")
	n := arrayLikeLen(recv)
	start, count := spliceBounds(n, args)
	var items []Value
	if len(args) > 2 {
		items = args[2:]
	}
	out := make([]Value, 0, n-count+len(items))
	for k := 0; k < start; k++ {
		out = append(out, arrayLikeGet(recv, k))
	}
	out = append(out, items...)
	for k := start + count; k < n; k++ {
		out = append(out, arrayLikeGet(recv, k))
	}
	requireArrayCreateLength(len(out))
	return NewArrayValue(out)
}

// GenericWith runs Array.prototype.with: a copy of the receiver with one index
// replaced. A negative index counts from the end, and an index outside the range
// throws a RangeError rather than growing the result, which is what separates it from
// a plain write.
func GenericWith(recv, index, val Value) Value {
	requireArrayLikeThis(recv, "with")
	n := arrayLikeLen(recv)
	k := toIntegerValue(index)
	if k < 0 {
		k += float64(n)
	}
	if k < 0 || k >= float64(n) {
		Throw(NewRangeError(FromGoString("Invalid index")))
	}
	requireArrayCreateLength(n)
	out := make([]Value, n)
	for i := 0; i < n; i++ {
		if i == int(k) {
			out[i] = val
		} else {
			out[i] = arrayLikeGet(recv, i)
		}
	}
	return NewArrayValue(out)
}

// GenericFindLast runs Array.prototype.findLast, the backward twin of find: it walks
// from the end and answers the first element the predicate accepts, or undefined. It
// visits every index rather than skipping holes, which is what the find family does
// and what separates it from forEach.
func GenericFindLast(recv, cb Value, thisArg ...Value) Value {
	requireArrayLikeThis(recv, "findLast")
	n := arrayLikeLen(recv)
	for k := n - 1; k >= 0; k-- {
		elem := arrayLikeGet(recv, k)
		if ToBoolean(callBack(cb, thisOrUndefined(thisArg), elem, k)) {
			return elem
		}
	}
	return Undefined
}

// GenericFindLastIndex is findLast's index form, answering the index the predicate
// accepted or -1.
func GenericFindLastIndex(recv, cb Value, thisArg ...Value) Value {
	requireArrayLikeThis(recv, "findLastIndex")
	n := arrayLikeLen(recv)
	for k := n - 1; k >= 0; k-- {
		if ToBoolean(callBack(cb, thisOrUndefined(thisArg), arrayLikeGet(recv, k), k)) {
			return Number(float64(k))
		}
	}
	return Number(-1)
}

// thisOrUndefined picks the optional thisArg a callback-taking method threads, the
// receiver the callback runs with. Absent, it is undefined, which is what a callback
// written as a plain function sees.
func thisOrUndefined(thisArg []Value) Value {
	if len(thisArg) > 0 {
		return thisArg[0]
	}
	return Undefined
}

// flattenInto appends the receiver's elements to out, descending into an element that
// is itself an array while depth remains. It skips a hole rather than flattening it to
// undefined, which is flat's documented difference from a dense copy.
func flattenInto(out []Value, recv Value, depth float64) []Value {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue
		}
		elem := arrayLikeGet(recv, k)
		if depth > 0 && elem.Kind() == KindArray {
			out = flattenInto(out, elem, depth-1)
			continue
		}
		out = append(out, elem)
	}
	return out
}

// GenericFlat runs Array.prototype.flat, concatenating nested arrays into the result
// down to the given depth, one level by default and every level for Infinity.
func GenericFlat(recv Value, depth ...Value) Value {
	requireArrayLikeThis(recv, "flat")
	d := float64(1)
	if len(depth) > 0 && !depth[0].IsUndefined() {
		d = toIntegerValue(depth[0])
		if math.IsInf(ToNumber(depth[0]), 1) {
			d = math.Inf(1)
		}
	}
	return NewArrayValue(flattenInto(nil, recv, d))
}

// GenericFlatMap runs Array.prototype.flatMap, which maps then flattens one level.
// It is not map followed by flat: the callback's result is spread only when it is an
// array, and only ever one level deep, whatever the callback returns.
func GenericFlatMap(recv, cb Value, thisArg ...Value) Value {
	requireArrayLikeThis(recv, "flatMap")
	n := arrayLikeLen(recv)
	var out []Value
	for k := 0; k < n; k++ {
		if !arrayLikeHas(recv, k) {
			continue
		}
		mapped := callBack(cb, thisOrUndefined(thisArg), arrayLikeGet(recv, k), k)
		if mapped.Kind() == KindArray {
			out = flattenInto(out, mapped, 0)
			continue
		}
		out = append(out, mapped)
	}
	return NewArrayValue(out)
}

// sortKeys splits the receiver's indices into the elements that take part in the
// comparison and the counts that follow them. The spec sorts the present, defined
// elements, then puts every undefined after them and every hole after those, and no
// comparator ever sees an undefined or a hole.
func sortKeys(recv Value) (elems []Value, undef, holes int) {
	n := arrayLikeLen(recv)
	for k := 0; k < n; k++ {
		switch {
		case !arrayLikeHas(recv, k):
			holes++
		case arrayLikeGet(recv, k).IsUndefined():
			undef++
		default:
			elems = append(elems, arrayLikeGet(recv, k))
		}
	}
	return elems, undef, holes
}

// sortElems orders the comparable elements by a comparator, defaulting to the
// language's own rule of comparing their string forms rather than their numbers,
// which is why [10, 9] sorts to [10, 9] with no comparator. The sort is stable, the
// guarantee the language has made since ES2019.
func sortElems(elems []Value, cmp Value) {
	if cmp.IsUndefined() {
		sort.SliceStable(elems, func(i, j int) bool {
			return ToString(elems[i]).Compare(ToString(elems[j])) < 0
		})
		return
	}
	sort.SliceStable(elems, func(i, j int) bool {
		r := ToNumber(cmp.Call(elems[i], elems[j]))
		return r < 0
	})
}

// GenericSort runs Array.prototype.sort on a generic receiver, ordering it in place
// and returning it. A comparator that is neither undefined nor callable throws before
// anything is read, the way the spec checks it first.
func GenericSort(recv Value, cmp ...Value) Value {
	requireArrayLikeThis(recv, "sort")
	c := thisOrUndefined(cmp)
	requireSortComparator(c)
	elems, undef, holes := sortKeys(recv)
	k := 0
	for _, e := range elems {
		arrayLikeSet(recv, k, e)
		k++
	}
	for i := 0; i < undef; i++ {
		arrayLikeSet(recv, k, Undefined)
		k++
	}
	for i := 0; i < holes; i++ {
		arrayLikeDelete(recv, k)
		k++
	}
	sortElems(elems, c)
	k = 0
	for _, e := range elems {
		arrayLikeSet(recv, k, e)
		k++
	}
	return recv
}

// GenericToSorted runs Array.prototype.toSorted, the copying form of sort: it builds a
// new dense array in order and leaves the receiver alone. Being dense, a hole in the
// source becomes an undefined at the end rather than staying a hole.
func GenericToSorted(recv Value, cmp ...Value) Value {
	requireArrayLikeThis(recv, "toSorted")
	c := thisOrUndefined(cmp)
	requireSortComparator(c)
	elems, undef, holes := sortKeys(recv)
	sortElems(elems, c)
	requireArrayCreateLength(len(elems) + undef + holes)
	out := make([]Value, 0, len(elems)+undef+holes)
	out = append(out, elems...)
	for i := 0; i < undef+holes; i++ {
		out = append(out, Undefined)
	}
	return NewArrayValue(out)
}

// requireSortComparator performs the check sort and toSorted make before they read
// anything: a comparator argument that is present but not callable throws a TypeError,
// so sort(1) fails rather than quietly sorting by string.
func requireSortComparator(cmp Value) {
	if !cmp.IsUndefined() && cmp.Kind() != KindFunc {
		Throw(NewTypeError(FromGoString("The comparison function must be either a function or undefined")))
	}
}

// GenericToReversed runs Array.prototype.toReversed, the copying form of reverse.
func GenericToReversed(recv Value) Value {
	requireArrayLikeThis(recv, "toReversed")
	n := arrayLikeLen(recv)
	requireArrayCreateLength(n)
	out := make([]Value, n)
	for k := 0; k < n; k++ {
		out[k] = arrayLikeGet(recv, n-1-k)
	}
	return NewArrayValue(out)
}

// GenericToString runs Array.prototype.toString, which is join with a comma and is
// what String(arr) and a template interpolation of an array both reach.
func GenericToString(recv Value) Value {
	requireArrayLikeThis(recv, "toString")
	return GenericJoin(recv)
}

// GenericToLocaleString runs Array.prototype.toLocaleString, which joins with a comma
// like toString but renders each element by calling its own toLocaleString rather than
// its toString. A nullish element renders empty, the way join treats one.
func GenericToLocaleString(recv Value) Value {
	requireArrayLikeThis(recv, "toLocaleString")
	n := arrayLikeLen(recv)
	out := FromGoString("")
	for k := 0; k < n; k++ {
		if k > 0 {
			out = Concat(out, FromGoString(","))
		}
		elem := arrayLikeGet(recv, k)
		if elem.IsNullish() {
			continue
		}
		// The element's own toLocaleString is read off it and called with no argument.
		// A dynamic method read binds its receiver at the read (objectProtoBuiltin closes
		// over it), so the call already runs with this === elem and needs nothing threaded.
		//
		// A primitive element has no such method to read: the value model carries no
		// Number.prototype or String.prototype, so the read answers undefined and calling
		// it would throw where Node renders the number. The fallback is the element's
		// ordinary string form, which is what Number.prototype.toLocaleString answers for
		// the default locale anyway, so this differs from Node only where a locale would
		// group digits, and never by throwing.
		if m := elem.Get(FromGoString("toLocaleString")); m.Kind() == KindFunc {
			out = Concat(out, ToString(m.Call()))
			continue
		}
		out = Concat(out, ToString(elem))
	}
	return StringValue(out)
}

// GenericKeys, GenericValues and GenericEntries hand back the three array iterators,
// each as the object a manual next() drives and a for...of pulls.
func GenericKeys(recv Value) Value {
	requireArrayLikeThis(recv, "keys")
	return iterValue(NewArrayIter(recv, ArrayIterKeys))
}

func GenericValues(recv Value) Value {
	requireArrayLikeThis(recv, "values")
	return iterValue(NewArrayIter(recv, ArrayIterValues))
}

func GenericEntries(recv Value) Value {
	requireArrayLikeThis(recv, "entries")
	return iterValue(NewArrayIter(recv, ArrayIterEntries))
}

// iterValue wraps a running array walk in the object JavaScript expects an iterator to
// be: a next() answering { value, done }, and a Symbol.iterator answering itself so the
// result of arr.values() can be handed straight to anything that takes an iterable.
func iterValue(it *ArrayIter) Value {
	obj := NewObject()
	obj.Set(FromGoString("next"), NewFunc(func(args []Value) Value {
		r := it.Next()
		res := NewObject()
		res.Set(FromGoString("value"), r.Value)
		res.Set(FromGoString("done"), Bool(r.Done))
		return res
	}))
	obj.setSymKey(symbolIterator, NewFunc(func(args []Value) Value { return obj }))
	return obj
}
