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
