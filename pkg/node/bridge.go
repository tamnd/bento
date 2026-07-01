package node

import "github.com/tamnd/bento/pkg/engine"

// LoopHost is the slice of the event loop the networking modules need. The loop
// satisfies it directly; the interface keeps this package from importing the
// loop package and lets tests drive the bridge with a fake.
//
// Post runs a closure on the loop goroutine, the only goroutine allowed to touch
// the engine. AddRef and Unref keep the loop alive while a listener or a socket
// is open, mirroring how Node stays running as long as a server is bound.
type LoopHost interface {
	Post(task func())
	AddRef()
	Unref()
}

// netBridge is the shared plumbing between blocking Go I/O and single-threaded
// JavaScript. Every networking host module embeds it. Blocking work runs on a
// pool goroutine through pool; results cross back to JavaScript through emit,
// which calls a JS global on the loop goroutine.
type netBridge struct {
	eng  engine.Engine
	loop LoopHost
}

// emit schedules a call to a JavaScript global on the loop goroutine. It is the
// only way a pool goroutine is allowed to reach the engine: it hands the call to
// the loop rather than invoking it inline, so the run-to-completion contract
// holds. Errors from the JS side are dispatched to handlers there, so a failed
// call here has nowhere useful to go and is dropped.
func (b *netBridge) emit(fn string, args ...any) {
	b.loop.Post(func() {
		_, _ = b.eng.Call(fn, args...)
	})
}

// pool runs a blocking task on its own goroutine, standing in for a libuv worker.
// The task must never touch the engine directly; it posts back through emit.
func (b *netBridge) pool(task func()) {
	go task()
}
