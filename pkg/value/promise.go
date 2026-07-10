package value

// This file is the value-model side of a JavaScript promise, the smallest honest
// piece the class-shape tests observe (spec 2075 doc 11 owns the full event loop).
// A compiled test262 job runs to completion in a single turn: there are no timers
// or IO to make a promise pending, so every promise this runtime mints is already
// settled the moment it is created. That collapses the event loop to one thing the
// generated code needs: a microtask queue that main drains once at its end, so a
// .then callback registered during the synchronous run fires after that run the way
// JavaScript schedules it, not inline.
//
// A promise is settled either fulfilled, carrying a value of its element type, or
// rejected, carrying the thrown value an async body raised. An async body runs
// synchronously up to its first await, and an await-free body runs to completion on
// the calling stack, so Async runs the body now and turns a normal return into a
// fulfilled promise and a thrown value (a Go panic carrying a Thrown, the same
// payload a catch recovers) into a rejected one.

// Unit is the no-value placeholder a void promise carries as its element type, so a
// Promise<void> lowers to a concrete *Promise[Unit] rather than needing a special
// element-less promise. It holds nothing: a fulfilled unit promise records only that
// the void async body completed.
type Unit struct{}

// microtasks is the package-level queue of callbacks a settled promise's Then and
// Catch enqueue. It is drained by RunMicrotasks at the end of main. A single queue
// for the whole program matches the single event loop JavaScript runs; the compiled
// job is single-turn, so the queue is filled during the synchronous run and emptied
// once.
var microtasks []func()

// enqueueMicrotask appends a callback to the microtask queue. A settled promise
// does not run its callback inline: JavaScript always defers a then callback to the
// microtask checkpoint, so even an already-resolved promise schedules rather than
// calls, and the observable ordering (synchronous code first, then callbacks) holds.
func enqueueMicrotask(f func()) {
	microtasks = append(microtasks, f)
}

// RunMicrotasks drains the microtask queue to completion, running each callback in
// the order it was enqueued. A callback may enqueue more (a then inside a then), so
// the loop re-reads the length each pass and runs until the queue is empty, the
// run-to-completion semantics of the microtask checkpoint. The assembled main calls
// it once at its end when the program minted any promise.
func RunMicrotasks() {
	for len(microtasks) > 0 {
		f := microtasks[0]
		microtasks = microtasks[1:]
		f()
	}
}

// Promise is a settled JavaScript promise of element type T. It holds either a
// fulfilled value or the thrown value a rejection carries; rejected tells the two
// apart. It is a pointer type so a promise has reference identity the way a
// JavaScript promise object does. Only settled promises exist in 6a: no async
// source can leave one pending, so there is no state beyond the two settled cases.
type Promise[T any] struct {
	value    T
	rejected bool
	reason   Thrown
}

// Resolved mints a promise already fulfilled with v, the promise an await-free
// async body returns when it runs to a normal completion.
func Resolved[T any](v T) *Promise[T] {
	return &Promise[T]{value: v}
}

// Rejected mints a promise already rejected with reason, the promise an async body
// returns when it throws. The reason is the thrown value a catch would recover.
func Rejected[T any](reason Thrown) *Promise[T] {
	return &Promise[T]{rejected: true, reason: reason}
}

// Async runs an await-free async body now and turns its completion into a settled
// promise: a normal return fulfills, and a thrown value (a Go panic carrying a
// Thrown, the payload every bento throw raises) rejects. This mirrors the
// JavaScript rule that a synchronous throw inside an async body becomes a rejected
// promise rather than propagating. A Go runtime panic (not a Thrown) is a runtime
// bug, not a program throw, so it is re-panicked to keep its original stack.
func Async[T any](body func() T) (p *Promise[T]) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			p = Rejected[T](t)
		}
	}()
	return Resolved(body())
}

// AsyncVoid is Async for an async body with no value, a Promise<void>. It runs the
// body and settles a unit promise: fulfilled on a normal return, rejected on a
// thrown value. The element type is Unit, the value model's no-value placeholder,
// so a void async method has a concrete Go result type like any other.
func AsyncVoid(body func()) (p *Promise[Unit]) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			p = Rejected[Unit](t)
		}
	}()
	body()
	return Resolved(Unit{})
}

// Then schedules onFulfilled to run with the fulfilled value at the next microtask
// checkpoint. A rejected promise does not run onFulfilled: rejection propagation
// through a returned promise is a later slice, so a then with no rejection handler
// simply does not fire on a rejected promise, which is safe because the fixtures
// observe rejection through Catch. The callback is always deferred, never inlined,
// so synchronous code after the then runs first. It returns a settled unit promise,
// the promise then produces in JavaScript, so a then whose result is bound or
// chained has a value of the right type; the callback covered here returns nothing,
// so the returned promise carries no value.
func (p *Promise[T]) Then(onFulfilled func(T)) *Promise[Unit] {
	if !p.rejected {
		v := p.value
		enqueueMicrotask(func() { onFulfilled(v) })
	}
	return Resolved(Unit{})
}

// Catch schedules onRejected to run with the rejection reason at the next microtask
// checkpoint. A fulfilled promise does not run onRejected. The reason is handed
// over as a dynamic value: a caught rejection is typed any in JavaScript, so the
// callback reads it through the value model the way a catch binding boxed into the
// dynamic world does. Like Then it returns a settled unit promise so a bound or
// chained catch has a value of the right type.
func (p *Promise[T]) Catch(onRejected func(Value)) *Promise[Unit] {
	if p.rejected {
		reason := p.reason
		enqueueMicrotask(func() { onRejected(thrownValue(reason)) })
	}
	return Resolved(Unit{})
}

// thrownValue boxes a thrown value into the dynamic value a rejection handler
// reads. A runtime *Error boxes to its stable object view (so reading .message off
// the reason works); any other Thrown is surfaced through the same view its own
// ToValue gives when it has one, falling back to undefined for a payload with no
// dynamic projection yet, a later slice.
func thrownValue(t Thrown) Value {
	if e, ok := t.(*Error); ok {
		return e.ToValue()
	}
	if v, ok := t.(interface{ ToValue() Value }); ok {
		return v.ToValue()
	}
	return Undefined
}
