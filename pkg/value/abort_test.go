package value

import "testing"

// TestAbortControllerAbortTripsSignal pins the core of the pair: a fresh controller's
// signal is not aborted, and after the controller aborts, the signal reports aborted and
// carries the default AbortError reason. The reason's name is AbortError so a consumer
// can tell a cancellation from another failure.
func TestAbortControllerAbortTripsSignal(t *testing.T) {
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	if ToBoolean(sig.Get(FromGoString("aborted"))) {
		t.Fatal("a fresh signal must not be aborted")
	}
	ctrl.Get(FromGoString("abort")).Call()
	if !ToBoolean(sig.Get(FromGoString("aborted"))) {
		t.Fatal("after abort the signal must be aborted")
	}
	reason := sig.Get(FromGoString("reason"))
	if got := reason.Get(FromGoString("name")).AsString().ToGoString(); got != "AbortError" {
		t.Fatalf("want AbortError reason, got %q", got)
	}
}

// TestAbortControllerAbortWithReason pins that an explicit reason flows through: the
// reason the caller passes to abort is the exact value signal.reason reads back, not the
// default AbortError, so a program aborting with its own error recovers that error.
func TestAbortControllerAbortWithReason(t *testing.T) {
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	reason := StringValue(FromGoString("stop now"))
	ctrl.Get(FromGoString("abort")).Call(reason)
	if got := ToString(sig.Get(FromGoString("reason"))).ToGoString(); got != "stop now" {
		t.Fatalf("want the passed reason, got %q", got)
	}
}

// TestAbortSignalDispatchesAbortEvent pins that a signal fires an abort event to a
// listener added with addEventListener, the EventTarget path a cancelable operation
// waits on. The listener records that it ran; one abort fires it once.
func TestAbortSignalDispatchesAbortEvent(t *testing.T) {
	var fired int
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	sig.Get(FromGoString("addEventListener")).Call(
		StringValue(FromGoString("abort")),
		NewFunc(func(args []Value) Value { fired++; return Undefined }),
	)
	ctrl.Get(FromGoString("abort")).Call()
	if fired != 1 {
		t.Fatalf("want the abort listener to fire once, fired %d times", fired)
	}
}

// TestAbortIsIdempotent pins that a second abort is a no-op: the listener fires only on
// the first abort, and the reason stays the one the first abort set rather than being
// overwritten by the second call's reason.
func TestAbortIsIdempotent(t *testing.T) {
	var fired int
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	sig.Get(FromGoString("addEventListener")).Call(
		StringValue(FromGoString("abort")),
		NewFunc(func(args []Value) Value { fired++; return Undefined }),
	)
	ctrl.Get(FromGoString("abort")).Call(StringValue(FromGoString("first")))
	ctrl.Get(FromGoString("abort")).Call(StringValue(FromGoString("second")))
	if fired != 1 {
		t.Fatalf("want one abort event, got %d", fired)
	}
	if got := ToString(sig.Get(FromGoString("reason"))).ToGoString(); got != "first" {
		t.Fatalf("want the first reason to stick, got %q", got)
	}
}

// TestAbortSignalOnAbortHandler pins the onabort handler slot: a function assigned to
// signal.onabort runs when the signal aborts, the event-handler idiom Node supports
// alongside addEventListener.
func TestAbortSignalOnAbortHandler(t *testing.T) {
	var ran bool
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	sig.Set(FromGoString("onabort"), NewFunc(func(args []Value) Value { ran = true; return Undefined }))
	ctrl.Get(FromGoString("abort")).Call()
	if !ran {
		t.Fatal("want the onabort handler to run on abort")
	}
}

// TestAbortSignalThrowIfAborted pins throwIfAborted: it is a no-op on a live signal and
// throws the reason once the signal is aborted, the guard a consumer runs before work.
func TestAbortSignalThrowIfAborted(t *testing.T) {
	ctrl := NewAbortController()
	sig := ctrl.Get(FromGoString("signal"))
	sig.Get(FromGoString("throwIfAborted")).Call() // live signal: no throw

	ctrl.Get(FromGoString("abort")).Call(StringValue(FromGoString("done")))
	func() {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("want throwIfAborted to throw on an aborted signal")
			}
			thrown, ok := r.(Thrown)
			if !ok {
				t.Fatalf("want a thrown value, got %T", r)
			}
			if got := ToString(Caught(thrown).ToValue()).ToGoString(); got != "done" {
				t.Fatalf("want the reason thrown, got %q", got)
			}
		}()
		sig.Get(FromGoString("throwIfAborted")).Call()
	}()
}

// TestNewAbortSignalAborted pins the static-factory shape: a signal built already aborted
// reports aborted from the first read and carries the given reason, the value
// AbortSignal.abort(reason) returns to a consumer that wants a pre-tripped signal.
func TestNewAbortSignalAborted(t *testing.T) {
	sig := NewAbortSignalAborted(StringValue(FromGoString("preset")))
	if !ToBoolean(sig.Get(FromGoString("aborted"))) {
		t.Fatal("a pre-aborted signal must report aborted")
	}
	if got := ToString(sig.Get(FromGoString("reason"))).ToGoString(); got != "preset" {
		t.Fatalf("want the preset reason, got %q", got)
	}
}
