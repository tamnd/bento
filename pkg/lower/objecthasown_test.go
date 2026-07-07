package lower

import (
	"strings"
	"testing"
)

// TestObjectHasOwnPresentFoldsTrue pins that Object.hasOwn with a string-literal
// key that names a required field folds to the constant true, since the shape
// guarantees the key is present.
func TestObjectHasOwnPresentFoldsTrue(t *testing.T) {
	src := `
const o = { a: 1, b: "x" };
console.log(o.a, Object.hasOwn(o, "a"));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.BoolToString(true)") {
		t.Fatalf("expected hasOwn of a present key to fold to true, got:\n%s", out)
	}
}

// TestObjectHasOwnAbsentFoldsFalse pins that Object.hasOwn with a key the shape
// does not have folds to the constant false.
func TestObjectHasOwnAbsentFoldsFalse(t *testing.T) {
	src := `
const o = { a: 1, b: "x" };
console.log(o.a, Object.hasOwn(o, "z"));
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.BoolToString(false)") {
		t.Fatalf("expected hasOwn of an absent key to fold to false, got:\n%s", out)
	}
}

// TestObjectHasOwnOptionalHandsBack pins that Object.hasOwn on an optional
// property hands back, since the shape cannot settle whether the field is
// present at runtime.
func TestObjectHasOwnOptionalHandsBack(t *testing.T) {
	src := `
type Shape = { a: number; b?: string };
const o: Shape = { a: 1 };
console.log(o.a, Object.hasOwn(o, "b"));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "optional property") {
		t.Fatalf("expected an optional-property handback, got: %q", reason)
	}
}

// TestObjectHasOwnDynamicKeyHandsBack pins that Object.hasOwn with a key that is
// not a string literal hands back, since the field is not statically known.
func TestObjectHasOwnDynamicKeyHandsBack(t *testing.T) {
	src := `
const o = { a: 1 };
const k = "a";
console.log(o.a, Object.hasOwn(o, k));
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "not a string literal") {
		t.Fatalf("expected a dynamic-key handback, got: %q", reason)
	}
}

// TestObjectHasOwnOrphanedReceiverBuilds pins that a receiver whose only read is the
// folded Object.hasOwn call is blanked, so the emit builds rather than tripping the
// declared-and-not-used check on the dropped argument.
func TestObjectHasOwnOrphanedReceiverBuilds(t *testing.T) {
	src := `
var o = { x: 1, y: 2 };
var b = Object.hasOwn(o, "x");
console.log(b);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "_ = o") {
		t.Fatalf("expected a _ = o blank for the orphaned hasOwn receiver, got:\n%s", out)
	}
}

// TestObjectHasOwnRuns builds and runs the emitted Go and checks the fold
// against the Node oracle: a present key is true and an absent one is false.
func TestObjectHasOwnRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const o = { a: 1, b: "x", c: true };
console.log(o.a);
console.log(Object.hasOwn(o, "a"));
console.log(Object.hasOwn(o, "b"));
console.log(Object.hasOwn(o, "c"));
console.log(Object.hasOwn(o, "z"));
`
	got := runProgramGo(t, src)
	want := "1\ntrue\ntrue\ntrue\nfalse\n"
	if got != want {
		t.Fatalf("Object.hasOwn run mismatch:\n got %q\nwant %q", got, want)
	}
}
