package value

import "math"

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

// iterLimit validates a take or drop count the way the spec does before either helper
// runs: coerce to a number, reject NaN with a RangeError, truncate toward zero the way
// ToIntegerOrInfinity does, and reject a negative count with a RangeError. A count of
// positive infinity passes through, so take(Infinity) keeps every value and
// drop(Infinity) skips every value.
func iterLimit(limit Value) float64 {
	n := ToNumber(limit)
	if math.IsNaN(n) {
		Throw(NewRangeError(FromGoString("Iterator limit must not be NaN")))
	}
	n = toInteger(n)
	if n < 0 {
		Throw(NewRangeError(FromGoString("Iterator limit must not be negative")))
	}
	return n
}

// IterTake yields at most limit values from the source and then reports done, the lazy
// take. It counts down as values are pulled, so a source shorter than the limit is
// yielded whole and a source longer is cut off once the count runs out, without pulling
// the values past the cut. A limit of positive infinity never runs out, so the whole
// source is yielded.
func IterTake(next func() IterResult, limit Value) *IterHelper {
	remaining := iterLimit(limit)
	return &IterHelper{next: func() IterResult {
		if remaining <= 0 {
			return IterResult{Value: Undefined, Done: true}
		}
		remaining--
		return next()
	}}
}

// IterDrop skips the first limit values from the source and then yields the rest, the
// lazy drop. It pulls and discards up to the count on the first advance, stopping early
// if the source runs out inside the skip, and yields every value after, so dropping more
// than the source holds yields nothing. A limit of positive infinity skips the whole
// source.
func IterDrop(next func() IterResult, limit Value) *IterHelper {
	toDrop := iterLimit(limit)
	skipped := false
	return &IterHelper{next: func() IterResult {
		if !skipped {
			skipped = true
			for toDrop > 0 {
				r := next()
				if r.Done {
					return r
				}
				toDrop--
			}
		}
		return next()
	}}
}

// iterSource builds the next closure that walks an iterable value, the shared
// machinery behind Iterator.from and flatMap's flatten step. An array walks its
// indices live and a string, when allowString is set, walks its code points, the
// same two sources for...of ranges directly; both yield boxed values. It returns
// false for any other value, which the two callers turn into the TypeError each
// wants: Iterator.from allows strings (iterate-string-primitives) and flatMap does
// not (reject-primitives).
func iterSource(src Value, allowString bool) (func() IterResult, bool) {
	switch src.kind {
	case KindArray:
		i := 0
		return func() IterResult {
			if i >= arrayLikeLen(src) {
				return IterResult{Value: Undefined, Done: true}
			}
			v := arrayLikeGet(src, i)
			i++
			return IterResult{Value: v}
		}, true
	case KindString:
		if !allowString {
			return nil, false
		}
		cps := src.str().CodePoints()
		i := 0
		return func() IterResult {
			if i >= len(cps) {
				return IterResult{Value: Undefined, Done: true}
			}
			v := StringValue(cps[i])
			i++
			return IterResult{Value: v}
		}, true
	}
	return nil, false
}

// IterFrom wraps an iterable value as an IterHelper, the runtime behind
// Iterator.from. It drives an array over its indices and a string over its code
// points, the iterate-string-primitives handling Iterator.from asks for. A value
// that is neither is not an iterable this path drives, so it throws a TypeError the
// way the spec does for a non-iterable argument.
func IterFrom(src Value) *IterHelper {
	next, ok := iterSource(src, true)
	if !ok {
		Throw(NewTypeError(FromGoString("Iterator.from called on a non-iterable value")))
		return nil
	}
	return &IterHelper{next: next}
}

// IterFlatMap maps each value through fn and flattens the results, the lazy flatMap.
// It drives the outer source one value at a time and, for each, drives the iterable
// fn returns to exhaustion before pulling the next outer value, so the yields
// interleave in the order the spec lays out. The mapped value flattens under
// reject-primitives handling: an array flattens over its elements and anything else,
// a string or any other primitive included, throws a TypeError, matching flatMap's
// GetIteratorFlattenable(reject-primitives). The index passed to fn counts the outer
// values seen.
func IterFlatMap(next func() IterResult, fn Value) *IterHelper {
	i := 0
	var inner func() IterResult
	return &IterHelper{next: func() IterResult {
		for {
			if inner != nil {
				r := inner()
				if !r.Done {
					return r
				}
				inner = nil
			}
			outer := next()
			if outer.Done {
				return outer
			}
			mapped := fn.Call(outer.Value, Number(float64(i)))
			i++
			src, ok := iterSource(mapped, false)
			if !ok {
				Throw(NewTypeError(FromGoString("Iterator.prototype.flatMap mapper must return an iterable")))
				return IterResult{Value: Undefined, Done: true}
			}
			inner = src
		}
	}}
}
