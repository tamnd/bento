package value

// An async function that awaits is a body that suspends at each await and resumes
// when the awaited promise settles. The Go target has no coroutine, so bento models
// the body the same way it models a generator: a goroutine that hands control back to
// the event loop at each suspend point over a pair of unbuffered channels. At an
// await the body parks and control returns to whoever is driving the loop; a
// microtask the awaited promise schedules resumes the body one step later, with the
// settled value fed back or the rejection raised at the await. The two goroutines
// never run at once, so a body field an async function reads across an await is free
// of a data race, the same handoff the generator coroutine relies on.
//
// The function returns its promise immediately, pending, at the first await; the
// coroutine settles it later, when the body runs off its end (fulfilled) or throws a
// value the body did not catch (rejected). An await-free async body never parks and
// keeps the plain value.Async path; only a body that awaits needs this machinery.

// asyncResume is the message the driver sends the parked body to resume it: the
// settled value of the awaited promise, or the rejection to raise at the await. isThrow
// tells the body which resumption it is, so a fulfilled await returns the value and a
// rejected one panics the reason into the body where a try/catch can catch it.
type asyncResume struct {
	val     any
	thrown  Thrown
	isThrow bool
}

// AsyncCo is the handle a suspending async body holds. The body parks by sending on
// parked and waits for the next step on resume; the driver advances it by sending the
// settled await result on resume and waiting for the body to park or complete on
// parked. It carries no element type: the awaited value rides asyncResume as a boxed
// any, since a single body awaits promises of many element types.
type AsyncCo struct {
	resume chan asyncResume
	parked chan struct{}
}

// RunAsync runs an async body that awaits and returns the promise it settles. The body
// runs in a goroutine up to its first await (or its completion if it never parks) and
// hands control back through parked, so RunAsync returns a pending promise the moment
// the body first suspends. A normal completion fulfills the promise with the body's
// value; a thrown value the body did not catch (a Go panic carrying a Thrown, the
// payload every bento throw raises) rejects it, matching the rule that a throw inside
// an async body becomes a rejection. A Go runtime panic that is not a Thrown is a real
// bug and keeps its stack.
func RunAsync[T any](body func(*AsyncCo) T) *Promise[T] {
	co := &AsyncCo{resume: make(chan asyncResume), parked: make(chan struct{})}
	result := &Promise[T]{}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t, ok := r.(Thrown)
				if !ok {
					panic(r)
				}
				result.reject(t)
				co.parked <- struct{}{}
			}
		}()
		v := body(co)
		result.fulfill(v)
		co.parked <- struct{}{}
	}()
	<-co.parked
	return result
}

// RunAsyncVoid is RunAsync for an async body with no value, a Promise<void>. It runs
// the body and settles a unit promise: fulfilled on a normal completion, rejected on a
// thrown value the body did not catch.
func RunAsyncVoid(body func(*AsyncCo)) *Promise[Unit] {
	co := &AsyncCo{resume: make(chan asyncResume), parked: make(chan struct{})}
	result := &Promise[Unit]{}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				t, ok := r.(Thrown)
				if !ok {
					panic(r)
				}
				result.reject(t)
				co.parked <- struct{}{}
			}
		}()
		body(co)
		result.fulfill(Unit{})
		co.parked <- struct{}{}
	}()
	<-co.parked
	return result
}

// Await suspends the async body at an await expression until p settles, then returns
// its fulfilled value or raises its rejection into the body. It registers a reaction on
// p that, when p settles, resumes the body one step; then it parks, handing control
// back to the driver. Because the reaction is scheduled as a microtask even when p has
// already settled, the code after an await always runs in a later turn, the ordering
// JavaScript fixes for await. The awaited value rides the resume as a boxed any and is
// asserted back to the promise's element type X, which the lowerer knows at the await
// site.
func Await[X any](co *AsyncCo, p *Promise[X]) X {
	p.subscribe(func() {
		if p.state == promiseRejected {
			co.resume <- asyncResume{isThrow: true, thrown: p.reason}
		} else {
			co.resume <- asyncResume{val: p.value}
		}
		<-co.parked
	})
	co.parked <- struct{}{}
	sig := <-co.resume
	if sig.isThrow {
		panic(sig.thrown)
	}
	return sig.val.(X)
}

// AwaitValue awaits a plain, non-promise value. JavaScript awaiting a non-thenable
// wraps it in a resolved promise and suspends for one microtask turn before yielding
// it back, so AwaitValue resolves v into a promise and awaits that, taking the same
// suspend-and-resume path and the same one-turn delay a real await imposes.
func AwaitValue[X any](co *AsyncCo, v X) X {
	return Await(co, Resolved(v))
}
