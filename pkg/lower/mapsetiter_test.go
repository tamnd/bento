package lower

import (
	"strings"
	"testing"
)

// A for...of over a Map, its entries() spelling, or a Set's entries() with a single
// binding yields one [key, value] pair per turn that the body reads through the bound
// name. The pair has to exist as a value, so it materializes the interned positional
// tuple: e[0] reads position 0 and e[1] position 1. A Map pairs its keys with their
// own values in insertion order off the Keys/Values snapshots; a Set's entries pair is
// the member twice.

// TestForOfMapPairSingleBuildsTuple proves a Map for-of with one binding ranges the
// key snapshot, pulls the value by index, and packs both into the interned tuple.
func TestForOfMapPairSingleBuildsTuple(t *testing.T) {
	const src = "const m = new Map<string, number>();\nm.set(\"a\", 1);\nfor (const e of m) {\n  console.log(e[0] + \"=\" + e[1]);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Keys()") || !strings.Contains(source, ".Values()") {
		t.Errorf("Map pair loop did not range the Keys/Values snapshots:\n%s", source)
	}
	if !strings.Contains(source, "Tuple_str_num{") {
		t.Errorf("Map pair loop did not build the interned [key, value] tuple:\n%s", source)
	}
	if strings.Contains(source, ".AtI(") {
		t.Errorf("Map pair loop should read tuple fields, not array positions:\n%s", source)
	}
}

// TestForOfMapEntriesPairSingleBuildsTuple proves the entries() spelling lowers the
// same way as the default iterator.
func TestForOfMapEntriesPairSingleBuildsTuple(t *testing.T) {
	const src = "const m = new Map<string, number>();\nm.set(\"a\", 1);\nfor (const e of m.entries()) {\n  console.log(e[0] + \":\" + e[1]);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Tuple_str_num{") {
		t.Errorf("entries() pair loop did not build the interned tuple:\n%s", source)
	}
}

// TestForOfSetEntriesPairSingleBuildsTuple proves a Set's entries() pairs the member
// with itself in the tuple.
func TestForOfSetEntriesPairSingleBuildsTuple(t *testing.T) {
	const src = "const s = new Set<number>();\ns.add(10);\nfor (const p of s.entries()) {\n  console.log(p[0] + \",\" + p[1]);\n}\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Members()") {
		t.Errorf("Set entries loop did not range the Members snapshot:\n%s", source)
	}
	if !strings.Contains(source, "Tuple_num_num{") {
		t.Errorf("Set entries loop did not build the interned [v, v] tuple:\n%s", source)
	}
}

// TestForOfMapPairUnusedBindingDropsTuple proves a loop that never reads its binding
// ranges one snapshot with no tuple built, so the counting idiom compiles clean.
func TestForOfMapPairUnusedBindingDropsTuple(t *testing.T) {
	const src = "const m = new Map<string, number>();\nm.set(\"a\", 1);\nlet count = 0;\nfor (const e of m) {\n  count = count + 1;\n}\nconsole.log(count);\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "Tuple_") {
		t.Errorf("unused-binding loop should not build a tuple:\n%s", source)
	}
	if !strings.Contains(source, ".Keys()") {
		t.Errorf("unused-binding loop should still range a snapshot:\n%s", source)
	}
}

// TestForOfMapSetPairSingleRuns builds and runs each pair form so the tuple reads are
// proven to pick the right key and value each turn.
func TestForOfMapSetPairSingleRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const m = new Map<string, number>();
m.set("a", 1);
m.set("b", 2);
for (const e of m) {
  console.log(e[0] + "=" + e[1]);
}
for (const e of m.entries()) {
  console.log(e[0] + ":" + e[1]);
}
const s = new Set<number>();
s.add(10);
for (const p of s.entries()) {
  console.log(p[0] + "," + p[1]);
}
`
	want := "a=1\nb=2\na:1\nb:2\n10,10\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("map/set pair for-of printed %q, want %q", got, want)
	}
}

// TestSpreadMapDirectBuildsTupleSlice proves a spread of a Map used directly collects
// its Keys/Values snapshots into a fresh slice of the interned tuple the append splices.
func TestSpreadMapDirectBuildsTupleSlice(t *testing.T) {
	const src = "const m = new Map<string, number>();\nm.set(\"a\", 1);\nconst a = [...m];\nconsole.log(a[0][0]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Keys()") || !strings.Contains(source, ".Values()") {
		t.Errorf("map spread did not collect the Keys/Values snapshots:\n%s", source)
	}
	if !strings.Contains(source, "[]Tuple_str_num{") && !strings.Contains(source, "make([]Tuple_str_num") {
		t.Errorf("map spread did not build a slice of the interned tuple:\n%s", source)
	}
	if !strings.Contains(source, "append(") {
		t.Errorf("map spread did not splice the tuple slice with append:\n%s", source)
	}
}

// TestSpreadMapEntriesBuildsTupleSlice proves the entries() spelling spreads the same
// way as the default iterator.
func TestSpreadMapEntriesBuildsTupleSlice(t *testing.T) {
	const src = "const m = new Map<string, number>();\nm.set(\"a\", 1);\nconst a = [...m.entries()];\nconsole.log(a[0][0]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "Tuple_str_num{") {
		t.Errorf("entries() spread did not build the interned tuple:\n%s", source)
	}
}

// TestSpreadSetEntriesBuildsTupleSlice proves a Set's entries() spreads a [v, v] tuple
// per member off the Members snapshot.
func TestSpreadSetEntriesBuildsTupleSlice(t *testing.T) {
	const src = "const s = new Set<number>();\ns.add(10);\nconst a = [...s.entries()];\nconsole.log(a[0][0]);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Members()") {
		t.Errorf("set entries spread did not range the Members snapshot:\n%s", source)
	}
	if !strings.Contains(source, "Tuple_num_num{") {
		t.Errorf("set entries spread did not build the interned [v, v] tuple:\n%s", source)
	}
}

// TestSpreadMapEntriesRuns builds and runs each spread form so the collected tuples are
// proven to carry the right pairs in insertion order, including destructuring off the
// spread result.
func TestSpreadMapEntriesRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const m = new Map<string, number>();
m.set("a", 1);
m.set("b", 2);
const pairs = [...m];
console.log(pairs.length);
console.log(pairs[0][0] + "=" + pairs[0][1]);
const ents = [...m.entries()];
console.log(ents[1][0] + ":" + ents[1][1]);
const s = new Set<number>();
s.add(10);
const svs = [...s.entries()];
console.log(svs[0][0] + "," + svs[0][1]);
for (const [k, v] of [...m]) {
  console.log(k + "->" + v);
}
`
	want := "2\na=1\nb:2\n10,10\na->1\nb->2\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("map/set entries spread printed %q, want %q", got, want)
	}
}
