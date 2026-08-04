package value

import "strings"

// This file is the runtime side of process.on. Node's process is an EventEmitter, and
// a test registers a listener on it for one of a handful of reasons: to assert at exit
// that something ran (exit), to do one last thing while the loop is still turning
// (beforeExit), to catch what escaped every try (uncaughtException), or to see a
// promise rejection nobody handled (unhandledRejection). Those four are the events a
// compiled bento program can raise, and this file raises them.
//
// Every other event registers and never fires, which is what Node does too when
// nothing raises it: a program with no IPC channel never sees message or disconnect,
// and a program that emits no warning never sees warning. Registering is not the same
// as promising to deliver. The one place that reasoning does not hold is a signal,
// where the host really can raise the event and a listener also changes what the
// default disposition does, so a signal listener is refused at lowering rather than
// accepted here and quietly dropped.
//
// The listeners live in one package-level registry because a listener registered in
// one function and the drain that runs it are different call sites that must reach the
// same list, the same way Node's single process object holds its listeners.

// processListeners holds each event's listeners in registration order. Keyed by the
// event name, which is what process.on takes; the order inside an event is the order
// Node runs them in.
var processListeners = map[string][]Value{}

// OnProcessEvent registers a process listener, the runtime behind process.on(event,
// fn). The listener is held as a value so a closure that captured its module's state
// runs with that state when the event fires.
func OnProcessEvent(event string, fn Value) {
	processListeners[event] = append(processListeners[event], fn)
}

// HasProcessListener reports whether any listener is registered for an event. The
// uncaught-error and unhandled-rejection reporters ask before printing, since a
// program that registered a handler for either has said it wants to deal with the
// condition itself rather than crash on it.
func HasProcessListener(event string) bool {
	return len(processListeners[event]) > 0
}

// EmitProcessEvent runs every listener registered for an event, in registration order,
// and reports whether there was one. The listeners are copied first so a listener that
// registers another does not extend the batch in progress, which matches Node, where
// emit takes the listener array at the point of the emit.
func EmitProcessEvent(event string, args ...Value) bool {
	ls := processListeners[event]
	if len(ls) == 0 {
		return false
	}
	batch := make([]Value, len(ls))
	copy(batch, ls)
	for _, fn := range batch {
		fn.Call(args...)
	}
	return true
}

// IsSignalEvent reports whether an event name is a signal, the one family a listener
// cannot merely be registered for. A signal name is upper case and starts with SIG,
// which is how Node's own emitter tells one from an ordinary event.
func IsSignalEvent(event string) bool {
	return len(event) > 3 && event[:3] == "SIG" && event == strings.ToUpper(event)
}

// RunBeforeExit fires the beforeExit event, the point Node reaches when the loop has
// drained and the process is about to leave but has not left yet. A listener may
// schedule more work, which is the whole reason the event exists, so the loop turns
// again after the listeners run and the event fires once more when that work is done.
// A program that scheduled nothing new leaves after one pass.
//
// It is not called from process.exit, matching Node, which skips beforeExit entirely
// when the program asked to leave rather than ran out of things to do.
func RunBeforeExit() {
	for {
		if !EmitProcessEvent("beforeExit", Number(0)) {
			return
		}
		if !hasPendingWork() {
			return
		}
		RunEventLoop()
	}
}
