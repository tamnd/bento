package build

import "testing"

// Node's setTimeout returns a Timeout object carrying ref, unref, hasRef, and refresh.
// A compiled bento program holds the timer's id instead, which is what the standard
// library declaration says the call returns, so those four are operations on the id.
// These build and run programs that use each one, because what unref is worth is
// entirely in whether the process waits for the timer, which only a real run shows.

// TestUnrefLetsTheProcessLeave pins the whole point of unref: the unrefed timeout no
// longer holds the process open, so the program leaves after the refed one rather than
// waiting a second for a callback it does not care about. This is the shape Node's own
// suite writes, setTimeout(common.mustNotCall(), 1000).unref().
func TestUnrefLetsTheProcessLeave(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('should not run'); }, 100000).unref();\n"+
			"setTimeout(function () { console.log('refed'); }, 5);\n")
	if want := "refed\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestUnrefDoesNotCancel pins the difference between unref and clearTimeout: the
// callback is still scheduled and still runs, as long as something else keeps the loop
// turning. A program that read unref as a cancel would print nothing here.
func TestUnrefDoesNotCancel(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setTimeout(function () { console.log('unrefed ran'); }, 5).unref();\n"+
			"setTimeout(function () { console.log('refed ran'); }, 40);\n")
	if want := "unrefed ran\nrefed ran\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRefPutsTheTimerBack pins that ref undoes unref, so a program that unrefs a timer
// and changes its mind waits for it again.
func TestRefPutsTheTimerBack(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const t = setTimeout(function () { console.log('ran'); }, 20);\n"+
			"t.unref();\n"+
			"t.ref();\n")
	if want := "ran\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestHasRefReadsTheRefFlagOnly pins hasRef against Node, including the part that reads
// wrong at first glance: clearing a timeout does not unref it, so a cleared timer still
// answers true. hasRef reports whether the timer is refed, not whether it is live.
func TestHasRefReadsTheRefFlagOnly(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const t = setTimeout(function () {}, 50);\n"+
			"console.log('fresh', t.hasRef());\n"+
			"t.unref();\n"+
			"console.log('unrefed', t.hasRef());\n"+
			"t.ref();\n"+
			"console.log('refed', t.hasRef());\n"+
			"clearTimeout(t);\n"+
			"console.log('cleared', t.hasRef());\n")
	if want := "fresh true\nunrefed false\nrefed true\ncleared true\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestUnrefSurvivesTheTimerBeingForgotten pins the bookkeeping behind the previous
// test: the runtime drops a finished timer from its registry, and an id it has
// forgotten reads as refed. Forgetting an unrefed one would make hasRef answer true
// after an unref, so unrefed timers are kept.
func TestUnrefSurvivesTheTimerBeingForgotten(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const t = setTimeout(function () {}, 5);\n"+
			"t.unref();\n"+
			"setTimeout(function () { console.log('after', t.hasRef()); }, 20);\n")
	if want := "after false\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestRefreshMovesTheDeadline pins refresh: the timeout is re-armed for its original
// delay counted from the refresh, so the timer that was scheduled first fires last.
func TestRefreshMovesTheDeadline(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const a = setTimeout(function () { console.log('a'); }, 20);\n"+
			"setTimeout(function () { console.log('b'); }, 25);\n"+
			"setTimeout(function () { a.refresh(); }, 10);\n")
	if want := "b\na\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestImmediateUnrefLetsTheProcessLeave pins the same rule for setImmediate, which
// Node's suite unrefs the same way. Nothing else is scheduled, so the process leaves
// without running the callback at all.
func TestImmediateUnrefLetsTheProcessLeave(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setImmediate(function () { console.log('should not run'); }).unref();\n"+
			"console.log('body');\n")
	if want := "body\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestIntervalUnrefLetsTheProcessLeave pins the case that would otherwise hang: an
// interval nobody clears keeps the loop alive forever, and unref is how a program says
// it should not.
func TestIntervalUnrefLetsTheProcessLeave(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"setInterval(function () { console.log('tick'); }, 100000).unref();\n"+
			"console.log('body');\n")
	if want := "body\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
