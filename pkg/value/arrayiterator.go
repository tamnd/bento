package value

// The array iterator is the object arr.values(), arr.keys(), and arr.entries()
// hand back, the ArrayIterator the language drives through for...of, spread, and a
// hand-rolled next(). It walks an array by index producing, per step, the element
// (values), the index (keys), or the [index, element] pair (entries), and reports
// done once the index passes the length. It reads length and each index live on
// every step, so it visits an index the array grew into and yields undefined for a
// hole rather than skipping it, matching the spec's [[Get]] rather than a
// HasProperty probe. The result of each step is the same value.IterResult a
// generator's next hands back, so a manual driver reads .value and .done off it the
// same way and a for...of pulls it the same way.

// ArrayIterKind selects which projection an array iterator yields: the element, the
// index, or the [index, element] pair, the three kinds values, keys, and entries
// produce.
type ArrayIterKind int

const (
	// ArrayIterKeys yields each index as a number, the projection arr.keys() takes.
	ArrayIterKeys ArrayIterKind = iota
	// ArrayIterValues yields each element, the projection arr.values() takes and the
	// one for...of and spread over an array walk.
	ArrayIterValues
	// ArrayIterEntries yields each [index, element] pair as a two-element array, the
	// projection arr.entries() takes.
	ArrayIterEntries
)

// ArrayIter is a running walk over an array's indices. src is the array as a boxed
// value, so length and each index read through the same dynamic accessors a
// generic-receiver method uses; i is the next index to visit; kind picks the
// projection. It holds no goroutine, unlike a generator, since an array walk needs
// no suspended body: each Next reads the next index directly.
type ArrayIter struct {
	src  Value
	i    int
	kind ArrayIterKind
}

// NewArrayIter mints an array iterator over a boxed array, the form the runtime
// takes when the source is already a value.Value, a dynamic array or an array-like.
func NewArrayIter(src Value, kind ArrayIterKind) *ArrayIter {
	return &ArrayIter{src: src, kind: kind}
}

// ArrayIterFromTyped mints an array iterator over a statically typed array by
// boxing its elements into a dynamic array once, the form the runtime takes when
// the source is a *Array[T] the lowerer holds. The box closure lifts each typed
// element into a value.Value, the same constructor a static-to-dynamic crossing
// uses, since the iterator yields boxed values whatever the element type. Boxing
// eagerly snapshots the elements, so an iterator over a typed array reflects the
// elements present when it was created.
func ArrayIterFromTyped[T any](a *Array[T], kind ArrayIterKind, box func(T) Value) *ArrayIter {
	elems := a.Elems()
	boxed := make([]Value, len(elems))
	for i, e := range elems {
		boxed[i] = box(e)
	}
	return &ArrayIter{src: NewArrayValue(boxed), kind: kind}
}

// Next advances the iterator one step and packs the { value, done } result. Once
// the index reaches the current length it reports done with undefined; otherwise it
// yields the projection its kind selects and steps the index. It reads length live,
// so an array that grew is walked to its new end and one that shrank stops early.
func (it *ArrayIter) Next() IterResult {
	n := arrayLikeLen(it.src)
	if it.i >= n {
		return IterResult{Value: Undefined, Done: true}
	}
	i := it.i
	it.i++
	switch it.kind {
	case ArrayIterKeys:
		return IterResult{Value: Number(float64(i))}
	case ArrayIterEntries:
		return IterResult{Value: NewArrayValue([]Value{Number(float64(i)), arrayLikeGet(it.src, i)})}
	default: // ArrayIterValues
		return IterResult{Value: arrayLikeGet(it.src, i)}
	}
}
