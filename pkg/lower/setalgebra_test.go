package lower

import (
	"strings"
	"testing"
)

// TestSetAlgebraEmitsMethods pins that the ES2025 set-algebra calls lower to the
// matching value.Set methods over the other set.
func TestSetAlgebraEmitsMethods(t *testing.T) {
	cases := []struct {
		call    string
		emit    string
		observe string // how to reference the result so the program lowers whole
	}{
		{"a.union(b)", "a.Union(b)", "r.size"},
		{"a.intersection(b)", "a.Intersection(b)", "r.size"},
		{"a.difference(b)", "a.Difference(b)", "r.size"},
		{"a.symmetricDifference(b)", "a.SymmetricDifference(b)", "r.size"},
		{"a.isSubsetOf(b)", "a.IsSubsetOf(b)", "r"},
		{"a.isSupersetOf(b)", "a.IsSupersetOf(b)", "r"},
		{"a.isDisjointFrom(b)", "a.IsDisjointFrom(b)", "r"},
	}
	for _, c := range cases {
		src := `
const a = new Set<number>();
a.add(1);
const b = new Set<number>();
b.add(1);
const r = ` + c.call + `;
console.log(` + c.observe + `);
`
		out := renderProgram(t, src)
		if !strings.Contains(out, c.emit) {
			t.Errorf("%s did not lower to %s, got:\n%s", c.call, c.emit, out)
		}
	}
}

// TestSetAlgebraMismatchedMemberTypeHandsBack pins that a set-algebra call
// between sets of different member types hands back, since the runtime method
// takes another set of the receiver's member type and the two would not share a
// Go type.
func TestSetAlgebraMismatchedMemberTypeHandsBack(t *testing.T) {
	src := `
const a = new Set<number>();
a.add(1);
const b = new Set<string>();
b.add("x");
console.log(a.union(b).size);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "different member types") {
		t.Fatalf("expected a mismatched-member-type handback, got: %q", reason)
	}
}

// TestSetAlgebraRuns builds and runs the emitted Go and checks each operation
// against the Node oracle. It probes the result sets by size and membership
// rather than iterating them, since spread over a set still waits on iterator
// lowering; the result order is pinned at the value level.
func TestSetAlgebraRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const a = new Set<number>();
a.add(1); a.add(2); a.add(3);
const b = new Set<number>();
b.add(3); b.add(4); b.add(5);

const u = a.union(b);
console.log(u.size, u.has(1), u.has(5), u.has(6));

const i = a.intersection(b);
console.log(i.size, i.has(3), i.has(1));

const d = a.difference(b);
console.log(d.size, d.has(1), d.has(3));

const s = a.symmetricDifference(b);
console.log(s.size, s.has(1), s.has(3), s.has(4));

console.log(a.isSubsetOf(b), a.isSupersetOf(b), a.isDisjointFrom(b));

const sub = new Set<number>();
sub.add(1); sub.add(2);
console.log(sub.isSubsetOf(a), a.isSupersetOf(sub), sub.isDisjointFrom(b));
`
	got := runProgramGo(t, src)
	want := "5 true true false\n" +
		"1 true false\n" +
		"2 true false\n" +
		"4 true false true\n" +
		"false false false\n" +
		"true true true\n"
	if got != want {
		t.Fatalf("set algebra run mismatch:\n got %q\nwant %q", got, want)
	}
}
