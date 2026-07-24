package value

// This file is the runtime side of the process 'exit' event. Node fires exit
// when the event loop has drained and the process is about to leave, running
// every listener registered with process.on('exit', fn) synchronously, in the
// order they were registered. A compiled bento program has no event loop to
// drain, so the compiled main runs the registered callbacks once at its very
// end, after the microtask checkpoint, which is the same observable point: the
// synchronous body and every queued microtask have finished and nothing remains
// but to exit.

// exitCallbacks holds the process 'exit' listeners in registration order. It is a
// package-level slice rather than per-call state because a listener registered in
// one function and the end-of-main drain that runs it are different call sites
// that must reach the same list, the same way Node's single process object holds
// its listeners.
var exitCallbacks []Value

// OnExit registers a process 'exit' listener, the runtime behind process.on('exit',
// fn). The listener is appended so the callbacks run in registration order, the
// order Node runs its exit listeners. The listener is held as a value so a closure
// that captured its module's state runs with that state at exit time.
func OnExit(fn Value) {
	exitCallbacks = append(exitCallbacks, fn)
}

// RunExitCallbacks runs every registered 'exit' listener once, in registration
// order, the drain the compiled main appends as its final statement when the
// program registered any listener. It is what lets common.mustCall, which asserts
// on exit that a wrapped function ran the expected number of times, observe the
// run. Each listener is called with no arguments, matching Node, which passes the
// exit code only to a listener that declares the parameter; the covered surface
// takes none.
func RunExitCallbacks() {
	for _, fn := range exitCallbacks {
		fn.Call()
	}
}
