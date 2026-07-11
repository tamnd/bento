package value

import "os"

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

// ReportUnhandledRejections surfaces every promise that settled rejected and was never
// subscribed to, the unhandled-rejection path JavaScript runs after the microtask
// checkpoint. It prints each one to stderr in the shape Node uses and, if any exist,
// exits non-zero, so a test that asserts a rejection observes the crash rather than a
// false pass. The assembled main calls it once, right after the final microtask drain,
// so every reaction that could still consume a rejection has already run.
func ReportUnhandledRejections() {
	reported := false
	for _, p := range trackedRejections {
		reason, unhandled := p.unhandledReason()
		if !unhandled {
			continue
		}
		v := thrownValue(reason)
		line := "Uncaught (in promise) " + describeRejection(v)
		_, _ = os.Stderr.WriteString(line + "\n")
		reported = true
	}
	if reported {
		os.Exit(1)
	}
}

// describeRejection renders a rejection reason the way Node names it on the
// unhandled-rejection line: an error object shows its name and message, and any other
// value shows its string form, so a rejection with a plain string or number reason is
// still legible rather than collapsed to a generic label.
func describeRejection(v Value) string {
	name := v.Get(FromGoString("name"))
	if name.Kind() == KindString {
		text := name.AsString().ToGoString()
		if msg := v.Get(FromGoString("message")); msg.Kind() == KindString {
			if m := msg.AsString().ToGoString(); m != "" {
				return text + ": " + m
			}
		}
		return text
	}
	return ToString(v).ToGoString()
}

// promiseState is which of the three states a promise is in: pending until it
// settles, then fulfilled with a value or rejected with a reason. An await-free
// async body settles its promise the moment it returns, so its promise is born in a
// settled state; an async body that awaits returns a pending promise the coroutine
// settles later, when the body runs off its end or throws (pkg/value/async.go).
type promiseState uint8

const (
	promisePending promiseState = iota
	promiseFulfilled
	promiseRejected
)

// Promise is a JavaScript promise of element type T. It holds its state and, once
// settled, either a fulfilled value or the thrown value a rejection carries. While
// pending it holds the reactions a Then, a Catch, or an await registered, which fire
// as microtasks when it settles. It is a pointer type so a promise has reference
// identity the way a JavaScript promise object does.
type Promise[T any] struct {
	state    promiseState
	value    T
	reason   Thrown
	onSettle []func()
	handled  bool
}

// rejectedPromise is the element-type-erased view the end-of-run check reads a
// rejected promise through, so trackedRejections can hold promises of any element
// type in one slice. unhandledReason reports the rejection reason and whether it is
// still unhandled, the pair the reporter needs without knowing T.
type rejectedPromise interface {
	unhandledReason() (Thrown, bool)
}

// unhandledReason satisfies rejectedPromise: a promise is an unhandled rejection when
// it settled rejected and no reaction ever subscribed to it, JavaScript's condition
// for the unhandledrejection signal.
func (p *Promise[T]) unhandledReason() (Thrown, bool) {
	return p.reason, p.state == promiseRejected && !p.handled
}

// trackedRejections records every promise that rejects, so ReportUnhandledRejections
// can surface the ones no handler consumed after the microtask queue drains. A single
// package-level list matches the single event loop; the compiled job is single-turn,
// so the list is filled during the run and read once at the end.
var trackedRejections []rejectedPromise

// Resolved mints a promise already fulfilled with v, the promise an await-free
// async body returns when it runs to a normal completion.
func Resolved[T any](v T) *Promise[T] {
	return &Promise[T]{state: promiseFulfilled, value: v}
}

// Rejected mints a promise already rejected with reason, the promise an async body
// returns when it throws. The reason is the thrown value a catch would recover.
func Rejected[T any](reason Thrown) *Promise[T] {
	p := &Promise[T]{state: promiseRejected, reason: reason}
	trackedRejections = append(trackedRejections, p)
	return p
}

// fulfill settles a pending promise with v, running its registered reactions as
// microtasks. Settling an already-settled promise is a no-op, matching the
// JavaScript rule that a promise settles once and keeps its first state.
func (p *Promise[T]) fulfill(v T) {
	if p.state != promisePending {
		return
	}
	p.state = promiseFulfilled
	p.value = v
	p.flush()
}

// reject settles a pending promise as rejected with reason, running its reactions.
// Like fulfill it is a no-op once the promise has settled.
func (p *Promise[T]) reject(reason Thrown) {
	if p.state != promisePending {
		return
	}
	p.state = promiseRejected
	p.reason = reason
	trackedRejections = append(trackedRejections, p)
	p.flush()
}

// flush schedules every reaction the promise gathered while pending, in registration
// order, then clears the list so a reaction runs once. Each reaction reads the now
// settled state when its microtask runs.
func (p *Promise[T]) flush() {
	for _, r := range p.onSettle {
		enqueueMicrotask(r)
	}
	p.onSettle = nil
}

// subscribe registers a reaction to run when the promise settles. A pending promise
// stores it to fire on settle; an already-settled promise schedules it now, since
// JavaScript always defers a reaction to the microtask checkpoint rather than running
// it inline.
func (p *Promise[T]) subscribe(reaction func()) {
	p.handled = true
	if p.state == promisePending {
		p.onSettle = append(p.onSettle, reaction)
		return
	}
	enqueueMicrotask(reaction)
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
// checkpoint. The callback is always deferred, never inlined, so synchronous code after
// the then runs first. It returns the promise then produces in JavaScript: a fresh
// promise that fulfills with unit only once the callback has run, so a chained then runs
// one turn later than this one, the ordering a following then observes. A rejection of
// the receiver passes straight through to the returned promise, since a then with no
// rejection handler forwards the rejection down the chain, and a callback that throws
// (a Go panic carrying a Thrown) rejects the returned promise. The callback covered here
// returns nothing, so the returned promise carries only unit.
func (p *Promise[T]) Then(onFulfilled func(T)) *Promise[Unit] {
	next := &Promise[Unit]{}
	p.subscribe(func() {
		if p.state == promiseRejected {
			next.reject(p.reason)
			return
		}
		settleFromBody(next, func() Unit {
			onFulfilled(p.value)
			return Unit{}
		})
	})
	return next
}

// ThenMap is Then for a callback that returns a plain value, the chaining form
// p.then((v) => v + 1): the returned promise fulfills with the callback's result once
// the receiver fulfills, so a following then reads the mapped value. A rejection of the
// receiver passes straight through to the returned promise, since a then with no
// rejection handler forwards the rejection down the chain, and a callback that throws
// (a Go panic carrying a Thrown) rejects the returned promise rather than propagating.
// The receiver's and result's element types are inferred from the receiver and the
// callback, so a chain of thens carries each stage's value type without annotation.
func ThenMap[T, U any](p *Promise[T], onFulfilled func(T) U) *Promise[U] {
	next := &Promise[U]{}
	p.subscribe(func() {
		if p.state == promiseRejected {
			next.reject(p.reason)
			return
		}
		settleFromBody(next, func() U { return onFulfilled(p.value) })
	})
	return next
}

// ThenFlat is Then for a callback that returns a promise, the adoption form
// p.then((v) => fetch(v)): the returned promise adopts the state of the promise the
// callback returns, fulfilling or rejecting the way that inner promise settles, so the
// chain flattens rather than nesting a promise of a promise. Like ThenMap a rejection
// of the receiver passes through and a callback that throws rejects the returned
// promise. The inner promise's value type is the returned promise's element type, so a
// following then reads the inner value directly.
func ThenFlat[T, U any](p *Promise[T], onFulfilled func(T) *Promise[U]) *Promise[U] {
	next := &Promise[U]{}
	p.subscribe(func() {
		if p.state == promiseRejected {
			next.reject(p.reason)
			return
		}
		guardThrow(next, func() {
			inner := onFulfilled(p.value)
			inner.subscribe(func() {
				if inner.state == promiseRejected {
					next.reject(inner.reason)
				} else {
					next.fulfill(inner.value)
				}
			})
		})
	})
	return next
}

// settleFromBody runs a then callback that produces a value and fulfills next with it,
// turning a throw inside the callback (a Go panic carrying a Thrown) into a rejection
// of next rather than a propagating panic. It is the value-producing half of the two
// chaining reactions; a Go runtime panic that is not a Thrown stays a real crash.
func settleFromBody[U any](next *Promise[U], body func() U) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			next.reject(t)
		}
	}()
	next.fulfill(body())
}

// guardThrow runs a then callback whose body settles next itself (the adoption case
// subscribes next to an inner promise), turning a throw inside the body into a
// rejection of next. It differs from settleFromBody in not fulfilling next on a normal
// return, since the body arranges the settle. A non-Thrown panic stays a real crash.
func guardThrow[U any](next *Promise[U], body func()) {
	defer func() {
		if r := recover(); r != nil {
			t, ok := r.(Thrown)
			if !ok {
				panic(r)
			}
			next.reject(t)
		}
	}()
	body()
}

// Catch schedules onRejected to run with the rejection reason at the next microtask
// checkpoint. A fulfilled promise does not run onRejected; its fulfillment passes
// through, so the returned promise fulfills and a following then still runs. The reason
// is handed over as a dynamic value: a caught rejection is typed any in JavaScript, so
// the callback reads it through the value model the way a catch binding boxed into the
// dynamic world does. Like Then it returns a fresh promise that settles only once the
// reaction has run, so a chained then or finally runs one turn later, and a callback that
// throws rejects the returned promise rather than swallowing the error.
func (p *Promise[T]) Catch(onRejected func(Value)) *Promise[Unit] {
	next := &Promise[Unit]{}
	p.subscribe(func() {
		if p.state != promiseRejected {
			next.fulfill(Unit{})
			return
		}
		settleFromBody(next, func() Unit {
			onRejected(thrownValue(p.reason))
			return Unit{}
		})
	})
	return next
}

// Finally schedules onFinally to run when the promise settles, fulfilled or rejected
// alike, with no argument: the cleanup reaction .finally registers to run whichever way
// the promise ends. Like Then and Catch the callback is deferred to the microtask
// checkpoint, never inlined, so it runs after the synchronous code and in settle order
// among the reactions the promise gathered. It returns a fresh promise that settles only
// once the callback has run, so a chained reaction runs a turn later. A finally does not
// consume a rejection: the returned promise re-raises the receiver's rejection after the
// callback, and a callback that throws overrides it, the rules .finally follows.
func (p *Promise[T]) Finally(onFinally func()) *Promise[Unit] {
	next := &Promise[Unit]{}
	p.subscribe(func() {
		if p.state == promiseRejected {
			guardThrow(next, func() {
				onFinally()
				next.reject(p.reason)
			})
			return
		}
		settleFromBody(next, func() Unit {
			onFinally()
			return Unit{}
		})
	})
	return next
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
