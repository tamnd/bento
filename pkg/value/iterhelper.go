package value

// The iterator helpers are the lazy iterator methods ES2024 hangs off
// Iterator.prototype: map, filter, take, drop, and flatMap return a new iterator
// that pulls from the source on demand, and reduce, toArray, forEach, some, every,
// and find drive the source to exhaustion and return a value (10_advanced group 5).
//
// IterHelper is the one runtime iterator every helper produces and consumes. It
// holds a next closure that yields the same value.IterResult a generator or an
// array iterator hands back, so a helper reads .Value and .Done off a step the same
// way a for...of or a manual next() does, and a chain of helpers is a chain of next
// closures each wrapping the one below it. The helpers are free functions taking a
// next closure rather than methods, so an array iterator's Next and an IterHelper's
// Next feed them the same way, which is how arr.values().map(...) and
// Iterator.from(...).map(...) share one path.
type IterHelper struct {
	next func() IterResult
}

// NewIterHelper wraps a next closure as an IterHelper, the constructor the lowerer
// emits when it needs to lift an array iterator's Next into the helper the chain
// consumes. A nil closure yields a done result forever, so a helper built over an
// exhausted or empty source is safe to pull.
func NewIterHelper(next func() IterResult) *IterHelper {
	return &IterHelper{next: next}
}

// Next advances the iterator one step, the drive a for...of over a helper result and
// a manual it.next() both take. It reports done once the underlying closure is
// exhausted and keeps reporting done after, so a caller that pulls past the end reads
// { undefined, true } rather than panicking on a nil closure.
func (h *IterHelper) Next() IterResult {
	if h.next == nil {
		return IterResult{Value: Undefined, Done: true}
	}
	return h.next()
}

// IterMap lifts each yielded value through fn(value, index), the lazy map. It calls
// fn only as the result is pulled, and the index counts the values actually seen, so
// a value the source never reaches is never mapped. A done step passes straight
// through without calling fn.
func IterMap(next func() IterResult, fn Value) *IterHelper {
	i := 0
	return &IterHelper{next: func() IterResult {
		r := next()
		if r.Done {
			return r
		}
		mapped := fn.Call(r.Value, Number(float64(i)))
		i++
		return IterResult{Value: mapped}
	}}
}

// IterFilter keeps each value for which fn(value, index) is truthy, the lazy filter.
// It pulls from the source until a value passes the predicate or the source is done,
// so a filtered iterator skips the rejected values without materializing them. The
// index counts every value the predicate sees, kept or dropped.
func IterFilter(next func() IterResult, fn Value) *IterHelper {
	i := 0
	return &IterHelper{next: func() IterResult {
		for {
			r := next()
			if r.Done {
				return r
			}
			keep := ToBoolean(fn.Call(r.Value, Number(float64(i))))
			i++
			if keep {
				return IterResult{Value: r.Value}
			}
		}
	}}
}

// IterFrom wraps an iterable value as an IterHelper, the runtime behind
// Iterator.from and the flatten step flatMap takes over each mapped value. An array
// walks its indices live and a string walks its code points, the same two sources
// for...of ranges directly; both yield boxed values. A value that is neither, and is
// not already a helper, is not an iterable this path drives, so it throws a
// TypeError the way the spec does for a non-iterable argument.
func IterFrom(src Value) *IterHelper {
	switch src.kind {
	case KindArray:
		i := 0
		return &IterHelper{next: func() IterResult {
			if i >= arrayLikeLen(src) {
				return IterResult{Value: Undefined, Done: true}
			}
			v := arrayLikeGet(src, i)
			i++
			return IterResult{Value: v}
		}}
	case KindString:
		cps := src.str().CodePoints()
		i := 0
		return &IterHelper{next: func() IterResult {
			if i >= len(cps) {
				return IterResult{Value: Undefined, Done: true}
			}
			v := StringValue(cps[i])
			i++
			return IterResult{Value: v}
		}}
	}
	Throw(NewTypeError(FromGoString("Iterator.from called on a non-iterable value")))
	return nil
}
