package lower

import (
	"strings"
	"testing"
)

// TestObjectKeysEmitsNameArray pins that Object.keys on a fixed-shape object
// lowers to a compile-time value.NewArray[value.BStr] of the property-name
// literals in declaration order, not a runtime property walk.
func TestObjectKeysEmitsNameArray(t *testing.T) {
	src := `
const o = { name: "hi", age: 3 };
const ks = Object.keys(o);
console.log(ks.length);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.NewArray[value.BStr]") {
		t.Fatalf("expected a NewArray[value.BStr] of the keys, got:\n%s", out)
	}
	if !strings.Contains(out, `value.FromGoString("name")`) || !strings.Contains(out, `value.FromGoString("age")`) {
		t.Fatalf("expected the property-name literals, got:\n%s", out)
	}
	// The order of the literals must follow declaration order.
	if strings.Index(out, `FromGoString("name")`) > strings.Index(out, `FromGoString("age")`) {
		t.Fatalf("expected name before age in the key array, got:\n%s", out)
	}
}

// TestObjectKeysDynamicHandsBack pins that Object.keys of an expression that is
// not a plain identifier hands back, since only the argument's type is read and a
// non-identifier could carry a side effect this slice would drop.
func TestObjectKeysDynamicHandsBack(t *testing.T) {
	src := `
const ks = Object.keys({ a: 1, b: 2 });
console.log(ks.length);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "not a plain identifier") {
		t.Fatalf("expected a non-identifier handback, got: %q", reason)
	}
}

// TestObjectKeysOrphanedReceiverBlanks pins the fix for the fold that orphaned its
// argument: when the receiver's only read is the Object.keys call, the fold drops it
// from the emitted Go, so the binding is blanked with _ = o to survive the Go
// compiler's declared-and-not-used check the way an unused var does elsewhere.
func TestObjectKeysOrphanedReceiverBlanks(t *testing.T) {
	src := `
var o = { x: 1, y: 2 };
var a = Object.keys(o);
console.log(a.length);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "_ = o") {
		t.Fatalf("expected a _ = o blank for the orphaned receiver, got:\n%s", out)
	}
}

// TestObjectKeysUsedReceiverNoBlank pins the other half: a receiver read somewhere
// past the fold stays used in the emitted Go, so no blank is added and the binding is
// not double-counted as unused.
func TestObjectKeysUsedReceiverNoBlank(t *testing.T) {
	src := `
var o = { x: 1, y: 2 };
var a = Object.keys(o);
console.log(o.x);
console.log(a.length);
`
	out := renderProgram(t, src)
	if strings.Contains(out, "_ = o") {
		t.Fatalf("did not expect a blank for a receiver read elsewhere, got:\n%s", out)
	}
}

// TestObjectKeysOrphanedReceiverRuns builds and runs the orphaned-receiver case, the
// one that failed the Go build before the blank, and checks the key list.
func TestObjectKeysOrphanedReceiverRuns(t *testing.T) {
	skipIfShort(t)
	src := `
var o = { x: 1, y: 2 };
var a = Object.keys(o);
console.log(a.length);
console.log(a[0]);
console.log(a[1]);
`
	got := runProgramGo(t, src)
	want := "2\nx\ny\n"
	if got != want {
		t.Fatalf("orphaned Object.keys run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestObjectKeysRuns builds and runs the emitted Go and checks the key list
// against the Node oracle: the names in declaration order and the count.
func TestObjectKeysRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const o = { name: "hi", age: 3, active: true };
const ks = Object.keys(o);
console.log(o.name);
console.log(ks.length);
console.log(ks[0]);
console.log(ks[1]);
console.log(ks[2]);
`
	got := runProgramGo(t, src)
	want := "hi\n3\nname\nage\nactive\n"
	if got != want {
		t.Fatalf("Object.keys run mismatch:\n got %q\nwant %q", got, want)
	}
}
