package value

import (
	"sort"
	"time"
)

// This file is the runtime side of the timer globals: setTimeout, setInterval,
// setImmediate, and the three clear functions that cancel them. They are the
// macrotask half of the event loop, where queueMicrotask and a promise reaction
// (promise.go) are the microtask half. A compiled bento program used to have only
// the microtask queue, drained once at the end of main, because nothing could make
// a promise pending; a Node program schedules work for later as a matter of course,
// so the AOT path needs a loop that keeps running after the synchronous body ends
// until the scheduled work is done.
//
// The loop this file runs is the shape libuv gives Node, cut down to the phases a
// compiled bento binary has work for. One turn runs the timers whose deadline has
// passed, then the immediates queued before the turn began, draining the microtask
// queue after every callback the way the language runs a microtask checkpoint
// between macrotasks. With no callback ready and a timer still pending, the loop
// sleeps until that timer's deadline rather than spinning, so a program that waits
// a second costs a second of sleep and no CPU.
//
// A callback that schedules more work keeps the loop alive, and the loop ends when
// nothing is left scheduled, which is when Node's process exits on its own.
//
// A scheduling call returns a number, the timer's id, which is what the standard
// library's setTimeout declaration says it returns and so what the checker types the
// binding as. Node instead returns a Timeout object. The four methods that object
// carries, ref, unref, hasRef, and refresh, are offered here as operations on the id:
// the lowerer turns a call of one of them on a number receiver into the matching
// TimerRef, TimerUnref, TimerHasRef, or TimerRefresh below. That is the whole of what
// a program does with the object, and every one of them has a real effect here rather
// than a stand-in. What is still divergent is the handle itself: typeof reports
// "number" where Node reports "object", and there is no Timeout to compare or print.

// timerHandle is one scheduled callback, the state behind the id a timer global
// returns. The deadline is when a timeout fires; period is non-zero only for an
// interval, which re-arms itself by that much after each run. An immediate carries no
// deadline at all, since it runs in the next turn's check phase rather than at a time.
// The sequence number breaks the tie between two timers that share a deadline, so they
// fire in the order they were scheduled the way Node orders them.
//
// The delay is kept alongside the deadline because refresh re-arms a timeout for the
// delay it was scheduled with, which a one-shot timeout does not otherwise record.
// unrefed is Node's ref flag inverted: an unrefed timer still fires if the loop is
// running for some other reason, but it no longer keeps the process alive on its own.
type timerHandle struct {
	id        float64
	fn        Value
	args      []Value
	deadline  time.Time
	delay     time.Duration
	period    time.Duration
	immediate bool
	seq       uint64
	cleared   bool
	unrefed   bool
}

// The scheduler state. pendingTimers is kept sorted by deadline and then by sequence
// number, so the timer that fires next is always the first element and the loop can
// read the next deadline without a scan. pendingImmediates is a plain queue, since
// immediates carry no deadline and run first-in-first-out. timersByID resolves the id a
// clear call hands back to the scheduled callback, which is what lets the handle be a
// plain number rather than an object holding the state directly. One set of
// package-level state matches the single event loop a program runs.
var (
	pendingTimers     []*timerHandle
	pendingImmediates []*timerHandle
	timersByID        = map[float64]*timerHandle{}
	timerSeq          uint64
	nextTimerID       float64 = 1
)

// SetTimeout schedules fn to run once after delay milliseconds, the runtime behind
// the global setTimeout. Extra arguments are held and passed to the callback when it
// runs, the way Node forwards them. It returns the timer's id, the handle a program
// passes to clearTimeout to cancel the callback before it fires.
func SetTimeout(fn Value, delay Value, args ...Value) float64 {
	return scheduleTimeout(fn, delay, false, args)
}

// SetInterval schedules fn to run every delay milliseconds until it is cancelled, the
// runtime behind the global setInterval. It returns an id the same way setTimeout does;
// the difference is that the timer re-arms after each run, so a program that never
// clears it keeps the loop alive forever, as it does under Node.
func SetInterval(fn Value, delay Value, args ...Value) float64 {
	return scheduleTimeout(fn, delay, true, args)
}

// scheduleTimeout registers a one-shot or repeating timer and returns its id. The
// caller passes a repeat flag rather than a period because the period is the clamped
// delay in both cases and clamping happens here: Node coerces the delay to a number and
// forces anything below 1ms, non-finite, or past the 32-bit millisecond ceiling to 1ms,
// so setTimeout(fn) and setTimeout(fn, 0) both mean "as soon as the next turn comes
// round" rather than "now, inline".
func scheduleTimeout(fn Value, delay Value, repeat bool, args []Value) float64 {
	ms := clampTimerDelay(delay)
	t := &timerHandle{
		id:       newTimerID(),
		fn:       fn,
		args:     args,
		deadline: time.Now().Add(ms),
		delay:    ms,
		seq:      newTimerSeq(),
	}
	if repeat {
		t.period = ms
	}
	timersByID[t.id] = t
	scheduleTimer(t)
	return t.id
}

// SetImmediate schedules fn to run in the next turn's check phase, the runtime behind
// the global setImmediate. It is not a zero-delay timeout: an immediate runs before
// any timer the same turn re-arms, and an immediate scheduled from inside an immediate
// waits for the following turn rather than running in the current batch, which is what
// keeps a self-rescheduling immediate from starving the timers.
func SetImmediate(fn Value, args ...Value) float64 {
	t := &timerHandle{
		id:        newTimerID(),
		fn:        fn,
		args:      args,
		immediate: true,
		seq:       newTimerSeq(),
	}
	timersByID[t.id] = t
	pendingImmediates = append(pendingImmediates, t)
	return t.id
}

// ClearTimer cancels the callback a timer id stands for, the runtime behind
// clearTimeout, clearInterval, and clearImmediate. One function backs all three because
// the recorded timer already knows which kind it is, so the three differ only in the
// handle they are documented to take. An id that names no live timer, whether because
// it was already cleared, already fired, or never was a timer id, is ignored rather
// than treated as an error, since clearing a finished timeout is normal in Node.
func ClearTimer(handle Value) {
	t, ok := timersByID[ToNumber(handle)]
	if !ok || t.cleared {
		return
	}
	t.cleared = true
	forgetTimer(t)
	if t.immediate {
		pendingImmediates = removeTimer(pendingImmediates, t)
		return
	}
	pendingTimers = removeTimer(pendingTimers, t)
}

// TimerUnref takes a timer out of the count that keeps the process alive, the runtime
// behind timeout.unref(). The callback is not cancelled: an unrefed timer still fires
// if the loop is turning for some other reason, which is the difference between unref
// and clearTimeout and the whole reason a program reaches for it. A program whose only
// remaining work is unrefed exits instead of waiting, which is what
// setTimeout(mustNotCall(), 1000).unref() is asserting.
//
// It returns the handle so a call can be chained, the way Node's returns the Timeout.
// An id naming no live timer is ignored, since unrefing a timer that already fired is
// as normal as clearing one.
func TimerUnref(handle Value) float64 {
	return setTimerRef(handle, false)
}

// TimerRef puts a timer back into the count that keeps the process alive, the runtime
// behind timeout.ref() and the undo of TimerUnref. A timer is refed when it is
// scheduled, so this only matters after an unref.
func TimerRef(handle Value) float64 {
	return setTimerRef(handle, true)
}

// setTimerRef is the body both ref calls share, returning the id so the call can be
// chained.
func setTimerRef(handle Value, refed bool) float64 {
	id := ToNumber(handle)
	if t, ok := timersByID[id]; ok {
		t.unrefed = !refed
	}
	return id
}

// TimerHasRef reports whether a timer is refed, the runtime behind timeout.hasRef().
// It reads the ref flag and nothing else, which is what Node's Timeout does: a timeout
// that has already fired or been cleared still answers true, because clearing a timer
// does not unref it. A timer this runtime has forgotten therefore answers true as well,
// and forgetting only ever happens to a refed one; see retireTimer.
func TimerHasRef(handle Value) bool {
	t, ok := timersByID[ToNumber(handle)]
	return !ok || !t.unrefed
}

// TimerRefresh re-arms a timeout for its original delay counted from now, the runtime
// behind timeout.refresh(). It is what a program with an idle timeout calls on every
// bit of activity, so the timeout only fires after a real gap rather than a fixed time
// after it was created.
//
// The call a program most often makes is from inside the timeout's own callback, which
// is how an idle timeout keeps itself alive; a timer is retired only after its callback
// returns, so the handle is still live when that call arrives and the refresh re-arms it
// rather than dropping. An id naming a timer that has been cleared, or one this process
// never handed out, is ignored. An immediate has no deadline to move, so it is ignored
// too, which is what Node's Immediate does by not carrying the method at all.
func TimerRefresh(handle Value) float64 {
	id := ToNumber(handle)
	t, ok := timersByID[id]
	if !ok || t.cleared || t.immediate {
		return id
	}
	pendingTimers = removeTimer(pendingTimers, t)
	t.deadline = time.Now().Add(t.delay)
	scheduleTimer(t)
	return id
}

// RunEventLoop runs scheduled callbacks until nothing is left scheduled, the loop the
// assembled main enters after the synchronous body when the program used a timer. It
// opens with a microtask drain so a promise reaction the body queued runs before the
// first macrotask, the ordering the language gives, then turns the loop: due timers,
// then immediates, sleeping until the next deadline when neither has anything ready.
//
// Running out of scheduled work is what ends the loop, which is when Node's process
// exits on its own. A program with a live interval it never clears therefore runs
// forever here exactly as it does under Node, rather than exiting early and losing the
// callbacks it asked for.
//
// A signal that arrived is delivered at the top of a turn, ahead of the timers due in
// that same turn, which is where Node delivers one. A signal does not keep the loop
// turning on its own, again matching Node: a program whose only reason to stay is a
// listener for SIGINT has already left by the time the signal could arrive.
func RunEventLoop() {
	RunMicrotasks()
	for hasPendingWork() {
		drainSignals()
		if d := timeUntilNextDue(); d > 0 {
			waitForWork(d)
			drainSignals()
		}
		runDueTimers()
		runImmediates()
	}
	drainSignals()
}

// hasPendingWork reports whether anything that keeps the process alive is still
// scheduled, which is what keeps the loop turning. An unrefed timer does not count: it
// still fires while the loop is turning for some other reason, but on its own it lets
// the process leave, which is what unref means. The beforeExit event asks the same
// question after its listeners run, since a listener that scheduled something has put
// the process back to work.
func hasPendingWork() bool {
	return countRefed(pendingTimers)+countRefed(pendingImmediates) > 0
}

// countRefed counts the timers in a queue that still hold the process open.
func countRefed(queue []*timerHandle) int {
	n := 0
	for _, t := range queue {
		if !t.unrefed && !t.cleared {
			n++
		}
	}
	return n
}

// runDueTimers runs every timer whose deadline has passed, in deadline order. The due
// set is taken before any callback runs, so a timeout a callback schedules waits for a
// later turn instead of joining the batch in progress and starving the loop. An
// interval re-arms before its callback runs rather than after, so a clearInterval from
// inside the callback finds the handle in the queue and removes it; re-arming after
// would put it back after the clear had already looked.
func runDueTimers() {
	now := time.Now()
	var due []*timerHandle
	rest := make([]*timerHandle, 0, len(pendingTimers))
	for _, t := range pendingTimers {
		switch {
		case t.cleared:
			// dropped
		case t.deadline.After(now):
			rest = append(rest, t)
		default:
			due = append(due, t)
		}
	}
	pendingTimers = rest
	for _, t := range due {
		if t.cleared {
			continue
		}
		if t.period > 0 {
			t.deadline = time.Now().Add(t.period)
			scheduleTimer(t)
		}
		runTimerCallback(t)
	}
}

// runImmediates runs the immediates queued as of the start of the check phase. The
// queue is taken whole and replaced with an empty one, so an immediate scheduled from
// inside one of these callbacks lands in the next turn's batch. That is Node's rule,
// and it is what lets setImmediate be used to yield to the loop rather than to loop
// inline forever.
func runImmediates() {
	batch := pendingImmediates
	pendingImmediates = nil
	for _, t := range batch {
		if t.cleared {
			continue
		}
		runTimerCallback(t)
	}
}

// runTimerCallback invokes one scheduled callback with the arguments it was registered
// with, then drains the microtask queue. The drain is per callback, not per turn,
// because a microtask checkpoint falls between two macrotasks: a promise a timer
// callback resolves must run its reaction before the next timer fires, not after the
// whole batch.
func runTimerCallback(t *timerHandle) {
	t.fn.Call(t.args...)
	retireTimer(t)
	RunMicrotasks()
}

// timeUntilNextDue returns how long the loop may sleep before something is ready. A
// pending immediate is ready now, so the answer is zero; otherwise it is the time until
// the earliest timer's deadline, which the sorted queue keeps at the front. Sleeping
// that long rather than spinning is what makes a program that waits a second cost a
// second of sleep and no CPU.
func timeUntilNextDue() time.Duration {
	if len(pendingImmediates) > 0 {
		return 0
	}
	if len(pendingTimers) == 0 {
		return 0
	}
	return time.Until(pendingTimers[0].deadline)
}

// scheduleTimer inserts a timer into the pending queue at its place in deadline order,
// with the sequence number breaking a tie so two timers armed for the same instant fire
// in the order they were scheduled. Keeping the queue sorted on insert is what lets the
// loop read the next deadline off the front instead of scanning.
func scheduleTimer(t *timerHandle) {
	i := sort.Search(len(pendingTimers), func(i int) bool {
		o := pendingTimers[i]
		if o.deadline.Equal(t.deadline) {
			return o.seq > t.seq
		}
		return o.deadline.After(t.deadline)
	})
	pendingTimers = append(pendingTimers, nil)
	copy(pendingTimers[i+1:], pendingTimers[i:])
	pendingTimers[i] = t
}

// removeTimer returns the queue with one timer taken out, the cancel half of the
// scheduler. The handle is compared by identity, so clearing one of two timers armed
// for the same instant removes the one the program named.
func removeTimer(queue []*timerHandle, t *timerHandle) []*timerHandle {
	for i, o := range queue {
		if o == t {
			return append(queue[:i], queue[i+1:]...)
		}
	}
	return queue
}

// clampTimerDelay coerces a delay argument to the duration the timer waits, applying
// Node's clamp: a delay below 1ms, not a number, or above the 32-bit millisecond
// ceiling becomes 1ms. That is why setTimeout(fn, 0) still yields to the loop instead
// of running inline, and why a nonsense delay schedules soon rather than never.
func clampTimerDelay(delay Value) time.Duration {
	ms := ToNumber(delay)
	if !(ms >= 1) || ms > maxTimerDelayMS {
		ms = 1
	}
	return time.Duration(ms * float64(time.Millisecond))
}

// maxTimerDelayMS is the largest delay Node accepts, the signed 32-bit millisecond
// ceiling its timer layer stores a delay in. A longer delay is not an error; Node
// clamps it to 1ms and warns, and this runtime clamps it the same way.
const maxTimerDelayMS = 2147483647

// newTimerID returns the next timer identity. Node numbers its timers from 1 and never
// reuses a number within a process, which is what makes a handle safe to hold past the
// callback it scheduled.
func newTimerID() float64 {
	id := nextTimerID
	nextTimerID++
	return id
}

// newTimerSeq returns the next scheduling sequence number, the tiebreak between two
// timers armed for the same instant.
func newTimerSeq() uint64 {
	timerSeq++
	return timerSeq
}

// retireTimer drops a timer that has finished from the id registry, so a program that
// schedules in a loop does not grow the map for the life of the process. An interval is
// kept, since its id stays valid until the program clears it, and so is a timeout that
// its own callback re-armed with refresh, which is back in the queue with a new
// deadline and would otherwise be dropped from the registry it is still scheduled in.
func retireTimer(t *timerHandle) {
	if t.period > 0 || isPendingTimer(t) {
		return
	}
	forgetTimer(t)
}

// forgetTimer drops a finished timer from the id registry, unless it was unrefed. The
// exception is what keeps hasRef answering truthfully: a forgotten id reads as refed,
// which is right for a timer nobody unrefed and wrong for one somebody did, so the
// unrefed ones stay. Only a program that unrefs in a loop grows the map for it, and a
// program that unrefs is by definition holding the handle it unrefed.
func forgetTimer(t *timerHandle) {
	if t.unrefed {
		return
	}
	delete(timersByID, t.id)
}

// isPendingTimer reports whether a timer is in the queue, which after its callback has
// run means the callback put it back there.
func isPendingTimer(t *timerHandle) bool {
	for _, o := range pendingTimers {
		if o == t {
			return true
		}
	}
	return false
}
