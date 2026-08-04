package value

import "testing"

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
