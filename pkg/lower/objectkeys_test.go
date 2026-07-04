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
