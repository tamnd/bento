package lower

import (
	"strings"
	"testing"
)

// TestStoredMapEntriesForOfLowers pins that a Map entries() iterator stored in a local
// and driven by one for...of lowers: the declaration emits nothing (no orphaned Go
// value of the iterator-object type that does not lower) and the loop ranges the
// receiver's Keys and Values snapshots the direct m.entries() form ranges.
func TestStoredMapEntriesForOfLowers(t *testing.T) {
	src := `const m = new Map<string, number>([["a", 1], ["b", 2]]);
const it = m.entries();
for (const [k, v] of it) {
  console.log(k, v);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Keys()") || !strings.Contains(out, ".Values()") {
		t.Fatalf("stored map entries for...of did not range the receiver snapshots:\n%s", out)
	}
	if strings.Contains(out, "it :=") || strings.Contains(out, "it =") {
		t.Fatalf("stored iterator binding was not suppressed:\n%s", out)
	}
}

// TestStoredSetValuesForOfLowers pins the single-binding Set form: a stored s.values()
// ranges the Members snapshot.
func TestStoredSetValuesForOfLowers(t *testing.T) {
	src := `const s = new Set<number>([1, 2, 3]);
const it = s.values();
for (const x of it) {
  console.log(x);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, ".Members()") {
		t.Fatalf("stored set values for...of did not range Members:\n%s", out)
	}
}

// TestStoredIterUsedTwiceHandsBack guards the single-use invariant: a stored iterator
// read by two for...of loops would replay the receiver's snapshot from the start the
// second time, which the live iterator would not, so it must not be recorded and the
// unit hands back on the iterator-object declaration.
func TestStoredIterUsedTwiceHandsBack(t *testing.T) {
	src := `const m = new Map<string, number>([["a", 1]]);
const it = m.keys();
for (const k of it) {
  console.log(k);
}
for (const k of it) {
  console.log(k);
}`
	renderProgramHandBack(t, src)
}

// TestStoredIterNonForOfUseHandsBack guards that a stored iterator whose one reference
// is not a for...of iterable (a manual next() drive) is not recorded, so the
// declaration is not suppressed and the unit hands back rather than drop the use.
func TestStoredIterNonForOfUseHandsBack(t *testing.T) {
	src := `const m = new Map<string, number>([["a", 1]]);
const it = m.entries();
console.log(it.next().done);`
	renderProgramHandBack(t, src)
}

// TestStoredIterReassignedReceiverHandsBack guards that a reassigned receiver is not
// recorded: capturing m at the declaration and ranging it at the loop would name a
// different object than the loop-time m if m were reassigned between them, so a written
// receiver hands back.
func TestStoredIterReassignedReceiverHandsBack(t *testing.T) {
	src := `let m = new Map<string, number>([["a", 1]]);
const it = m.keys();
m = new Map<string, number>([["b", 2]]);
for (const k of it) {
  console.log(k);
}`
	renderProgramHandBack(t, src)
}

// TestStoredIterForOfRuns builds and runs every stored form, a Map's entries/keys/
// values and a Set's values/entries, so the snapshot drive is proven to yield the
// receiver's members in insertion order the way the direct call form does.
func TestStoredIterForOfRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const m = new Map<string, number>([["a", 1], ["b", 2]]);
const me = m.entries();
for (const [k, v] of me) {
  console.log(k, v);
}
const mk = m.keys();
for (const k of mk) {
  console.log(k);
}
const mv = m.values();
for (const v of mv) {
  console.log(v);
}
const s = new Set<number>([7, 8]);
const sv = s.values();
for (const x of sv) {
  console.log(x);
}
const se = s.entries();
for (const [a, b] of se) {
  console.log(a, b);
}
`
	got := runProgramGo(t, src)
	want := "a 1\nb 2\na\nb\n1\n2\n7\n8\n7 7\n8 8\n"
	if got != want {
		t.Fatalf("stored iterator for...of run mismatch:\n got %q\nwant %q", got, want)
	}
}
