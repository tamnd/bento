package value

// StructuredClone deep-copies a value the way the WHATWG global structuredClone
// does, for the subset bento's value model represents. A primitive is returned
// unchanged. A plain object or array is copied one own enumerable string key at a
// time, and shared or cyclic references are preserved: an object reached twice
// through the input graph is the same object twice in the clone, and a cycle
// clones to a cycle rather than looping forever. A value the structured-clone
// algorithm rejects, a function or a symbol, throws rather than returning a lossy
// copy, and so does a Proxy, whose traps bento cannot faithfully reproduce in a
// copy. The runtime never sees a Date, RegExp, Map, or Set here, since none of
// those box into a value object in the dynamic model; a program that reaches this
// path holds a primitive, a plain object, an array, a function, or a proxy.
func StructuredClone(v Value) Value {
	return structuredCloneValue(v, map[*Object]Value{})
}

// structuredCloneValue is the recursive worker StructuredClone drives. The memo is
// keyed by the source *Object and holds the clone that stands in for it, so a
// reference the walk has already begun cloning resolves to that same clone. The
// clone is recorded in the memo before its contents are filled, which is what lets
// a cycle terminate: a property that points back at its own object finds the
// half-built clone in the memo rather than recursing into it again.
func structuredCloneValue(v Value, memo map[*Object]Value) Value {
	switch v.kind {
	case KindUndefined, KindNull, KindBool, KindNumber, KindBigInt, KindString:
		return v
	case KindSymbol:
		throwDataClone("a Symbol")
	case KindFunc:
		throwDataClone("a function")
	case KindObject, KindArray:
		o := v.object()
		if o.proxy != nil {
			throwDataClone("a Proxy")
		}
		if o.call != nil {
			throwDataClone("a function")
		}
		if existing, ok := memo[o]; ok {
			return existing
		}
		if v.kind == KindArray {
			clone := NewArrayValue(nil)
			memo[o] = clone
			dst := clone.object()
			dst.elems = make([]Value, len(o.elems))
			for i, e := range o.elems {
				if isHole(e) {
					dst.elems[i] = hole
					continue
				}
				dst.elems[i] = structuredCloneValue(e, memo)
			}
			cloneStringKeys(o, clone, memo)
			return clone
		}
		clone := NewObject()
		memo[o] = clone
		cloneStringKeys(o, clone, memo)
		return clone
	}
	return Undefined
}

// cloneStringKeys copies the source's own enumerable string-keyed properties onto
// the clone, each value cloned in turn through the shared memo. Only enumerable
// string keys are copied, the same keys Object.assign and the spread walk, since
// the structured-clone algorithm carries an object's own enumerable data
// properties and drops its symbol-keyed ones. The read goes through Get so an own
// accessor is invoked for its value the way the algorithm reads each property.
func cloneStringKeys(src *Object, clone Value, memo map[*Object]Value) {
	srcVal := objectValue(src)
	for _, k := range src.orderedStringKeysFiltered(true) {
		clone.Set(k, structuredCloneValue(srcVal.Get(k), memo))
	}
}

// throwDataClone raises the error structuredClone throws when it meets a value the
// structured-clone algorithm cannot carry. Node throws a DataCloneError DOMException
// here; bento reports the same fact through its own error type, naming what could
// not be cloned so the message points at the offending value.
func throwDataClone(what string) {
	Throw(NewError(FromGoString(what + " could not be cloned")))
}
