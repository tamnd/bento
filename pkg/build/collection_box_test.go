package build

import "testing"

// A collection that crosses into a dynamic slot is a run-time surface, not an
// emission: what matters is what the built binary prints when a program logs a Map,
// walks one it got from a call, or asserts two are deep equal. These build real
// binaries and hold their whole output against what Node v24.18.0 prints for the same
// program, which is the only check that says the box is compatible rather than
// merely present.

// TestABoxedCollectionPrintsAndWalksLikeNode is one program covering the surface a
// dynamic collection presents: how it prints, that it carries no own keys, the
// iteration protocols, and the member reads. Every expected line came from running
// the same source under Node.
func TestABoxedCollectionPrintsAndWalksLikeNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const m = new Map([[1, 2], [3, 4]]);\n"+
			"console.log(m);\n"+
			"console.log(JSON.stringify(m), Object.keys(m).length, Object.entries(m).length, typeof m);\n"+
			"let seen = '';\n"+
			"for (const [k, v] of m) { seen += k + ':' + v + ' '; }\n"+
			"console.log(seen);\n"+
			"console.log([...m.keys()].join(','), [...m.values()].join(','));\n"+
			"const s = new Set(['a', 'b']);\n"+
			"console.log(s, s.size, s.has('a'), s.has('z'));\n")
	want := "Map(2) { 1 => 2, 3 => 4 }\n" +
		"{} 0 0 object\n" +
		"1:2 3:4 \n" +
		"1,3 2,4\n" +
		"Set(2) { 'a', 'b' } 2 true false\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}

// TestAssertOverCollectionsMatchesNode covers the reason the box was worth building
// before the rest of the wall: assert reads its arguments as dynamic values, so a
// deep comparison of two Maps went through the box or not at all. The failure message
// is part of the check, since a program's test output is the diff and not just the
// throw.
func TestAssertOverCollectionsMatchesNode(t *testing.T) {
	got := buildAndRunFile(t, "main.js",
		"const assert = require('assert');\n"+
			"assert.deepStrictEqual(new Map([[1, 2]]), new Map([[1, 2]]));\n"+
			"assert.deepStrictEqual(new Set([1]), new Set([1]));\n"+
			"try {\n"+
			"  assert.deepStrictEqual(new Map([[1, 2]]), new Map([[1, 3]]));\n"+
			"} catch (e) {\n"+
			"  console.log(e.message);\n"+
			"}\n"+
			"console.log('done');\n")
	want := "Expected values to be strictly deep-equal:\n" +
		"+ actual - expected\n" +
		"\n" +
		"  Map(1) {\n" +
		"+   1 => 2\n" +
		"-   1 => 3\n" +
		"  }\n" +
		"\ndone\n"
	if got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
