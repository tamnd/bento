package value

// This file is the Promise constructor side of the value model: new Promise runs
// an executor now and hands it a resolve and a reject function it calls to settle
// the promise. The executor is where user code decides the promise's fate, so it
// runs synchronously on the constructing stack, and whichever of resolve or reject
// it calls first wins, later calls being no-ops the way a promise settles once.
//
// A resolve carries a value of the promise's element type; a reject carries an
// arbitrary JavaScript value, not only an Error, so a program may reject with a
// string or a number the way the language allows. The promise's reason field is
// typed Thrown, so a rejected value rides in rejectionValue, a thin Thrown wrapper
// a catch or an await reads the original value back out of.

// NewPromise mints a pending promise and runs executor now, passing it the resolve
// and reject callbacks that settle the promise. resolve fulfills with a value of the
// element type; reject settles as rejected, carrying the value it was handed through
// rejectionValue. An executor that throws (a Go panic carrying a Thrown, the payload
// every bento throw raises) rejects the promise with that thrown value, matching the
// rule that a synchronous throw inside the executor rejects rather than propagates. A
// Go runtime panic that is not a Thrown is a real bug and keeps its stack.
func NewPromise[T any](executor func(resolve func(T), reject func(Value))) (p *Promise[T]) {
	p = &Promise[T]{}
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			p.reject(t)
		}
	}()
	executor(
		func(v T) { p.fulfill(v) },
		func(reason Value) { p.reject(rejectionValue{reason}) },
	)
	return p
}

// All combines an array of promises into one promise of the array of their fulfilled
// values, the value.Promise side of Promise.all. It fulfills with the values in input
// order once every input has fulfilled, and rejects with the reason of the first input
// to reject, ignoring later settlements the way the first rejection wins. An empty
// input fulfills immediately with an empty array. Each input is subscribed, so a
// reaction runs at the microtask checkpoint when the input settles; the combined
// promise stays pending until its count reaches zero, then fulfills and flushes its
// own reactions.
func All[T any](ps *Array[*Promise[T]]) *Promise[*Array[T]] {
	result := &Promise[*Array[T]]{}
	elems := ps.Elems()
	if len(elems) == 0 {
		result.fulfill(NewArray[T]())
		return result
	}
	vals := make([]T, len(elems))
	remaining := len(elems)
	for i, p := range elems {
		p.subscribe(func() {
			if result.state != promisePending {
				return
			}
			if p.state == promiseRejected {
				result.reject(p.reason)
				return
			}
			vals[i] = p.value
			remaining--
			if remaining == 0 {
				result.fulfill(NewArray[T](vals...))
			}
		})
	}
	return result
}

// Race combines an array of promises into one promise that settles the way the first
// input to settle does, the value.Promise side of Promise.race: it fulfills with that
// input's value if it fulfilled, or rejects with its reason if it rejected, and later
// settlements are ignored once the race is decided. An empty input never settles, the
// forever-pending promise Promise.race([]) returns, so the loop simply subscribes
// nothing and the result stays pending.
func Race[T any](ps *Array[*Promise[T]]) *Promise[T] {
	result := &Promise[T]{}
	for _, p := range ps.Elems() {
		p.subscribe(func() {
			if result.state != promisePending {
				return
			}
			if p.state == promiseRejected {
				result.reject(p.reason)
			} else {
				result.fulfill(p.value)
			}
		})
	}
	return result
}

// Any combines an array of promises into one promise that fulfills with the first
// input to fulfill, the value.Promise side of Promise.any: a rejection does not decide
// the race, so the result stays pending while rejections accumulate, and only when every
// input has rejected does it reject with an AggregateError whose errors array carries the
// rejection reasons in input order. An empty input has no promise that can fulfill, so it
// rejects at once with an AggregateError over no errors, matching Promise.any([]).
func Any[T any](ps *Array[*Promise[T]]) *Promise[T] {
	result := &Promise[T]{}
	elems := ps.Elems()
	if len(elems) == 0 {
		result.reject(NewAggregateError([]Value{}, FromGoString("All promises were rejected")))
		return result
	}
	reasons := make([]Value, len(elems))
	remaining := len(elems)
	for i, p := range elems {
		p.subscribe(func() {
			if result.state != promisePending {
				return
			}
			if p.state == promiseRejected {
				reasons[i] = thrownValue(p.reason)
				remaining--
				if remaining == 0 {
					result.reject(NewAggregateError(reasons, FromGoString("All promises were rejected")))
				}
			} else {
				result.fulfill(p.value)
			}
		})
	}
	return result
}

// NewRejection wraps an arbitrary value into the Thrown a rejected promise carries,
// so Promise.reject and a manual Rejected can settle with any JavaScript value, not
// only a runtime Error. A catch handler or a rejected await reads the value back
// through the Thrown's ToValue.
func NewRejection(reason Value) Thrown { return rejectionValue{reason} }

// rejectionValue wraps the arbitrary value a reject call carries into a Thrown, so a
// promise's reason field (typed Thrown) can hold any JavaScript value, not only a
// runtime Error. A catch handler or a rejected await reads the original value back
// through ToValue; ErrorName and ErrorMessage read the name and message off the value
// when it is an error-shaped object, so an uncaught rejection with an Error still
// reports its own name and message rather than a generic placeholder.
type rejectionValue struct{ v Value }

// ErrorName reports the name the rejection value carries when it is an error-shaped
// object, falling back to "Error" for a rejection with a plain value that has no name
// property, the label an uncaught non-error rejection is reported under.
func (r rejectionValue) ErrorName() string {
	if n := r.v.Get(FromGoString("name")); n.Kind() == KindString {
		return n.AsString().ToGoString()
	}
	return "Error"
}

// ErrorMessage reports the message the rejection value carries when it is an
// error-shaped object, falling back to the empty string for a rejection with a plain
// value that has no message property.
func (r rejectionValue) ErrorMessage() string {
	if m := r.v.Get(FromGoString("message")); m.Kind() == KindString {
		return m.AsString().ToGoString()
	}
	return ""
}

// ToValue hands back the original value the reject call carried, so a catch handler
// or a rejected await reads the very value the program rejected with rather than a
// wrapper around it.
func (r rejectionValue) ToValue() Value { return r.v }
