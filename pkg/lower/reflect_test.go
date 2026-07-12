package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestReflectConstructHandsBack pins the boundary: Reflect.construct reaches into
// [[Construct]] and the newTarget slot, neither of which bento's class path models,
// so it hands back with a named reason rather than emitting an unfaithful call.
func TestReflectConstructHandsBack(t *testing.T) {
	const src = "export function make(C: any): any { return Reflect.construct(C, [1, 2]); }\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Reflect.construct") {
		t.Errorf("hand-back reason = %q, want it to mention Reflect.construct", nyl.Reason)
	}
}

// TestReflectGetReceiverHandsBack pins that the three-argument receiver overload of
// Reflect.get, which reads a property with a receiver other than the target, is its
// own later slice and hands back rather than dropping the receiver silently.
func TestReflectGetReceiverHandsBack(t *testing.T) {
	const src = "export function read(o: any, r: any): any { return Reflect.get(o, \"k\", r); }\n"
	prog := compile(t, src)
	rr := NewRenderer(prog)
	_, err := rr.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Reflect.get") {
		t.Errorf("hand-back reason = %q, want it to mention Reflect.get", nyl.Reason)
	}
}

// TestReflectSetReceiverHandsBack pins that the four-argument receiver overload of
// Reflect.set, which writes with a receiver other than the target, is its own later
// slice and hands back rather than dropping the receiver silently.
func TestReflectSetReceiverHandsBack(t *testing.T) {
	const src = "export function write(o: any, r: any): boolean { return Reflect.set(o, \"k\", 1, r); }\n"
	prog := compile(t, src)
	rr := NewRenderer(prog)
	_, err := rr.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "Reflect.set") {
		t.Errorf("hand-back reason = %q, want it to mention Reflect.set", nyl.Reason)
	}
}
