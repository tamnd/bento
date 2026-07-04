package lower

import (
	"strings"
	"testing"
)

// TestArraySomeEmitsMethod pins that some over an inline arrow lowers to the
// value.Array Some method applied to the lowered callback.
func TestArraySomeEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.some((n: number): boolean => n > 2));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Some(func(n float64) bool") {
		t.Errorf("some did not lower to the Some method over an arrow:\n%s", source)
	}
}

// TestArrayForEachEmitsVoidCallback pins that forEach lowers to the ForEach
// method over a callback with no results, so the call stands as a statement.
func TestArrayForEachEmitsVoidCallback(t *testing.T) {
	src := "const a = [1, 2, 3];\na.forEach((n: number): void => { console.log(n); });\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ForEach(func(n float64)") {
		t.Errorf("forEach did not lower to the ForEach method over a void callback:\n%s", source)
	}
}

// TestArrayEveryIndexCallbackHandsBack pins that a predicate callback that also
// reads the index parameter hands back, since the value method takes a
// single-element func the two-parameter arrow could not satisfy.
func TestArrayEveryIndexCallbackHandsBack(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.every((n: number, i: number): boolean => n > i));\n"
	renderProgramHandBack(t, src)
}

// TestArrayFindEmitsMethod pins that find over an inline arrow lowers to the
// value.Array Find method, whose Opt[T] result carries the T | undefined type.
func TestArrayFindEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconst r = a.find((n: number): boolean => n > 1);\nif (r !== undefined) { console.log(r); }\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Find(func(n float64) bool") {
		t.Errorf("find did not lower to the Find method over an arrow:\n%s", source)
	}
}

// TestArrayFindIndexEmitsMethod pins that findIndex lowers to the FindIndex
// method, whose float64 result carries the -1 not-found sentinel.
func TestArrayFindIndexEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.findIndex((n: number): boolean => n > 1));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".FindIndex(func(n float64) bool") {
		t.Errorf("findIndex did not lower to the FindIndex method over an arrow:\n%s", source)
	}
}

// TestArrayFindRuns builds and runs find and findIndex against the Node oracle:
// find on a hit narrowed through !== undefined and on a miss, and findIndex on
// both a hit and a miss returning its -1 sentinel.
func TestArrayFindRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [5, 10, 15, 20];
const hit = a.find((n: number): boolean => n > 12);
if (hit !== undefined) {
  console.log(hit);
}
const miss = a.find((n: number): boolean => n > 99);
console.log(miss === undefined);
console.log(a.findIndex((n: number): boolean => n === 15));
console.log(a.findIndex((n: number): boolean => n === 99));
`
	got := runProgramGo(t, src)
	want := "15\ntrue\n2\n-1\n"
	if got != want {
		t.Fatalf("array find program printed %q, want %q", got, want)
	}
}

// TestArrayPredicatesRun builds and runs some, every, and forEach against the
// Node oracle: some and every over both a satisfied and an unsatisfied
// predicate, the empty-array vacuous cases, and forEach accumulating a running
// sum through a captured local.
func TestArrayPredicatesRun(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
const empty: number[] = [];
console.log(a.some((n: number): boolean => n > 3));
console.log(a.some((n: number): boolean => n > 9));
console.log(a.every((n: number): boolean => n > 0));
console.log(a.every((n: number): boolean => n > 2));
console.log(empty.some((n: number): boolean => n > 0));
console.log(empty.every((n: number): boolean => n > 0));
let sum = 0;
a.forEach((n: number): void => {
  sum += n;
});
console.log(sum);
`
	got := runProgramGo(t, src)
	want := "true\nfalse\ntrue\nfalse\nfalse\ntrue\n10\n"
	if got != want {
		t.Fatalf("array predicate program printed %q, want %q", got, want)
	}
}
