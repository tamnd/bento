package lower

import (
	"strings"
	"testing"
)

// TestObjectSealReceiverBoxes proves a binding handed to Object.seal boxes to a
// dynamic bag from its literal, so the seal lowers to the runtime Seal on the box
// rather than handing back on a fixed-shape struct that has no integrity state.
func TestObjectSealReceiverBoxes(t *testing.T) {
	const src = "const o = { x: 1 };\nObject.seal(o);\n"
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "value.NewObject()") {
		t.Fatalf("Object.seal receiver did not box to a bag:\n%s", source)
	}
	if !strings.Contains(source, ".Seal()") {
		t.Fatalf("Object.seal did not lower to the runtime Seal on the box:\n%s", source)
	}
}

// TestObjectDefinePropertyReceiverNotRouted proves a binding handed to
// Object.defineProperty is not boxed by this routing. defineProperty stays out of the
// routed set: boxing an array-like the test then drives through Array.prototype.<m>.call
// turns a handback into a fail, because the array generic on a bag does not honor an
// accessor descriptor mutating length mid-iteration. So the binding keeps its fixed
// shape and the define hands the unit back rather than run wrong.
func TestObjectDefinePropertyReceiverNotRouted(t *testing.T) {
	const src = "const o = { x: 1 };\nObject.defineProperty(o, \"y\", { value: 2 });\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "Object.defineProperty on a fixed-shape receiver") {
		t.Fatalf("hand-back reason %q does not name the defineProperty fixed-shape guard", reason)
	}
}

// TestObjectSetPrototypeOfReceiverRuns proves the boxed binding runs against the Node
// oracle: setPrototypeOf writes the slot and getPrototypeOf reads it back, so a plain
// object routed through the prototype statics behaves the way the runtime chain does.
func TestObjectSetPrototypeOfReceiverRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const proto = { greet: 1 };\nconst o = { x: 1 };\nObject.setPrototypeOf(o, proto);\nconsole.log(Object.getPrototypeOf(o) === proto);\n"
	if got, want := runProgramGoTolerant(t, src), "true\n"; got != want {
		t.Fatalf("setPrototypeOf/getPrototypeOf run = %q, want %q", got, want)
	}
}

// TestObjectFreezeReceiverRuns proves a frozen boxed binding rejects a later write and
// keeps its value, matching the runtime freeze semantics the bag honors.
func TestObjectFreezeReceiverRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o = { x: 1 };\nObject.freeze(o);\nconsole.log(Object.isFrozen(o));\n"
	if got, want := runProgramGoTolerant(t, src), "true\n"; got != want {
		t.Fatalf("freeze/isFrozen run = %q, want %q", got, want)
	}
}
