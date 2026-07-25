package build

import "testing"

// TestAbortControllerAbortSetsAborted pins slice G1.5: a fresh controller's signal reads
// aborted false, and after the controller aborts it reads true. The body logs the flag
// before and after; Node prints false then true.
func TestAbortControllerAbortSetsAborted(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const ac = new AbortController();\n"+
			"console.log(ac.signal.aborted);\n"+
			"ac.abort();\n"+
			"console.log(ac.signal.aborted);\n")
	if want := "false\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAbortSignalFiresAbortListener pins that a listener added on the signal for the
// abort event runs when the controller aborts, the EventTarget path a cancelable
// operation waits on. The listener logs a line; Node prints it once.
func TestAbortSignalFiresAbortListener(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const ac = new AbortController();\n"+
			"ac.signal.addEventListener('abort', () => { console.log('aborted'); });\n"+
			"ac.abort();\n"+
			"console.log('done');\n")
	if want := "aborted\ndone\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAbortControllerAbortReason pins that the reason passed to abort flows to
// signal.reason: aborting with a string reads that string back. The body aborts with a
// string and logs signal.reason; Node prints the string.
func TestAbortControllerAbortReason(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const ac = new AbortController();\n"+
			"ac.abort('stop');\n"+
			"console.log(ac.signal.reason);\n")
	if want := "stop\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAbortDefaultReasonName pins that aborting with no reason gives an AbortError: the
// default reason's name is AbortError, so a consumer can tell a cancellation apart from
// another failure. The body aborts with no argument and logs signal.reason.name.
func TestAbortDefaultReasonName(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const ac = new AbortController();\n"+
			"ac.abort();\n"+
			"console.log(ac.signal.reason.name);\n")
	if want := "AbortError\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
