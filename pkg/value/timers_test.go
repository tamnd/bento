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
