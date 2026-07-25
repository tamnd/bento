package build

import "testing"

// TestQueueMicrotaskRunsAfterSyncBody pins slice G1.2: queueMicrotask(fn) runs fn at
// the microtask checkpoint, after the current synchronous run finishes. The body logs
// around a queueMicrotask call, so the scheduled callback must print last even though
// it was scheduled first. Node prints the two synchronous lines, then the microtask
// line.
func TestQueueMicrotaskRunsAfterSyncBody(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"queueMicrotask(() => { console.log('micro'); });\n"+
			"console.log('sync-a');\n"+
			"console.log('sync-b');\n")
	if want := "sync-a\nsync-b\nmicro\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestQueueMicrotaskFIFOOrder pins that two queued microtasks run in the order they
// were scheduled, the first-in-first-out order of the microtask queue, and that a
// microtask scheduled from inside a microtask runs after both, since it enqueues onto
// the same queue while it drains. Node prints one, two, then three.
func TestQueueMicrotaskFIFOOrder(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"queueMicrotask(() => { console.log('one'); queueMicrotask(() => { console.log('three'); }); });\n"+
			"queueMicrotask(() => { console.log('two'); });\n")
	if want := "one\ntwo\nthree\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestQueueMicrotaskBeforeExitCallback pins the queue ordering the async tests lean
// on: a microtask drains before the process 'exit' callbacks run, since exit fires
// only once the microtask checkpoint is clear. The body registers an exit listener
// and schedules a microtask; Node runs the microtask first, then the exit listener.
func TestQueueMicrotaskBeforeExitCallback(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"process.on('exit', () => { console.log('exit'); });\n"+
			"queueMicrotask(() => { console.log('micro'); });\n"+
			"console.log('body');\n")
	if want := "body\nmicro\nexit\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
