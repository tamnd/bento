package value

// An async generator (async function* g()) is both a generator and an async body: it
// yields values a consumer pulls one at a time, and it awaits promises between yields.
// The Go target has no coroutine, so bento models the body the same goroutine-over-
// channels way it models a plain generator and a plain async body, except the body now
// has two kinds of suspend point. At a yield the body parks and the current pull's
// promise fulfills with the { value, done } result; at an await the body parks and the
// pull's promise stays pending until the awaited promise settles and the body resumes,
// runs on to its next yield or its completion, and only then settles the pull. So a
// single goroutine drives, and the driver reads why the body parked to decide whether to
// settle the pull now (a yield or a completion) or to wait for a promise first (an await).
//
// Unlike a plain generator, whose Next returns the value synchronously, an async
// generator's Next returns a promise: the consumer awaits each pull. That promise is the
// join between the pull-at-a-time generator protocol and the settle-later promise
// protocol, and it lets a for await...of drive the async generator by awaiting each next.

// The reasons an async generator body parks and hands a frame back to its driver: it
// yielded a value, it is awaiting a promise, it completed with a return value, or an
// uncaught throw escaped it.
const (
	asyncGenYield = iota
	asyncGenAwait
	asyncGenDone
	asyncGenThrow
)

// asyncGenFrame is one message the body sends its driver when it parks. kind selects why
// it parked: a yield carries the value, an await carries a subscriber that registers the
// body's resume on the awaited promise, a completion carries the return value, and an
// escaped throw carries the thrown value the driver rejects the pull with.
type asyncGenFrame[Y any] struct {
	kind    int
	value   Y
	ret     Value
	awaited func(func(settled any, thrown Thrown, isThrow bool))
	thrown  Thrown
}

// The resume kinds a driver sends the parked body: a plain next(v) that resumes a yield
// with the sent value, a return(v) that unwinds the body through its finally blocks, a
// throw(e) that raises e at the suspended yield, and an await continuation that resumes a
// suspended await with the promise's settled value or its rejection.
const (
	asyncGenResumeNext = iota
	asyncGenResumeReturn
	asyncGenResumeThrow
	asyncGenResumeAwait
)

// asyncGenResume is the message a driver sends the parked body. For a yield resume it
// carries the sent value, the return value, or the thrown value; for an await
// continuation it carries the settled value and, when the awaited promise rejected, the
// rejection to raise at the await.
type asyncGenResume struct {
	kind         int
	sent         Value
	ret          Value
	thrown       Thrown
	awaitVal     any
	awaitThrown  Thrown
	awaitIsThrow bool
}

// AsyncGen is a running async generator of yield type Y. The body runs in a goroutine
// that parks on out; the driver advances it through Next and settles each pull's promise
// with the { value, done } result. started gates the goroutine launch to the first pull,
// so an async generator that is never pulled never runs its body, and done latches once
// the body completes so a later pull resolves to a done result without resuming.
type AsyncGen[Y any] struct {
	out     chan asyncGenFrame[Y]
	in      chan asyncGenResume
	body    func(*AsyncGenCo[Y]) Value
	started bool
	done    bool
}

// AsyncGenCo is the handle the body holds. It yields and awaits through the same channel
// pair the AsyncGen drives, so a yield sends a yield frame and blocks for the resume and
// an await sends an await frame and blocks for the settled value. It is passed to the
// body func the lowerer builds from the async generator source.
type AsyncGenCo[Y any] struct {
	out chan asyncGenFrame[Y]
	in  chan asyncGenResume
}

// NewAsyncGen mints an async generator whose body is the goroutine func the lowerer
// builds from an async generator function's source. The body takes the coroutine handle
// it yields and awaits through and returns the value the generator completes with,
// undefined for one that runs off its end with no return value.
func NewAsyncGen[Y any](body func(*AsyncGenCo[Y]) Value) *AsyncGen[Y] {
	return &AsyncGen[Y]{
		out:  make(chan asyncGenFrame[Y]),
		in:   make(chan asyncGenResume),
		body: body,
	}
}

// start launches the body goroutine on the first pull. The deferred recover turns the
// two non-normal exits into a frame: a return signal unwinds as a genAbort and completes
// with its value, and a throw the body did not catch escapes as a Thrown the driver
// rejects the pull with. A Go runtime panic that is neither keeps its stack.
func (g *AsyncGen[Y]) start() {
	co := &AsyncGenCo[Y]{out: g.out, in: g.in}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				switch t := r.(type) {
				case genAbort:
					g.out <- asyncGenFrame[Y]{kind: asyncGenDone, ret: t.ret}
				case Thrown:
					g.out <- asyncGenFrame[Y]{kind: asyncGenThrow, thrown: t}
				default:
					panic(r)
				}
			}
		}()
		ret := g.body(co)
		g.out <- asyncGenFrame[Y]{kind: asyncGenDone, ret: ret}
	}()
}

// step advances the body one step with a resume signal and reads the frame it parks on.
// On the first pull it launches the goroutine instead of sending a signal, since there
// is no suspended point to resume yet.
func (g *AsyncGen[Y]) step(sig asyncGenResume) asyncGenFrame[Y] {
	if !g.started {
		g.started = true
		g.start()
	} else {
		g.in <- sig
	}
	return <-g.out
}

// Next pulls the next value and returns the promise for the { value, done } result. It
// resumes the body with sig, then reads why the body parked: a yield fulfills the promise
// with the yielded value, a completion fulfills it with a done result, and a throw rejects
// it. An await keeps the promise pending: the body registers its resume on the awaited
// promise, and when that settles the driver resumes the body and reads the next park, so
// the pull settles only once the body reaches its next yield or its completion. The box
// closure lifts the typed yield into a value.Value, since the driver is generic over Y.
func (g *AsyncGen[Y]) Next(sent Value, box func(Y) Value) *Promise[IterResult] {
	p := &Promise[IterResult]{}
	if g.done {
		p.fulfill(IterResult{Value: Undefined, Done: true})
		return p
	}
	g.drive(p, box, asyncGenResume{kind: asyncGenResumeNext, sent: sent})
	return p
}

// drive resumes the body one step and settles the pull's promise from the frame the body
// parks on. A yield fulfills, a completion fulfills done, a throw rejects; an await
// subscribes the body's continuation to the awaited promise and returns, leaving the pull
// pending until that promise settles and drive runs again with the settled value.
func (g *AsyncGen[Y]) drive(p *Promise[IterResult], box func(Y) Value, sig asyncGenResume) {
	f := g.step(sig)
	switch f.kind {
	case asyncGenYield:
		p.fulfill(IterResult{Value: box(f.value), Done: false})
	case asyncGenDone:
		g.done = true
		p.fulfill(IterResult{Value: f.ret, Done: true})
	case asyncGenThrow:
		g.done = true
		p.reject(f.thrown)
	case asyncGenAwait:
		f.awaited(func(settled any, thrown Thrown, isThrow bool) {
			g.drive(p, box, asyncGenResume{kind: asyncGenResumeAwait, awaitVal: settled, awaitThrown: thrown, awaitIsThrow: isThrow})
		})
	}
}

// Yield sends v to the driver and blocks until the driver pulls again, then returns the
// value the consumer passed back through next(v). A return signal unwinds the body as a
// genAbort so its finally blocks run, and a throw signal raises the injected value at the
// yield the way a plain generator's yield does.
func (co *AsyncGenCo[Y]) Yield(v Y) Value {
	co.out <- asyncGenFrame[Y]{kind: asyncGenYield, value: v}
	sig := <-co.in
	switch sig.kind {
	case asyncGenResumeReturn:
		panic(genAbort{ret: sig.ret})
	case asyncGenResumeThrow:
		panic(sig.thrown)
	}
	return sig.sent
}

// promiseSettle erases a promise's element type into the subscriber an await frame
// carries: it registers a reaction that reports the fulfilled value as a boxed any or the
// rejection as a Thrown, so the driver can resume the body without knowing the awaited
// element type. The reaction runs as a microtask, so an await always resumes a turn later
// even when the promise has already settled, the ordering JavaScript fixes for await.
func promiseSettle[X any](p *Promise[X]) func(func(any, Thrown, bool)) {
	return func(cb func(any, Thrown, bool)) {
		p.subscribe(func() {
			if p.state == promiseRejected {
				cb(nil, p.reason, true)
			} else {
				cb(p.value, nil, false)
			}
		})
	}
}

// AsyncGenAwait suspends an async generator body at an await until p settles, then returns
// its fulfilled value or raises its rejection into the body. It parks the body with an
// await frame carrying p's type-erased subscriber, so the driver keeps the current pull
// pending and resumes the body once p settles. The settled value rides the resume as a
// boxed any and is asserted back to the promise's element type X, which the lowerer knows
// at the await site.
func AsyncGenAwait[Y, X any](co *AsyncGenCo[Y], p *Promise[X]) X {
	co.out <- asyncGenFrame[Y]{kind: asyncGenAwait, awaited: promiseSettle(p)}
	sig := <-co.in
	if sig.awaitIsThrow {
		panic(sig.awaitThrown)
	}
	return sig.awaitVal.(X)
}

// AsyncGenAwaitValue awaits a plain, non-promise value inside an async generator body.
// JavaScript awaiting a non-thenable wraps it in a resolved promise and suspends for one
// microtask turn, so this resolves v and awaits that, taking the same park-and-resume
// path and one-turn delay a real await imposes. The awaited element type X leads the type
// parameter list so the lowerer can pin it explicitly while the coroutine's yield type Y
// is inferred from co, the same explicit-element crossing plain AwaitValue takes.
func AsyncGenAwaitValue[X, Y any](co *AsyncGenCo[Y], v X) X {
	return AsyncGenAwait(co, Resolved(v))
}
