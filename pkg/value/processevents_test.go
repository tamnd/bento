package value

import (
	"strconv"
	"testing"
)

// The process event registry is a package-level map, so each of these registers
// under an event name no other test uses. That keeps them independent without
// making the registry itself resettable, which nothing in a compiled program wants.

// TestEmitProcessEventCallsEveryListenerInOrder pins the ordering Node's emitter
// gives: listeners run in registration order and each one sees the emitted argument.
func TestEmitProcessEventCallsEveryListenerInOrder(t *testing.T) {
	var seen []string
	OnProcessEvent("test-order", NewFunc(func(args []Value) Value {
		seen = append(seen, "a"+ToString(Arg(args, 0)).ToGoString())
		return Undefined
	}))
	OnProcessEvent("test-order", NewFunc(func(args []Value) Value {
		seen = append(seen, "b"+ToString(Arg(args, 0)).ToGoString())
		return Undefined
	}))
	if !EmitProcessEvent("test-order", Number(7)) {
		t.Fatal("emit reported no listeners, want two")
	}
	if len(seen) != 2 || seen[0] != "a7" || seen[1] != "b7" {
		t.Fatalf("listeners ran as %v, want [a7 b7]", seen)
	}
}

// TestEmitProcessEventWithNoListenersReportsSo pins the return value the callers
// lean on: an emit nobody registered for is how ReportUncaught and the unhandled
// rejection report decide to fall back to their own default behaviour.
func TestEmitProcessEventWithNoListenersReportsSo(t *testing.T) {
	if EmitProcessEvent("test-nobody-listening") {
		t.Fatal("emit reported a listener for an event nothing registered for")
	}
	if HasProcessListener("test-nobody-listening") {
		t.Fatal("HasProcessListener reported a listener for an unregistered event")
	}
}

// TestEmitProcessEventSeesOnlyTheListenersRegisteredWhenItStarted pins that a
// listener registering another listener for the same event does not have the new one
// run in the same emit. beforeExit relies on this: it emits, then loops, so a
// listener added during an emit is picked up by the next round rather than extending
// the current one into a walk over a slice that is growing underneath it.
func TestEmitProcessEventSeesOnlyTheListenersRegisteredWhenItStarted(t *testing.T) {
	runs := 0
	OnProcessEvent("test-reentrant", NewFunc(func(args []Value) Value {
		runs++
		if runs == 1 {
			OnProcessEvent("test-reentrant", NewFunc(func(args []Value) Value {
				runs++
				return Undefined
			}))
		}
		return Undefined
	}))
	EmitProcessEvent("test-reentrant")
	if runs != 1 {
		t.Fatalf("ran %d listeners in the first emit, want 1", runs)
	}
	EmitProcessEvent("test-reentrant")
	if runs != 3 {
		t.Fatalf("ran %d listeners across both emits, want 3", runs)
	}
}

// TestIsSignalEventNamesOnlySignals pins the test the lowerer refuses on. A signal
// name is SIG followed by an all-caps mnemonic, which no other process event looks
// like, so the check does not have to carry the list of names a platform defines.
func TestIsSignalEventNamesOnlySignals(t *testing.T) {
	for _, name := range []string{"SIGINT", "SIGTERM", "SIGHUP", "SIGUSR2"} {
		if !IsSignalEvent(name) {
			t.Errorf("%s is not read as a signal", name)
		}
	}
	for _, name := range []string{"exit", "beforeExit", "uncaughtException", "message", "SIG", "signal"} {
		if IsSignalEvent(name) {
			t.Errorf("%s is read as a signal", name)
		}
	}
}

// TestOnExitRegistersAnExitListener pins that the older exit-callback entry point and
// the event registry are the same list, so a program mixing the two orders them the
// way it wrote them.
func TestOnExitRegistersAnExitListener(t *testing.T) {
	if HasProcessListener("exit") {
		t.Skip("another test in this package already registered an exit listener")
	}
	OnExit(NewFunc(func(args []Value) Value { return Undefined }))
	if !HasProcessListener("exit") {
		t.Fatal("OnExit did not register against the exit event")
	}
}

// TestOnceProcessEventRunsOneTime pins the once registration: it is dropped as its
// listener is called rather than after it returns, so a listener that emits the same
// event again does not re-enter itself.
func TestOnceProcessEventRunsOneTime(t *testing.T) {
	runs := 0
	OnceProcessEvent(StringValue(FromGoString("test-once")), NewFunc(func(args []Value) Value {
		runs++
		EmitProcessEvent("test-once")
		return Undefined
	}))
	EmitProcessEvent("test-once")
	if runs != 1 {
		t.Fatalf("ran %d times, want 1", runs)
	}
	if EmitProcessEvent("test-once") {
		t.Fatal("the once registration is still there after it fired")
	}
}

// TestPrependProcessListenerRunsFirst pins why the method exists: a program uses it to
// see an event ahead of a listener something else already installed.
func TestPrependProcessListenerRunsFirst(t *testing.T) {
	var seen []string
	OnProcessEvent("test-prepend", NewFunc(func(args []Value) Value {
		seen = append(seen, "second")
		return Undefined
	}))
	PrependProcessListener(StringValue(FromGoString("test-prepend")), NewFunc(func(args []Value) Value {
		seen = append(seen, "first")
		return Undefined
	}))
	EmitProcessEvent("test-prepend")
	if len(seen) != 2 || seen[0] != "first" || seen[1] != "second" {
		t.Fatalf("listeners ran as %v, want [first second]", seen)
	}
}

// TestRemoveProcessListenerDropsTheMostRecentMatch pins Node's rule for a function
// registered twice: removeListener takes one registration away, the last one added,
// rather than every registration that names the same function.
func TestRemoveProcessListenerDropsTheMostRecentMatch(t *testing.T) {
	runs := 0
	fn := NewFunc(func(args []Value) Value {
		runs++
		return Undefined
	})
	event := StringValue(FromGoString("test-remove"))
	OnProcessEventNamed(event, fn)
	OnProcessEventNamed(event, fn)
	RemoveProcessListener(event, fn)
	if n := ProcessListenerCount(event); n != 1 {
		t.Fatalf("%v listeners left, want 1", n)
	}
	EmitProcessEvent("test-remove")
	if runs != 1 {
		t.Fatalf("ran %d listeners, want 1", runs)
	}
}

// TestProcessListenersAnswersACopy pins that the answer is a copy, which is what lets
// Node's own tests save the listeners for an event, remove them all, and put them back.
func TestProcessListenersAnswersACopy(t *testing.T) {
	event := StringValue(FromGoString("test-listeners"))
	fn := NewFunc(func(args []Value) Value { return Undefined })
	OnProcessEventNamed(event, fn)
	saved := ProcessListeners(event)
	RemoveAllProcessListeners(event)
	if n := ProcessListenerCount(event); n != 0 {
		t.Fatalf("%v listeners left after removeAllListeners, want 0", n)
	}
	if got := ToNumber(saved.Get(FromGoString("length"))); got != 1 {
		t.Fatalf("the saved list holds %v listeners, want 1", got)
	}
}

// TestProcessEventNamesKeepsRegistrationOrder pins that eventNames answers the events
// with a listener in the order their first listener arrived, and that an event whose
// listeners have all gone is not among them.
func TestProcessEventNamesKeepsRegistrationOrder(t *testing.T) {
	before := ToNumber(ProcessEventNames().Get(FromGoString("length")))
	fn := NewFunc(func(args []Value) Value { return Undefined })
	OnProcessEventNamed(StringValue(FromGoString("test-names-a")), fn)
	OnProcessEventNamed(StringValue(FromGoString("test-names-b")), fn)
	names := ProcessEventNames()
	n := ToNumber(names.Get(FromGoString("length")))
	if n != before+2 {
		t.Fatalf("eventNames answers %v names, want %v", n, before+2)
	}
	last := ToString(names.Get(FromGoString(strconv.Itoa(int(n) - 1)))).ToGoString()
	first := ToString(names.Get(FromGoString(strconv.Itoa(int(n) - 2)))).ToGoString()
	if first != "test-names-a" || last != "test-names-b" {
		t.Fatalf("the two new names read %q then %q, want test-names-a then test-names-b", first, last)
	}
	RemoveAllProcessListeners(StringValue(FromGoString("test-names-a")))
	RemoveAllProcessListeners(StringValue(FromGoString("test-names-b")))
	if got := ToNumber(ProcessEventNames().Get(FromGoString("length"))); got != before {
		t.Fatalf("eventNames answers %v names after the removals, want %v", got, before)
	}
}

// TestProcessEventKeyedBySymbol pins that a symbol names its own event: two symbols
// with the same description are two events, and the name eventNames answers is the
// symbol itself, the only value that can reach the same list again.
func TestProcessEventKeyedBySymbol(t *testing.T) {
	one := NewSymbol(FromGoString("tag"))
	two := NewSymbol(FromGoString("tag"))
	runs := 0
	OnProcessEventNamed(one, NewFunc(func(args []Value) Value {
		runs++
		return Undefined
	}))
	if ToBoolean(EmitProcessEventNamed(two)) {
		t.Fatal("the second symbol reached the first symbol's listeners")
	}
	if !ToBoolean(EmitProcessEventNamed(one)) {
		t.Fatal("the symbol event reported no listener")
	}
	if runs != 1 {
		t.Fatalf("ran %d listeners, want 1", runs)
	}
	RemoveAllProcessListeners(one)
}

// TestOnProcessEventNamedRefusesANonFunction pins the ERR_INVALID_ARG_TYPE Node throws
// at registration, and that nothing is registered when it does.
func TestOnProcessEventNamedRefusesANonFunction(t *testing.T) {
	event := StringValue(FromGoString("test-bad-listener"))
	code, msg := catchThrownCode(func() {
		OnProcessEventNamed(event, StringValue(FromGoString("nope")))
	})
	if code != "ERR_INVALID_ARG_TYPE" {
		t.Fatalf("code = %q, want ERR_INVALID_ARG_TYPE (message %q)", code, msg)
	}
	want := `The "listener" argument must be of type function. Received type string ('nope')`
	if msg != want {
		t.Fatalf("message = %q, want %q", msg, want)
	}
	if n := ProcessListenerCount(event); n != 0 {
		t.Fatalf("%v listeners registered, want 0", n)
	}
}

// catchThrownCode runs fn and reads the code and message off whatever it threw, the
// two things a program branches on. Both are empty when fn returned without throwing.
func catchThrownCode(fn func()) (code, msg string) {
	defer func() {
		if rec := recover(); rec != nil {
			v := Caught(rec).ToValue()
			code = ToString(v.Get(FromGoString("code"))).ToGoString()
			msg = ToString(v.Get(FromGoString("message"))).ToGoString()
		}
	}()
	fn()
	return "", ""
}
