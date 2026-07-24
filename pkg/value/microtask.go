package value

// This file is the runtime side of the WHATWG global queueMicrotask. Node exposes
// queueMicrotask(fn) as a global that runs fn at the next microtask checkpoint: after
// the current synchronous run finishes and before the next macrotask, in the same
// queue and the same first-in-first-out order as promise reactions. A compiled bento
// program already has that queue for promises (promise.go), drained once at the end
// of main, so queueMicrotask is a thin wrapper that enqueues onto it rather than a
// second scheduler.

// QueueMicrotask schedules fn on the microtask queue, the runtime behind the global
// queueMicrotask(fn). The callback is held as a value and invoked with no arguments
// when the queue drains, so a queueMicrotask callback and a promise then callback
// interleave in enqueue order, the ordering the language gives two microtasks. The
// compiled main drains the queue as part of its end-of-run checkpoint whenever the
// program used queueMicrotask or a promise.
func QueueMicrotask(fn Value) {
	enqueueMicrotask(func() { fn.Call() })
}
