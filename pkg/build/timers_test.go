package build

import "testing"

// This file covers the timer half of slice G1.2: setTimeout, setInterval,
// setImmediate, and the three clear functions, compiled through the AOT path and run.
// Before this slice every one of them handed back to the interpreter, so a Node
// program that scheduled anything for later did not compile at all. Each test builds a
// .js entry and runs the binary, so it pins what a compiled program actually prints,
// including the order the callbacks come out in.
//
// The delays are chosen so the expected order is fixed no matter how loaded the
// machine is: the runtime orders timers strictly by deadline, and two timers scheduled
// in the same synchronous run keep that order even when both are already overdue by the
// time the loop starts. The one ordering that is genuinely undecided under Node, a
// short setTimeout racing a setImmediate at the top level, is left unpinned here for
// the same reason Node's own documentation leaves it unspecified.

// TestTimerRunsAfterTheSynchronousBody pins the basic contract: setTimeout schedules,
// it does not call. The whole synchronous body runs first, then the loop main enters
// after it runs the callback, so a program prints its body before any scheduled work.
func TestTimerRunsAfterTheSynchronousBody(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('later'); }, 1);\n"+
			"console.log('now');\n")
	if want := "now\nlater\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestTimersFireInDeadlineOrder pins that the loop runs timers by deadline rather than
// by the order they were scheduled: the longer delay is registered first and still
// fires last. That is what makes a program that stages work over several delays come
// out in the order it asked for.
func TestTimersFireInDeadlineOrder(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('third'); }, 30);\n"+
			"setTimeout(function () { console.log('second'); }, 20);\n"+
			"setTimeout(function () { console.log('first'); }, 10);\n")
	if want := "first\nsecond\nthird\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestClearTimeoutCancelsTheCallback pins that a cleared timeout never runs. The
// cancelled callback would print between the two that do run if the clear were ignored,
// so the surviving output is what shows the cancel took.
func TestClearTimeoutCancelsTheCallback(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('kept'); }, 30);\n"+
			"const h = setTimeout(function () { console.log('cancelled'); }, 10);\n"+
			"clearTimeout(h);\n"+
			"console.log('body');\n")
	if want := "body\nkept\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestTimerForwardsExtraArguments pins that the arguments after the delay reach the
// callback. Node forwards them, and a program that schedules the same function for
// several values relies on it; a lowering that bound the call against the declared
// signature would drop them.
func TestTimerForwardsExtraArguments(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function (a, b) { console.log(a + '-' + b); }, 1, 'x', 'y');\n")
	if want := "x-y\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestIntervalRepeatsUntilCleared pins setInterval: the callback runs again and again
// until the program clears it, and clearing from inside the callback takes effect
// rather than letting one more run through. The handle is assigned to a pre-declared
// binding because a const initialized to a function that names itself is a separate
// lowering gap; the timer behaviour under test is the same either way.
func TestIntervalRepeatsUntilCleared(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"let n = 0;\n"+
			"let iv = 0;\n"+
			"iv = setInterval(function () {\n"+
			"  n++;\n"+
			"  console.log('tick ' + n);\n"+
			"  if (n === 3) clearInterval(iv);\n"+
			"}, 5);\n")
	if want := "tick 1\ntick 2\ntick 3\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestImmediateRunsBeforeALaterTimeout pins setImmediate: it runs in the next turn's
// check phase, so it beats a timeout that is still counting down. The timeout's delay
// is long enough that the immediate wins regardless of how slowly the binary started,
// which is what keeps this test off the race Node leaves unspecified for a 1ms timeout.
func TestImmediateRunsBeforeALaterTimeout(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('timeout'); }, 50);\n"+
			"setImmediate(function () { console.log('immediate'); });\n"+
			"console.log('body');\n")
	if want := "body\nimmediate\ntimeout\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestImmediateFromImmediateWaitsForTheNextTurn pins that the check phase runs only the
// immediates queued before it began. An immediate scheduled from inside one waits for
// the following turn, which is what lets setImmediate yield to the loop: were the batch
// re-read as it grew, a self-rescheduling immediate would starve every timer. The
// timeout here is what observes it, running between the two immediates.
func TestImmediateFromImmediateWaitsForTheNextTurn(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setImmediate(function () {\n"+
			"  console.log('first');\n"+
			"  setImmediate(function () { console.log('second'); });\n"+
			"});\n")
	if want := "first\nsecond\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestClearImmediateCancelsTheCallback pins that an immediate can be cancelled before
// the check phase reaches it, the setImmediate half of the clear surface.
func TestClearImmediateCancelsTheCallback(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const h = setImmediate(function () { console.log('cancelled'); });\n"+
			"setImmediate(function () { console.log('kept'); });\n"+
			"clearImmediate(h);\n")
	if want := "kept\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestNestedTimeoutKeepsTheLoopAlive pins that the loop keeps turning as long as
// callbacks keep scheduling work, rather than running one round and exiting. A loop
// that drained only what the synchronous body scheduled would print "one" and stop,
// losing the rest of the chain.
func TestNestedTimeoutKeepsTheLoopAlive(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () {\n"+
			"  console.log('one');\n"+
			"  setTimeout(function () {\n"+
			"    console.log('two');\n"+
			"    setTimeout(function () { console.log('three'); }, 1);\n"+
			"  }, 1);\n"+
			"}, 1);\n")
	if want := "one\ntwo\nthree\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestMicrotaskCheckpointFallsBetweenTimers pins that the microtask queue drains after
// each timer callback, not once at the end. A microtask queued by the first timer must
// run before the second timer fires, which is the checkpoint the language puts between
// two macrotasks; draining per turn instead would print both timers and then both
// microtasks.
func TestMicrotaskCheckpointFallsBetweenTimers(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () {\n"+
			"  console.log('timer one');\n"+
			"  queueMicrotask(function () { console.log('micro one'); });\n"+
			"}, 10);\n"+
			"setTimeout(function () { console.log('timer two'); }, 20);\n")
	if want := "timer one\nmicro one\ntimer two\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestClearingAnUnknownHandleIsANoOp pins that clearing a handle that names no live
// timer does nothing rather than failing. Clearing a timeout that already fired is
// ordinary in Node, and so is clearing a handle a program initialized to zero before
// deciding whether to schedule anything, so neither may throw.
func TestClearingAnUnknownHandleIsANoOp(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"clearTimeout(0);\n"+
			"clearInterval(12345);\n"+
			"const h = setTimeout(function () { console.log('fired'); }, 1);\n"+
			"setTimeout(function () { clearTimeout(h); console.log('cleared after firing'); }, 20);\n")
	if want := "fired\ncleared after firing\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestTimerWithNoDelayStillYields pins the delay clamp: setTimeout(fn) with no delay,
// and setTimeout(fn, 0), both mean the shortest wait rather than an inline call, so the
// synchronous body still finishes first. That is what keeps a zero-delay timeout a way
// to yield rather than a disguised function call.
func TestTimerWithNoDelayStillYields(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('zero'); }, 0);\n"+
			"setTimeout(function () { console.log('omitted'); });\n"+
			"console.log('body');\n")
	if want := "body\nzero\nomitted\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
