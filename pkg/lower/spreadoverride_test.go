package lower

import (
	"strings"
	"testing"
)

// TestSpreadOverriddenSourceBlanked pins that an object-spread source every one of
// whose fields a later spread overrides is blanked, not left declared and not used.
// The later source supplies every field, so the earlier source contributes no
// surviving field read to the composite literal, yet its identifier was read once
// by the spread; the emit must mark it used so Go accepts the declaration.
func TestSpreadOverriddenSourceBlanked(t *testing.T) {
	const src = `declare const a: { x: number }
declare const b: { x: number }
const c = { ...a, ...b };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "_ = a") {
		t.Fatalf("expected the fully overridden spread source a to be blanked:\n%s", out)
	}
}

// TestSpreadOverriddenSourceRuns builds and runs the override so the blanked source
// is proven inert: its initializer still evaluates, the later source wins every
// field, and the result reads the overriding value.
func TestSpreadOverriddenSourceRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = { x: 1 };
const b = { x: 2 };
const c = { ...a, ...b };
console.log(c.x);`
	got := runProgramGo(t, src)
	want := "2\n"
	if got != want {
		t.Fatalf("spread override run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestSpreadPartlyOverriddenSourceKept pins the guard's boundary: a source with a
// field no later member overrides still contributes that field, so it is read and
// must not be blanked, else a live read would be dropped.
func TestSpreadPartlyOverriddenSourceKept(t *testing.T) {
	skipIfShort(t)
	const src = `const a = { x: 1, y: 9 };
const b = { x: 2 };
const c = { ...a, ...b };
console.log(c.x + c.y);`
	got := runProgramGo(t, src)
	want := "11\n"
	if got != want {
		t.Fatalf("partial spread override run mismatch:\n got %q\nwant %q", got, want)
	}
}
