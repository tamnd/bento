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
	refs    int  // outstanding I/O handles keeping the loop alive
	stopped bool // set when a hard stop is requested
}

// New builds a loop bound to eng.
func New(eng engine.Engine) *Loop {
	return &Loop{
		eng:  eng,
		byID: make(map[int64]*timer),
	}
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
// to drain.
func (l *Loop) Stop() { l.stopped = true }

// Run drives the loop until there is nothing left to do: no pending timers, no
// referenced handles, and no microtasks in flight. It drains microtasks once up
// front so promises created by the entry module settle before the first timer.
func (l *Loop) Run() error {
	if _, err := l.eng.DrainMicrotasks(); err != nil {
		return err
	}

	for !l.stopped {
		if l.pq.Len() == 0 {
			if l.refs > 0 {
				// Nothing scheduled but a handle keeps us alive. A real I/O
				// integration would block on the netpoller here; for now yield
				// and re-check so the loop does not spin hot.
				time.Sleep(time.Millisecond)
				continue
			}
			return nil
		}

		next := l.pq.peek()
		if wait := time.Until(next.due); wait > 0 {
			time.Sleep(wait)
		}
		if l.stopped {
			return nil
		}

		t := heap.Pop(&l.pq).(*timer)
		// A timer cleared while we were sleeping is gone from byID.
		if _, live := l.byID[t.id]; !live {
			continue
		}

		if t.repeat {
			t.due = nowFunc().Add(t.interval)
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
