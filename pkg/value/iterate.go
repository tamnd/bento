package value

// Iterate is the runtime side of a for...of, a spread, or an array destructuring
// whose source is a boxed value rather than a shape the checker recognized. The
// lowerer emits it when it cannot tell statically what it is about to walk, which is
// every value that came back from a built-in: `require('os').cpus()` is a
// value.Value of KindArray, and the checker knows only that it is dynamic.
//
// It answers an *IterHelper because that is the one puller the runtime already has.
// A generator, an array iterator and an iterator-helper chain all hand back an
// IterResult from Next, so a caller drives whatever this returns the same way it
// drives those, and the lowerer needs no second loop shape.
//
// The source text is passed in because the message is worth getting right. Node says
// "os.cpus is not iterable", naming the expression the program wrote, and the runtime
// cannot recover that from the value. The lowerer has it, so it hands it over.
func Iterate(v Value, src string) *IterHelper {
	// The spec looks up Symbol.iterator first and unconditionally, so a receiver that
	// defines its own overrides everything below. That matters for an array: bento
	// installs no Array.prototype[Symbol.iterator], so an array normally falls through
	// to the index walk, but a program that assigned its own iterator to one gets that
	// instead, which is what the protocol requires.
	if m := v.getSymKey(symbolIterator); m.Kind() == KindFunc {
		return protocolIter(m.Call())
	}
	switch v.Kind() {
	case KindArray, KindObject:
		// An array walks its indices rather than a captured slice, re-reading length at
		// each step, because that is what the array iterator does: a body that pushes
		// extends the walk and a body that pops ends it early. Reading through the
		// array-like accessors rather than the dense storage means a plain object that
		// carries a length and integer keys walks the same way, which is the shape a
		// built-in that answers an arguments-like object hands back.
		//
		// A plain object with no length reads length as undefined, which ToLength turns
		// into 0, so it iterates zero times rather than throwing. That is a deliberate
		// narrowing of the spec, which throws for a non-iterable object. Getting it right
		// needs an own-length probe the lowerer does not currently have a use for, and
		// walking an object with a length is the case that appears in real programs.
		i := 0
		return NewIterHelper(func() IterResult {
			if i >= arrayLikeLen(v) {
				return IterResult{Value: Undefined, Done: true}
			}
			e := arrayLikeGet(v, i)
			i++
			return IterResult{Value: e}
		})
	case KindString:
		// A string yields one substring per Unicode code point, not per UTF-16 unit, so
		// an astral character is one step rather than two halves of a surrogate pair.
		pts := v.AsString().CodePoints()
		i := 0
		return NewIterHelper(func() IterResult {
			if i >= len(pts) {
				return IterResult{Value: Undefined, Done: true}
			}
			p := pts[i]
			i++
			return IterResult{Value: StringValue(p)}
		})
	}
	Throw(NewTypeError(Concat(FromGoString(src), FromGoString(" is not iterable"))))
	return nil
}

// protocolIter drives an iterator object the general way: pull next(), stop on a
// truthy done, hand back value. It is what a user class that defines
// [Symbol.iterator] returns, and what a built-in iterator such as the one an array's
// values() answers looks like from outside.
//
// A next that is not callable throws the way the spec's GetMethod step does, rather
// than reading undefined off the result forever, so a source that answers something
// iterator-shaped but not an iterator fails at the first pull instead of looping.
func protocolIter(it Value) *IterHelper {
	next := it.Get(FromGoString("next"))
	if next.Kind() != KindFunc {
		Throw(NewTypeError(FromGoString("The iterator result is not an object")))
	}
	done := false
	return NewIterHelper(func() IterResult {
		if done {
			return IterResult{Value: Undefined, Done: true}
		}
		r := next.Call()
		if ToBoolean(r.Get(FromGoString("done"))) {
			done = true
			return IterResult{Value: Undefined, Done: true}
		}
		return IterResult{Value: r.Get(FromGoString("value"))}
	})
}

// iterObject wraps a pull closure in the object JavaScript expects an iterator to be:
// a next() answering { value, done }, and a Symbol.iterator answering itself so the
// result can be handed straight to anything that takes an iterable. It is what a
// collection's keys(), values() and entries() hand back, and what an array's three
// iterator methods build on through iterValue.
func iterObject(next func() IterResult) Value {
	obj := NewObject()
	obj.Set(FromGoString("next"), NewFunc(func(args []Value) Value {
		r := next()
		res := NewObject()
		res.Set(FromGoString("value"), r.Value)
		res.Set(FromGoString("done"), Bool(r.Done))
		return res
	}))
	obj.setSymKey(symbolIterator, NewFunc(func(args []Value) Value { return obj }))
	return obj
}

// IterateToSlice drains an iterable into a Go slice, the eager form a spread and an
// array destructuring need where a loop will not stand: `[...it]` and
// `const [a, b] = it` both want every element at once. It is Iterate driven to
// exhaustion, so the two agree on what is iterable and on what each element is.
func IterateToSlice(v Value, src string) []Value {
	it := Iterate(v, src)
	out := []Value{}
	for {
		r := it.Next()
		if r.Done {
			return out
		}
		out = append(out, r.Value)
	}
}
