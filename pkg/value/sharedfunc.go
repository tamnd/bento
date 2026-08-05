package value

// sharedFuncBoxes memoizes the boxed form of a function that exists once in a run, so
// every place that boxes it hands out one value rather than a fresh wrapper each time.
//
// Boxing builds a closure at the site that needs it, and two closures are two values
// even when they wrap the same function. In the language they are one object, so
// without a memo `h === h` reads false through a dynamic slot, an array of listeners
// cannot be searched by the function that was pushed into it, and process.off never
// finds what process.on registered. The compiler keys the memo per module-level
// binding, which is the case where one key really is one function.
var sharedFuncBoxes = map[string]Value{}

// SharedFunc answers the one box for key, taking the wrapper it was handed as that box
// the first time it is asked. The wrapper is built by the caller either way, which
// costs one closure allocation at the sites that lose the race; making it lazy would
// mean a thunk per site, which allocates the same closure to avoid allocating a
// closure.
func SharedFunc(key string, box Value) Value {
	if v, ok := sharedFuncBoxes[key]; ok {
		return v
	}
	sharedFuncBoxes[key] = box
	return box
}
