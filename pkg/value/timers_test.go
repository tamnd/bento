package value

import (
	"math"
	"testing"
	"time"
)

// TestClampTimerDelayAppliesNodesFloorAndCeiling pins the delay clamp, the one piece of
// the scheduler that is a pure function of its argument and so can be checked without
// running the loop. Node forces a delay below 1ms, not a number, or above the 32-bit
// millisecond ceiling to 1ms, which is why setTimeout(fn, 0) yields to the loop instead
// of running inline and why a nonsense delay schedules soon rather than never. An
// ordinary delay passes through unchanged.
func TestClampTimerDelayAppliesNodesFloorAndCeiling(t *testing.T) {
	ms := float64(time.Millisecond)
	cases := []struct {
		name  string
		delay Value
		want  time.Duration
	}{
		{"ordinary delay passes through", Number(25), time.Duration(25 * ms)},
		{"one millisecond is the floor itself", Number(1), time.Duration(ms)},
		{"zero clamps up to the floor", Number(0), time.Duration(ms)},
		{"a negative delay clamps up to the floor", Number(-5), time.Duration(ms)},
		{"a fractional delay below one clamps up", Number(0.4), time.Duration(ms)},
		{"an omitted delay reads as the floor", Undefined, time.Duration(ms)},
		{"NaN clamps to the floor", Number(math.NaN()), time.Duration(ms)},
		{"infinity clamps to the floor", Number(math.Inf(1)), time.Duration(ms)},
		{"past the 32-bit ceiling clamps to the floor", Number(maxTimerDelayMS + 1), time.Duration(ms)},
		{"the ceiling itself passes through", Number(maxTimerDelayMS), time.Duration(maxTimerDelayMS * ms)},
		{"a numeric string coerces the way Node does", StringValue(FromGoString("20")), time.Duration(20 * ms)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := clampTimerDelay(c.delay); got != c.want {
				t.Fatalf("want %v, got %v", c.want, got)
			}
		})
	}
}

// TestScheduleTimerKeepsDeadlineOrder pins the queue invariant the loop reads: pending
// timers stay sorted by deadline, with the scheduling order breaking a tie, so the timer
// that fires next is always at the front. Inserting out of order is the case that
// matters, since a program commonly schedules a long delay before a short one.
func TestScheduleTimerKeepsDeadlineOrder(t *testing.T) {
	saved := pendingTimers
	defer func() { pendingTimers = saved }()
	pendingTimers = nil

	base := time.Now()
	late := &timerHandle{id: 1, deadline: base.Add(30 * time.Millisecond), seq: 1}
	early := &timerHandle{id: 2, deadline: base.Add(10 * time.Millisecond), seq: 2}
	tiedFirst := &timerHandle{id: 3, deadline: base.Add(20 * time.Millisecond), seq: 3}
	tiedSecond := &timerHandle{id: 4, deadline: base.Add(20 * time.Millisecond), seq: 4}
	for _, h := range []*timerHandle{late, early, tiedSecond, tiedFirst} {
		scheduleTimer(h)
	}

	want := []float64{2, 3, 4, 1}
	if len(pendingTimers) != len(want) {
		t.Fatalf("want %d pending timers, got %d", len(want), len(pendingTimers))
	}
	for i, id := range want {
		if pendingTimers[i].id != id {
			t.Fatalf("at position %d want timer %v, got %v", i, id, pendingTimers[i].id)
		}
	}
}

// TestRemoveTimerTakesOutOnlyTheNamedTimer pins the cancel half of the scheduler: two
// timers armed for the same instant are told apart by identity, so clearing one leaves
// the other pending. A removal that matched on the deadline would cancel both.
func TestRemoveTimerTakesOutOnlyTheNamedTimer(t *testing.T) {
	at := time.Now().Add(time.Millisecond)
	keep := &timerHandle{id: 1, deadline: at, seq: 1}
	drop := &timerHandle{id: 2, deadline: at, seq: 2}

	got := removeTimer([]*timerHandle{keep, drop}, drop)
	if len(got) != 1 || got[0] != keep {
		t.Fatalf("want only the kept timer to remain, got %d timers", len(got))
	}
	if same := removeTimer(got, drop); len(same) != 1 || same[0] != keep {
		t.Fatalf("removing an absent timer should leave the queue alone, got %d timers", len(same))
	}
}

// freshTimerState swaps the package's scheduler state for an empty one and puts the
// real one back when the test ends, so a test can schedule timers without the ids it
// hands out or the queue it leaves behind reaching the next one. The state is one set
// of package-level variables because a program runs one event loop, so a test that
// wants its own has to borrow them.
func freshTimerState(t *testing.T) {
	t.Helper()
	timers, immediates, byID := pendingTimers, pendingImmediates, timersByID
	t.Cleanup(func() {
		pendingTimers, pendingImmediates, timersByID = timers, immediates, byID
	})
	pendingTimers, pendingImmediates, timersByID = nil, nil, map[float64]*timerHandle{}
}

// TestTimerRefAndUnrefFlipTheRefFlag pins the pair a program calls to say whether a
// timer should hold the process open. Neither cancels anything, so the timer stays in
// the queue either way, and both hand the id back so the call can be chained the way
// Node's setTimeout(fn, d).unref() is written.
func TestTimerRefAndUnrefFlipTheRefFlag(t *testing.T) {
	freshTimerState(t)

	id := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(50))
	handle := Number(id)
	timer := timersByID[id]
	if timer.unrefed {
		t.Fatal("a fresh timer should be refed")
	}
	if got := TimerUnref(handle); got != id {
		t.Fatalf("unref should hand the id back, want %v, got %v", id, got)
	}
	if !timer.unrefed {
		t.Fatal("unref should clear the ref flag")
	}
	if len(pendingTimers) != 1 {
		t.Fatalf("unref should not cancel the timer, want 1 pending, got %d", len(pendingTimers))
	}
	if got := TimerRef(handle); got != id || timer.unrefed {
		t.Fatalf("ref should restore the flag and hand the id back, got %v unrefed=%v", got, timer.unrefed)
	}
}

// TestTimerRefOnAnIdNamingNothingIsQuiet pins that unrefing a timer that already fired
// is as ordinary as clearing one, so an id this runtime does not know is ignored rather
// than raising. The id still comes back, since the caller may be chaining.
func TestTimerRefOnAnIdNamingNothingIsQuiet(t *testing.T) {
	freshTimerState(t)

	if got := TimerUnref(Number(4242)); got != 4242 {
		t.Fatalf("want the id back, got %v", got)
	}
	if got := TimerRef(Number(4242)); got != 4242 {
		t.Fatalf("want the id back, got %v", got)
	}
}

// TestTimerHasRefReadsOnlyTheFlag pins hasRef against Node, including the part that
// reads wrong at first glance: clearing a timeout does not unref it, so a cleared timer
// still answers true, and an id the runtime never handed out answers true as well.
func TestTimerHasRefReadsOnlyTheFlag(t *testing.T) {
	freshTimerState(t)

	id := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(50))
	handle := Number(id)
	if !TimerHasRef(handle) {
		t.Fatal("a fresh timer should read as refed")
	}
	TimerUnref(handle)
	if TimerHasRef(handle) {
		t.Fatal("an unrefed timer should read as unrefed")
	}
	TimerRef(handle)
	ClearTimer(handle)
	if !TimerHasRef(handle) {
		t.Fatal("clearing a timer should not unref it")
	}
	if !TimerHasRef(Number(4242)) {
		t.Fatal("an id naming no timer should read as refed")
	}
}

// TestForgetTimerKeepsUnrefedOnes pins the bookkeeping hasRef rests on. A finished
// timer is dropped from the id registry so the map does not grow without bound, and a
// forgotten id reads as refed, which is right only for a timer nobody unrefed. So the
// unrefed ones are kept, and their flag survives the timer being over.
func TestForgetTimerKeepsUnrefedOnes(t *testing.T) {
	freshTimerState(t)

	refed := &timerHandle{id: 1}
	unrefed := &timerHandle{id: 2, unrefed: true}
	timersByID[refed.id] = refed
	timersByID[unrefed.id] = unrefed

	forgetTimer(refed)
	forgetTimer(unrefed)

	if _, ok := timersByID[refed.id]; ok {
		t.Fatal("a finished refed timer should be forgotten")
	}
	if _, ok := timersByID[unrefed.id]; !ok {
		t.Fatal("an unrefed timer should be kept so hasRef stays truthful")
	}
	if TimerHasRef(Number(unrefed.id)) {
		t.Fatal("the kept timer should still read as unrefed")
	}
}

// TestHasPendingWorkCountsOnlyRefedTimers pins what keeps a process alive. Unrefed and
// cleared timers do not count, in either queue, which is what lets a program whose only
// remaining work is an unrefed timeout leave instead of waiting for it.
func TestHasPendingWorkCountsOnlyRefedTimers(t *testing.T) {
	freshTimerState(t)

	if hasPendingWork() {
		t.Fatal("an empty scheduler should have no pending work")
	}
	timeout := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(50))
	immediate := SetImmediate(NewFunc(func([]Value) Value { return Undefined }))
	if !hasPendingWork() {
		t.Fatal("a refed timeout and immediate are pending work")
	}
	TimerUnref(Number(timeout))
	if !hasPendingWork() {
		t.Fatal("the immediate is still refed")
	}
	TimerUnref(Number(immediate))
	if hasPendingWork() {
		t.Fatal("nothing refed is left, so the process should not wait")
	}
}

// TestTimerRefreshRearmsTheTimeout pins refresh: the timeout is re-armed for the delay
// it was scheduled with, counted from the refresh rather than from when it was created,
// so a timeout refreshed on activity only fires after a real gap. Re-arming moves it in
// the queue, which is how it comes to fire after a timer that was scheduled later.
func TestTimerRefreshRearmsTheTimeout(t *testing.T) {
	freshTimerState(t)

	early := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(20))
	late := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(25))
	if pendingTimers[0].id != early {
		t.Fatalf("want the shorter delay first, got %v", pendingTimers[0].id)
	}

	// Rewind both deadlines to stand in for time passing, so the refresh below lands
	// partway through the wait the way a real one does. Shifting both by the same
	// amount leaves the queue's order alone, so this is only about the clock.
	for _, h := range pendingTimers {
		h.deadline = h.deadline.Add(-15 * time.Millisecond)
	}

	before := timersByID[early].deadline
	if got := TimerRefresh(Number(early)); got != early {
		t.Fatalf("refresh should hand the id back, want %v, got %v", early, got)
	}
	if !timersByID[early].deadline.After(before) {
		t.Fatal("refresh should move the deadline later")
	}
	if len(pendingTimers) != 2 {
		t.Fatalf("refresh should re-arm rather than duplicate, got %d pending", len(pendingTimers))
	}
	if pendingTimers[0].id != late {
		t.Fatalf("the refreshed timer should now be last, got %v first", pendingTimers[0].id)
	}
}

// TestTimerRefreshIgnoresWhatHasNoDeadline pins the cases refresh does nothing for: an
// id the runtime never handed out, a timer that was cleared, and an immediate, which
// has no deadline to move and which Node does not give the method at all.
func TestTimerRefreshIgnoresWhatHasNoDeadline(t *testing.T) {
	freshTimerState(t)

	if got := TimerRefresh(Number(4242)); got != 4242 {
		t.Fatalf("want the id back, got %v", got)
	}

	cleared := SetTimeout(NewFunc(func([]Value) Value { return Undefined }), Number(20))
	ClearTimer(Number(cleared))
	TimerRefresh(Number(cleared))
	if len(pendingTimers) != 0 {
		t.Fatalf("refreshing a cleared timer should not re-arm it, got %d pending", len(pendingTimers))
	}

	immediate := SetImmediate(NewFunc(func([]Value) Value { return Undefined }))
	TimerRefresh(Number(immediate))
	if len(pendingTimers) != 0 {
		t.Fatalf("refreshing an immediate should not give it a deadline, got %d pending", len(pendingTimers))
	}
	if len(pendingImmediates) != 1 {
		t.Fatalf("the immediate should still be queued, got %d", len(pendingImmediates))
	}
}
