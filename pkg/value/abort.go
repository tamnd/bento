package value

// This file gives the AOT runtime the AbortController and AbortSignal pair Node
// exposes as globals, the cancellation surface test/common and much of the timers
// and streams code reads at load. An AbortSignal is an EventTarget: it dispatches an
// "abort" event and carries the aborted flag and the reason a consumer reads, so it
// builds on NewEventTarget and adds the abort-specific state. A controller owns one
// signal and its abort method is the only way to trip it, which is why AbortSignal
// has no public constructor (new AbortSignal() is an illegal constructor in Node);
// a program obtains a signal from a controller or the static factories. Both are
// backed by ordinary value objects carrying method closures, so a compiled program
// reads them through the same dynamic member and call path a require'd module takes.

// NewAbortSignal builds an AbortSignal, an EventTarget carrying the aborted flag it
// starts false, the reason it starts undefined, and an onabort handler slot a program
// may set to a function. throwIfAborted throws the reason when the signal is aborted
// and is a no-op otherwise, the guard a consumer runs before starting work. The signal
// is returned as a value the caller holds as an any, the same shape an EventTarget takes.
func NewAbortSignal() Value {
	sig := NewEventTarget()
	sig.Set(FromGoString("aborted"), False)
	sig.Set(FromGoString("reason"), Undefined)
	sig.Set(FromGoString("onabort"), Null)
	sig.Set(FromGoString("throwIfAborted"), NewFunc(func(args []Value) Value {
		if ToBoolean(sig.Get(FromGoString("aborted"))) {
			Throw(NewThrownValue(sig.Get(FromGoString("reason"))))
		}
		return Undefined
	}))
	return sig
}

// abortSignalNow trips a signal: it records the aborted flag and reason, then fires the
// abort notification once. A signal already aborted is left untouched, the spec's rule
// that a second abort is a no-op. An undefined reason becomes the default AbortError, so
// a consumer reading signal.reason always sees an error object. The onabort handler runs
// first when it is a function, then the registered abort listeners fire through dispatch;
// Node orders onabort among the listeners by when it was assigned, which this approximates
// by running it before the dispatch, close enough for the aborted/reason reads that carry
// the weight. It is shared by the controller's abort method and the static factory.
func abortSignalNow(sig Value, reason Value) {
	if ToBoolean(sig.Get(FromGoString("aborted"))) {
		return
	}
	if reason.kind == KindUndefined {
		reason = newAbortError()
	}
	sig.Set(FromGoString("aborted"), True)
	sig.Set(FromGoString("reason"), reason)
	ev := NewEvent(StringValue(FromGoString("abort")), Undefined)
	if on := sig.Get(FromGoString("onabort")); on.kind == KindFunc {
		on.Call(ev)
	}
	sig.Get(FromGoString("dispatchEvent")).Call(ev)
}

// NewAbortController builds an AbortController, the object that owns one signal and can
// trip it. signal is the AbortSignal a consumer passes to a cancelable operation and
// abort(reason) trips that signal with the given reason or the default AbortError. The
// controller is the only holder of the trip, which is the whole point of the pair: the
// consumer sees a read-only signal while the producer keeps the ability to cancel.
func NewAbortController() Value {
	ctrl := NewObject()
	sig := NewAbortSignal()
	ctrl.Set(FromGoString("signal"), sig)
	ctrl.Set(FromGoString("abort"), NewFunc(func(args []Value) Value {
		abortSignalNow(sig, Arg(args, 0))
		return Undefined
	}))
	return ctrl
}

// NewAbortSignalAborted builds a signal that is already aborted with the given reason,
// the value AbortSignal.abort(reason) returns. It is the static factory's counterpart to
// a controller: no controller trips it, it starts tripped, so a consumer that reads it
// sees aborted true and the reason from the first read. An undefined reason becomes the
// default AbortError, matching a controller's abort with no argument.
func NewAbortSignalAborted(reason Value) Value {
	sig := NewAbortSignal()
	if reason.kind == KindUndefined {
		reason = newAbortError()
	}
	sig.Set(FromGoString("aborted"), True)
	sig.Set(FromGoString("reason"), reason)
	return sig
}

// newAbortError builds the DOMException-shaped error a signal aborted with no reason
// carries, the "This operation was aborted" AbortError Node hands a consumer. bento has
// no DOMException type, so it uses the runtime Error family with the AbortError name, the
// same name-carrying model the built-in errors take, boxed to the value a catch reads.
func newAbortError() Value {
	e := &Error{name: FromGoString("AbortError"), message: FromGoString("This operation was aborted")}
	return e.ToValue()
}
