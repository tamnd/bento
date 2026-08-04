package value

// This file is the runtime side of the process 'exit' event. Node fires exit
// when the event loop has drained and the process is about to leave, running
// every listener registered with process.on('exit', fn) synchronously, in the
// order they were registered. A compiled bento program runs them once at the very
// end of main, after the microtask checkpoint and the event loop, which is the
// same observable point: the synchronous body, every queued microtask, and every
// scheduled callback have finished and nothing remains but to exit.

// OnExit registers a process 'exit' listener, the runtime behind process.on('exit',
// fn). It is the same registry every other process event uses (processevents.go), so
// a listener registered through the lowerer's static path and one registered through
// the process object itself run in the one registration order.
func OnExit(fn Value) {
	OnProcessEvent("exit", fn)
}

// RunExitCallbacks runs every registered 'exit' listener once, in registration
// order, the drain the compiled main appends as its final statement when the
// program registered any listener. It is what lets common.mustCall, which asserts
// on exit that a wrapped function ran the expected number of times, observe the
// run. The exit status is 0 here, the status a program that ran off the end of main
// leaves with; process.exit passes its own.
func RunExitCallbacks() {
	RunExitCallbacksWithCode(0)
}

// RunExitCallbacksWithCode runs the 'exit' listeners with the status the process is
// leaving with, which Node passes each listener as its one argument. A listener that
// declares no parameter ignores it, so the argument costs nothing where it is not
// wanted and is there for the common `process.on('exit', (code) => ...)` that asserts
// the program left cleanly.
func RunExitCallbacksWithCode(code int) {
	EmitProcessEvent("exit", Number(float64(code)))
}
