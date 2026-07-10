package value

// A generator is a function body that suspends between the values it yields and
// resumes when the consumer pulls the next one. The Go target has no built-in
// coroutine, so bento models the body as a goroutine over a pair of unbuffered
// channels: the body sends each yielded value on out and blocks, and the
// consumer's Next receives that value and sends a resume signal on in, which
// unblocks the body for exactly one more step. The unbuffered channels are the
// suspend points, so the body runs no further than the consumer asks and the two
// goroutines never run at the same time, which keeps a shared field a generator
// body reads and writes free of a data race.
//
// Yield evaluates to the value the consumer passes back through next(v), so the
// resume signal carries that sent value. A consumer can also close the generator
// early with return(v) or inject a throw with throw(e); those arrive as the same
// resume signal with a different kind, and Yield turns them into the completion
// or the panic the suspended body would see, so a try/catch or try/finally in the
// body runs the way it does in JavaScript.

// genFrame is one message the body sends the consumer: either a yielded value
// (done false) or the completion (done true) carrying the body's return value, or
// a throw that escaped the body, re-raised into the consumer as panicked.
type genFrame[Y any] struct {
	value    Y
	ret      Value
	done     bool
	panicked bool
	thrown   Thrown
}

// The resume kinds a consumer sends the suspended body: a plain next(v) or an
// early return(v) that completes the generator. A throw(e) injection is a later
// slice, so no throw kind is modeled yet.
const (
	genResumeNext = iota
	genResumeReturn
)

// genSignal is the resume message the consumer sends the body on a pull: the kind
// selects next or return, and the payload carries the value next(v) resumes the
// yield with or the value return(v) completes with.
type genSignal struct {
	kind int
	sent Value
	ret  Value
}

// genAbort is the panic Yield raises when the consumer sends a return signal: it
// unwinds the body, running its finally blocks, without being caught by a
// try/catch (which recovers a Thrown, not a genAbort), matching the JavaScript
// rule that a return completion runs finally but is not caught. The goroutine's
// recover turns it into the completion frame carrying the return value.
type genAbort struct {
	ret Value
}

// Gen is a running generator of yield type Y. The body runs in a goroutine that
// suspends on out; the consumer drives it through Next, Return, and Throw. started
// gates the goroutine launch to the first pull, so a generator that is never pulled
// never runs its body, matching the JavaScript rule that calling a generator
// function only creates the object. done latches once the body completes, and
// result holds the value the completion carried, the value a { value, done: true }
// result reports.
type Gen[Y any] struct {
	out     chan genFrame[Y]
	in      chan genSignal
	body    func(*GenCo[Y]) Value
	started bool
	done    bool
	result  Value
}

// GenCo is the handle the body holds: it yields through the same channel pair the
// Gen drives, so a yield inside the body sends on out and blocks for the resume on
// in. It is passed to the body func the lowerer builds from the generator source.
type GenCo[Y any] struct {
	out chan genFrame[Y]
	in  chan genSignal
}

// NewGen mints a generator whose body is the goroutine func the lowerer builds
// from a generator function's source. The body takes the coroutine handle it
// yields through and returns the value the generator completes with, undefined for
// a generator with no return value.
func NewGen[Y any](body func(*GenCo[Y]) Value) *Gen[Y] {
	return &Gen[Y]{
		out:  make(chan genFrame[Y]),
		in:   make(chan genSignal),
		body: body,
	}
}

// start launches the body goroutine on the first pull. The goroutine runs the body
// to its next yield or its completion and sends a frame; the deferred recover turns
// the two non-normal exits into a frame too: a return signal unwinds as a genAbort
// and completes with its value, and a throw that the body did not catch escapes as
// a Thrown and is re-raised into the consumer. A Go runtime panic that is neither is
// a real bug and keeps its stack.
func (g *Gen[Y]) start() {
	co := &GenCo[Y]{out: g.out, in: g.in}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				switch t := r.(type) {
				case genAbort:
					g.out <- genFrame[Y]{done: true, ret: t.ret}
				case Thrown:
					g.out <- genFrame[Y]{done: true, panicked: true, thrown: t}
				default:
					panic(r)
				}
			}
		}()
		ret := g.body(co)
		g.out <- genFrame[Y]{done: true, ret: ret}
	}()
}

// resume advances the generator one step with a resume signal and reads the frame
// the body sends back. On the first pull it launches the goroutine instead of
// sending a signal, since there is no suspended yield to resume yet. A frame that
// carries a done completion latches the generator and records its return value; a
// frame that carries an escaped throw re-raises it into the consumer, so an
// uncaught throw inside a generator surfaces where the consumer pulled.
func (g *Gen[Y]) resume(sig genSignal) (Y, bool) {
	if g.done {
		var zero Y
		return zero, true
	}
	if !g.started {
		g.started = true
		g.start()
	} else {
		g.in <- sig
	}
	f := <-g.out
	if f.done {
		g.done = true
		g.result = f.ret
		if f.panicked {
			panic(f.thrown)
		}
		var zero Y
		return zero, true
	}
	return f.value, false
}

// Next pulls the next value, resuming the suspended yield with sent, the value
// next(v) passes back into the body. It returns the yielded value and whether the
// generator is done; a done pull reports the yield type's zero value, which the
// consumer ignores because done is true.
func (g *Gen[Y]) Next(sent Value) (Y, bool) {
	return g.resume(genSignal{kind: genResumeNext, sent: sent})
}

// Return closes the generator early with a return value, the way for...of closes an
// iterable it leaves mid-iteration: the suspended body unwinds through its finally
// blocks and completes carrying ret. A generator that has not started or is already
// done simply latches done, since there is no suspended body to unwind.
func (g *Gen[Y]) Return(ret Value) (Y, bool) {
	if g.done {
		var zero Y
		return zero, true
	}
	if !g.started {
		g.started = true
		g.done = true
		g.result = ret
		var zero Y
		return zero, true
	}
	return g.resume(genSignal{kind: genResumeReturn, ret: ret})
}

// Done reports whether the generator has completed, the state a manual driver reads
// off the result's done between pulls.
func (g *Gen[Y]) Done() bool { return g.done }

// Result is the value the generator completed with, valid once done. It is the
// value a { value, done: true } result carries, undefined for a generator that ran
// off the end with no return value.
func (g *Gen[Y]) Result() Value { return g.result }

// Yield sends v to the consumer and blocks until the consumer pulls again, then
// returns the value the consumer passed back through next(v). A return signal
// unwinds the body as a genAbort so its finally blocks run before the generator
// completes.
func (co *GenCo[Y]) Yield(v Y) Value {
	co.out <- genFrame[Y]{value: v}
	sig := <-co.in
	if sig.kind == genResumeReturn {
		panic(genAbort{ret: sig.ret})
	}
	return sig.sent
}
