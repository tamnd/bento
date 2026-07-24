package build

import "testing"

// TestEventTargetDispatchInvokesListener pins slice G1.4: a listener added for a type
// runs when an event of that type is dispatched, and the event it receives carries the
// dispatched type. The body adds a listener, dispatches an Event, and the listener logs
// the event's type; Node prints the type once.
func TestEventTargetDispatchInvokesListener(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const et = new EventTarget();\n"+
			"et.addEventListener('ping', (e) => { console.log('heard ' + e.type); });\n"+
			"et.dispatchEvent(new Event('ping'));\n")
	if want := "heard ping\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestEventTargetListenersFireInOrder pins that listeners for a type run in the order
// they were added. The body adds two inline listeners and dispatches once; Node prints
// the two lines in registration order. removeEventListener by a shared reference needs
// a function local to keep one boxed identity across the addEventListener and
// removeEventListener call sites, which the compiler does not give a boxed callback yet
// (each reference boxes a fresh value); that identity slice is tracked separately, and
// the runtime's removeEventListener is exercised directly in pkg/value.
func TestEventTargetListenersFireInOrder(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const et = new EventTarget();\n"+
			"et.addEventListener('x', () => { console.log('first'); });\n"+
			"et.addEventListener('x', () => { console.log('second'); });\n"+
			"et.dispatchEvent(new Event('x'));\n")
	if want := "first\nsecond\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestEventTargetOnceFiresOnlyOnce pins the once option: a listener registered with
// { once: true } runs on the first dispatch and is gone for the second. The body
// dispatches the same type twice; Node prints the listener's line once.
func TestEventTargetOnceFiresOnlyOnce(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const et = new EventTarget();\n"+
			"et.addEventListener('go', () => { console.log('fired'); }, { once: true });\n"+
			"et.dispatchEvent(new Event('go'));\n"+
			"et.dispatchEvent(new Event('go'));\n"+
			"console.log('done');\n")
	if want := "fired\ndone\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestEventPreventDefaultHonorsCancelable pins that preventDefault marks a cancelable
// event canceled and dispatchEvent returns false, while a non-cancelable event ignores
// preventDefault and dispatch returns true. The body dispatches one of each and logs
// the returned booleans and defaultPrevented; Node prints true then false for the
// non-cancelable event, then false then true for the cancelable one.
func TestEventPreventDefaultHonorsCancelable(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const et = new EventTarget();\n"+
			"et.addEventListener('a', (e) => { e.preventDefault(); });\n"+
			"const plain = new Event('a');\n"+
			"console.log(et.dispatchEvent(plain));\n"+
			"console.log(plain.defaultPrevented);\n"+
			"const cancelable = new Event('a', { cancelable: true });\n"+
			"console.log(et.dispatchEvent(cancelable));\n"+
			"console.log(cancelable.defaultPrevented);\n")
	if want := "true\nfalse\nfalse\ntrue\n"; got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
