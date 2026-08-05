package value

import "strings"

// This file is the runtime side of process as an EventEmitter. Node's process is one,
// and a test registers a listener on it for one of a handful of reasons: to assert at
// exit that something ran (exit), to do one last thing while the loop is still turning
// (beforeExit), to catch what escaped every try (uncaughtException), to see a promise
// rejection nobody handled (unhandledRejection), or to take a signal the host sent
// (SIGINT and its siblings, delivered from signals.go).
//
// Everything else registers and never fires, which is what Node does too when nothing
// raises it: a program with no IPC channel never sees message or disconnect, and a
// program that emits no warning never sees warning. Registering is not the same as
// promising to deliver.
//
// The listeners live in one package-level registry because a listener registered in
// one function and the drain that runs it are different call sites that must reach the
// same list, the same way Node's single process object holds its listeners.
//
// An event is named by a string or by a symbol, since an emitter takes either and a
// test that wants an event no other code can raise reaches for a symbol. So the
// registry is keyed by a small struct rather than by a string, and a symbol key holds
// the symbol itself, which is compared by identity the way a symbol property key is.

// eventKey identifies an event in the registry. Exactly one of the two fields is
// meaningful: sym is nil for a string-named event and holds the symbol otherwise, so
// the zero-value name of a symbol-keyed event never collides with a real string name.
type eventKey struct {
	name string
	sym  *Symbol
}

// processListener is one registration: what runs, what removeListener compares
// against, and whether the registration ends after the first call. once wraps its
// listener, and Node's removeListener still finds it by the function the program
// passed, so the original is kept beside the wrapper rather than in place of it.
type processListener struct {
	fn   Value
	once bool
}

// processListeners holds each event's listeners in registration order. processEvents
// keeps the events themselves in the order their first listener arrived, since
// eventNames answers a list and a map has no order to answer with.
var (
	processListeners = map[eventKey][]processListener{}
	processEvents    []eventKey
)

// OnProcessEvent registers a process listener, the runtime behind process.on(event,
// fn) for an event the lowerer read as a string literal. The listener is held as a
// value so a closure that captured its module's state runs with that state when the
// event fires.
func OnProcessEvent(event string, fn Value) {
	addProcessListener(eventKey{name: event}, fn, false, false)
}

// OnProcessEventNamed registers a listener for an event named by a value rather than
// by a literal, the runtime behind process.on(name, fn) where the name is computed or
// is a symbol. The listener is checked here rather than at compile time, since a value
// the checker could not see a call signature on may still be a function at run time,
// and a value that is not one is Node's ERR_INVALID_ARG_TYPE.
func OnProcessEventNamed(event, fn Value) Value {
	addProcessListener(eventKeyOf(event), requireListener(fn), false, false)
	return ProcessValue()
}

// OnceProcessEvent registers a listener that runs at most once, the runtime behind
// process.once. The registration is dropped as the listener is called rather than
// after it returns, so a listener that emits the same event again does not re-enter
// itself.
func OnceProcessEvent(event, fn Value) Value {
	addProcessListener(eventKeyOf(event), requireListener(fn), true, false)
	return ProcessValue()
}

// PrependProcessListener puts a listener at the front of an event's list, the runtime
// behind process.prependListener. A program uses it to see an event before a listener
// something else installed, which is the whole reason the method exists.
func PrependProcessListener(event, fn Value) Value {
	addProcessListener(eventKeyOf(event), requireListener(fn), false, true)
	return ProcessValue()
}

// PrependOnceProcessListener is prependListener and once at the same time, the runtime
// behind process.prependOnceListener.
func PrependOnceProcessListener(event, fn Value) Value {
	addProcessListener(eventKeyOf(event), requireListener(fn), true, true)
	return ProcessValue()
}

// addProcessListener is the one registration path. It arms the host signal when the
// event is one, so the delivery machinery starts exactly when a program first asks to
// hear about it, then records the event's order on first use and places the listener at
// the front or the back.
//
// The arm comes first because it is the step that can fail: a listener for a signal
// nothing can catch is refused, and Node refuses it before the registration rather than
// after, so a program that caught the throw does not find the listener registered
// anyway.
func addProcessListener(key eventKey, fn Value, once, prepend bool) {
	if key.sym == nil && IsSignalEvent(key.name) {
		armSignal(key.name)
	}
	if _, seen := processListeners[key]; !seen {
		processEvents = append(processEvents, key)
	}
	entry := processListener{fn: fn, once: once}
	if prepend {
		processListeners[key] = append([]processListener{entry}, processListeners[key]...)
	} else {
		processListeners[key] = append(processListeners[key], entry)
	}
}

// RemoveProcessListener drops one registration for an event, the runtime behind
// process.removeListener and its alias off. Node removes the most recently added
// match, and a once wrapper is found by the function the program passed rather than by
// the wrapper, which is why the listener a registration holds is the original.
func RemoveProcessListener(event, fn Value) Value {
	key := eventKeyOf(event)
	ls := processListeners[key]
	for i := len(ls) - 1; i >= 0; i-- {
		if !SameValueZero(ls[i].fn, fn) {
			continue
		}
		processListeners[key] = append(append([]processListener{}, ls[:i]...), ls[i+1:]...)
		if len(processListeners[key]) == 0 {
			dropEvent(key)
		}
		break
	}
	return ProcessValue()
}

// RemoveAllProcessListeners drops every listener for an event, or every listener for
// every event when it is called with no argument, the runtime behind
// process.removeAllListeners. A program calls it on a signal to hand the signal back
// to its default disposition, which is why the disarm matters as much as the drop:
// after this, a SIGINT terminates the process again.
func RemoveAllProcessListeners(args ...Value) Value {
	if len(args) == 0 || args[0].Kind() == KindUndefined {
		for _, key := range append([]eventKey{}, processEvents...) {
			dropEvent(key)
		}
		return ProcessValue()
	}
	dropEvent(eventKeyOf(args[0]))
	return ProcessValue()
}

// dropEvent forgets an event with no listeners left. The registry entry goes, so the
// event does not answer eventNames, and the order list goes with it, so registering
// again puts the event back at the end the way a fresh registration does. A signal is
// handed back to the host here, which is what restores the default disposition: after
// this, a SIGINT terminates the process again.
func dropEvent(key eventKey) {
	delete(processListeners, key)
	for i, e := range processEvents {
		if e == key {
			processEvents = append(append([]eventKey{}, processEvents[:i]...), processEvents[i+1:]...)
			break
		}
	}
	if key.sym == nil && IsSignalEvent(key.name) {
		disarmSignal(key.name)
	}
}

// ProcessListeners answers the listeners registered for an event, the runtime behind
// process.listeners. Node answers a copy, which is what lets the caller in Node's own
// test suite save a list, remove everything, and put the saved listeners back.
func ProcessListeners(event Value) Value {
	ls := processListeners[eventKeyOf(event)]
	out := make([]Value, 0, len(ls))
	for _, l := range ls {
		out = append(out, l.fn)
	}
	return NewArrayValue(out)
}

// ProcessListenerCount answers how many listeners an event has, the runtime behind
// process.listenerCount.
func ProcessListenerCount(event Value) float64 {
	return float64(len(processListeners[eventKeyOf(event)]))
}

// ProcessEventNames answers the events with at least one listener, in the order their
// first listener arrived, the runtime behind process.eventNames. A symbol-named event
// answers as the symbol itself, since that is the only value that can be handed back
// to on or emit and reach the same list.
func ProcessEventNames() Value {
	out := make([]Value, 0, len(processEvents))
	for _, key := range processEvents {
		if len(processListeners[key]) == 0 {
			continue
		}
		if key.sym != nil {
			out = append(out, symbolValue(key.sym))
			continue
		}
		out = append(out, StringValue(FromGoString(key.name)))
	}
	return NewArrayValue(out)
}

// HasProcessListener reports whether any listener is registered for an event. The
// uncaught-error and unhandled-rejection reporters ask before printing, since a
// program that registered a handler for either has said it wants to deal with the
// condition itself rather than crash on it.
func HasProcessListener(event string) bool {
	return len(processListeners[eventKey{name: event}]) > 0
}

// EmitProcessEvent runs every listener registered for an event, in registration order,
// and reports whether there was one. The listeners are copied first so a listener that
// registers another does not extend the batch in progress, which matches Node, where
// emit takes the listener array at the point of the emit.
func EmitProcessEvent(event string, args ...Value) bool {
	return emitProcessEvent(eventKey{name: event}, args)
}

// EmitProcessEventNamed is EmitProcessEvent for an event named by a value, the runtime
// behind process.emit. It answers the boolean emit answers: whether anything was
// listening.
func EmitProcessEventNamed(event Value, args ...Value) Value {
	return Bool(emitProcessEvent(eventKeyOf(event), args))
}

// emitProcessEvent is the one delivery path. A once registration is dropped before its
// listener runs, so a listener that emits the same event again finds itself already
// gone rather than running twice.
func emitProcessEvent(key eventKey, args []Value) bool {
	ls := processListeners[key]
	if len(ls) == 0 {
		return false
	}
	batch := make([]processListener, len(ls))
	copy(batch, ls)
	for _, l := range batch {
		if l.once {
			RemoveProcessListener(keyValue(key), l.fn)
		}
		l.fn.Call(args...)
	}
	return true
}

// eventKeyOf reads the registry key out of the value an emitter method was handed. A
// symbol keeps its identity, since two symbols with the same description are two
// events; anything else is coerced to a string, which is what an emitter does with a
// number or a boolean used as an event name.
func eventKeyOf(event Value) eventKey {
	if event.Kind() == KindSymbol {
		return eventKey{sym: event.symbol()}
	}
	return eventKey{name: ToString(event).ToGoString()}
}

// keyValue is eventKeyOf backwards, for the paths that hold a key and have to call one
// of the value-taking entry points with it.
func keyValue(key eventKey) Value {
	if key.sym != nil {
		return symbolValue(key.sym)
	}
	return StringValue(FromGoString(key.name))
}

// requireListener refuses a listener that is not callable, the check Node's emitter
// makes before it registers anything. Registering a non-function would fail later at
// the emit, in a stack that no longer names the registration, so the throw belongs
// here; the code is the one a program branches on.
func requireListener(fn Value) Value {
	if fn.Kind() != KindFunc {
		Throw(invalidArgType("listener", "function", fn))
	}
	return fn
}

// IsSignalEvent reports whether an event name is a signal. A signal name is upper case
// and starts with SIG, which is how Node's own emitter tells one from an ordinary
// event; whether the host actually has a signal by that name is a separate question,
// and one signals.go answers, since a program may name a signal this platform does not
// define and Node treats that as an ordinary event.
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
