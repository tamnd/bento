// Package loop is bento's event loop.
//
// JavaScript is single threaded with run-to-completion semantics, so the loop
// runs on one goroutine and never touches the engine from another. It keeps two
// queues that mirror the platform model: the microtask queue, drained by the
// engine after every unit of work, and the timer queue for setTimeout and
// friends. The process stays alive while timers are pending and returns once the
// queues drain, which matches how Node decides a program is done.
//
// Go's scheduler and netpoller stand in for libuv here. This milestone covers
// timers; I/O handles register through the same AddRef and Unref accounting so
// later milestones can keep the loop alive for sockets and file work without
// changing the shape of Run.
package loop

import (
	"container/heap"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tamnd/bento/pkg/engine"
)

// nowFunc is the clock the loop reads. It is a package variable so tests can
// swap it, and it defaults to the wall clock.
var nowFunc = time.Now

type timer struct {
	id       int64
	due      time.Time
	interval time.Duration
	repeat   bool
	index    int // maintained by the heap
}

// Loop schedules timers and drains microtasks against one engine.
type Loop struct {
	eng     engine.Engine
	pq      timerQueue
	byID    map[int64]*timer
	refs    int         // outstanding I/O handles keeping the loop alive
	stopped atomic.Bool // set when a hard stop is requested, possibly off-loop
	// mu guards pending, the only state other goroutines touch. Everything else
	// on the loop is owned by the loop goroutine and needs no lock.
	mu      sync.Mutex
	pending []func()
	// wake carries a single coalesced signal that pending has work, so Run stops
	// waiting on a timer and drains it. A buffer of one means a post that races
	// with Run deciding to sleep is never lost.
	wake chan struct{}
}

// New builds a loop bound to eng.
func New(eng engine.Engine) *Loop {
	return &Loop{
		eng:  eng,
		byID: make(map[int64]*timer),
		wake: make(chan struct{}, 1),
	}
}

// Post hands a task to the loop goroutine to run there. It is the one loop method
// safe to call from any goroutine, and it is how a blocking I/O goroutine returns
// control to JavaScript: the pool goroutine does the syscall, then posts a closure
// that touches the engine on the loop goroutine, honoring the single-threaded
// contract. A posted task keeps the loop from exiting only if a handle also holds
// a reference through AddRef; Post itself does not add one.
func (l *Loop) Post(task func()) {
	l.mu.Lock()
	l.pending = append(l.pending, task)
	l.mu.Unlock()
	// Signal Run without blocking; a buffered slot already pending is enough.
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// takePending atomically swaps out the queued tasks so the loop goroutine can run
// them without holding the lock while calling into the engine.
func (l *Loop) takePending() []func() {
	l.mu.Lock()
	tasks := l.pending
	l.pending = nil
	l.mu.Unlock()
	return tasks
}

// hasPending reports whether any task is waiting, used to decide whether an
// otherwise idle loop still has work.
func (l *Loop) hasPending() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.pending) > 0
}

// AddTimer registers a timer that fires after delay milliseconds. When repeat is
// true it reschedules itself at the same interval until cleared. It is called
// from the JavaScript timer bridge on the loop goroutine.
func (l *Loop) AddTimer(id int64, delayMs int, repeat bool) {
	if delayMs < 0 {
		delayMs = 0
	}
	d := time.Duration(delayMs) * time.Millisecond
	t := &timer{
		id:       id,
		due:      nowFunc().Add(d),
		interval: d,
		repeat:   repeat,
	}
	l.byID[id] = t
	heap.Push(&l.pq, t)
}

// ClearTimer cancels a pending timer. Unknown ids are ignored, matching
// clearTimeout.
func (l *Loop) ClearTimer(id int64) {
	t, ok := l.byID[id]
	if !ok {
		return
	}
	delete(l.byID, id)
	if t.index >= 0 {
		heap.Remove(&l.pq, t.index)
	}
}

// AddRef marks that an external handle (a socket, a file watcher) is keeping the
// loop alive even when no timers are pending.
func (l *Loop) AddRef() { l.refs++ }

// Unref drops a handle reference previously taken with AddRef.
func (l *Loop) Unref() {
	if l.refs > 0 {
		l.refs--
	}
}

// Stop asks Run to return after the current step without waiting for the queues
// to drain. It signals wake so a Run blocked on a posted task or a timer returns
// promptly. It is safe to call from another goroutine.
func (l *Loop) Stop() {
	l.stopped.Store(true)
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

// Run drives the loop until there is nothing left to do: no pending timers, no
// referenced handles, no posted tasks, and no microtasks in flight. It drains
// microtasks once up front so promises created by the entry module settle before
// the first timer.
func (l *Loop) Run() error {
	if _, err := l.eng.DrainMicrotasks(); err != nil {
		return err
	}

	for !l.stopped.Load() {
		if err := l.runPending(); err != nil {
			return err
		}
		if err := l.fireDueTimers(); err != nil {
			return err
		}
		if l.stopped.Load() {
			return nil
		}

		if l.pq.Len() == 0 {
			// No timers. Exit only when nothing keeps us alive: no referenced
			// handle and no task that a post slipped in after runPending.
			if l.refs == 0 && !l.hasPending() {
				return nil
			}
			// A handle holds the loop open; block until a task is posted.
			<-l.wake
			continue
		}

		// Wait until the next timer fires or a task is posted, whichever comes
		// first. A posted task drops us out early so its work runs promptly.
		wait := max(time.Until(l.pq.peek().due), 0)
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-l.wake:
			timer.Stop()
		}
	}
	return nil
}

// runPending runs every task posted from other goroutines, draining microtasks
// after each so a task's promise continuations settle before the next task.
func (l *Loop) runPending() error {
	for _, task := range l.takePending() {
		task()
		if _, err := l.eng.DrainMicrotasks(); err != nil {
			return err
		}
		if l.stopped.Load() {
			return nil
		}
	}
	return nil
}

// fireDueTimers fires every timer whose deadline has passed, in due order,
// draining microtasks after each. A repeating timer reschedules itself.
func (l *Loop) fireDueTimers() error {
	now := nowFunc()
	for l.pq.Len() > 0 && !l.pq.peek().due.After(now) {
		t := heap.Pop(&l.pq).(*timer)
		// A timer cleared since we last looked is gone from byID.
		if _, live := l.byID[t.id]; !live {
			continue
		}
		if t.repeat {
			t.due = now.Add(t.interval)
			heap.Push(&l.pq, t)
		} else {
			delete(l.byID, t.id)
		}
		if _, err := l.eng.Call("__bento_runTimer", t.id, t.repeat); err != nil {
			return err
		}
		if _, err := l.eng.DrainMicrotasks(); err != nil {
			return err
		}
		if l.stopped.Load() {
			return nil
		}
	}
	return nil
}

// timerQueue is a min-heap of timers ordered by due time, then id for a stable
// firing order among timers due at the same instant.
type timerQueue []*timer

func (q timerQueue) Len() int { return len(q) }
func (q timerQueue) Less(i, j int) bool {
	if q[i].due.Equal(q[j].due) {
		return q[i].id < q[j].id
	}
	return q[i].due.Before(q[j].due)
}
func (q timerQueue) Swap(i, j int) {
	q[i], q[j] = q[j], q[i]
	q[i].index = i
	q[j].index = j
}
func (q *timerQueue) Push(x any) {
	t := x.(*timer)
	t.index = len(*q)
	*q = append(*q, t)
}
func (q *timerQueue) Pop() any {
	old := *q
	n := len(old)
	t := old[n-1]
	old[n-1] = nil
	t.index = -1
	*q = old[:n-1]
	return t
}
func (q timerQueue) peek() *timer { return q[0] }
