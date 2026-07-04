package lower

import (
	"strings"
	"testing"
)

// TestArraySpreadEmitsAppendChain pins that an array literal with a spread
// lowers to a value.ArrayFrom over an append chain, the same splice a person
// would write in Go, rather than handing back.
func TestArraySpreadEmitsAppendChain(t *testing.T) {
	src := "const a = [1, 2];\nconst b = [0, ...a, 3];\nconsole.log(b.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ArrayFrom(append(append([]float64{0}, a.Elems()...), 3))") {
		t.Errorf("array spread did not lower to the expected append chain:\n%s", source)
	}
}

// TestArraySpreadOnlyEmitsSeededAppend pins that a lone spread [...a] seeds the
// chain with an empty typed slice so the result owns fresh storage and aliases
// none of the spread source.
func TestArraySpreadOnlyEmitsSeededAppend(t *testing.T) {
	src := "const a = [1, 2, 3];\nconst b = [...a];\nconsole.log(b.length);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ArrayFrom(append([]float64{}, a.Elems()...))") {
		t.Errorf("lone array spread did not lower to a seeded append:\n%s", source)
	}
}

// TestArraySpreadOfStringHandsBack pins that spreading a string, which is
// iterable in JavaScript but not a value.Array in Go, hands the unit back rather
// than emitting an Elems call on a value that has none.
func TestArraySpreadOfStringHandsBack(t *testing.T) {
	src := "const s = \"ab\";\nconst b = [...s];\nconsole.log(b.length);\n"
	renderProgramHandBack(t, src)
}

// TestArraySpreadRuns builds and runs spliced-array code against the Node
// oracle: a spread in the middle, a leading and trailing spread, a lone spread,
// and two spreads in one literal, reading back their lengths and elements.
func TestArraySpreadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3];
const b = [10, 20];
const mid = [0, ...a, 4];
const both = [...a, ...b];
const copy = [...a];
console.log(mid.length);
console.log(mid[0]);
console.log(mid[4]);
console.log(both.length);
console.log(both[3]);
console.log(copy.length);
console.log(copy[2]);
`
	got := runProgramGo(t, src)
	want := "5\n0\n4\n5\n10\n3\n3\n"
	if got != want {
		t.Fatalf("array spread program printed %q, want %q", got, want)
	}
}
