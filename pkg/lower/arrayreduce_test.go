package lower

import (
	"strings"
	"testing"
)

// TestArrayReduceSameTypeEmitsFreeFunc pins that reduce over a number array into
// a number accumulator lowers to value.Reduce with both type arguments spelled
// out, the element and accumulator Go types.
func TestArrayReduceSameTypeEmitsFreeFunc(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduce((acc: number, n: number): number => acc + n, 0));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Reduce[float64, float64](") {
		t.Errorf("same-type reduce did not lower to value.Reduce[float64, float64]:\n%s", source)
	}
}

// TestArrayReduceChangingTypeEmitsAccType pins that a reduce whose accumulator
// type differs from the element type spells that accumulator type as the second
// type argument, so a string fold over a number array reads value.Reduce with a
// value.BStr accumulator.
func TestArrayReduceChangingTypeEmitsAccType(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduce((acc: string, n: number): string => acc + n, \"\"));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.Reduce[float64, value.BStr](") {
		t.Errorf("type-changing reduce did not spell the accumulator type:\n%s", source)
	}
}

// TestArrayReduceNoInitHandsBack pins that reduce without an initial value, the
// first-element-seed form that throws on an empty array, hands the unit back.
func TestArrayReduceNoInitHandsBack(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduce((acc: number, n: number): number => acc + n));\n"
	renderProgramHandBack(t, src)
}

// TestArrayReduceRuns builds and runs reduce against the Node oracle: a numeric
// sum, a numeric product from a non-zero seed, a string fold that changes the
// accumulator type, and an empty array returning the initial value unchanged.
func TestArrayReduceRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
console.log(a.reduce((acc: number, n: number): number => acc + n, 0));
console.log(a.reduce((acc: number, n: number): number => acc * n, 1));
console.log(a.reduce((acc: string, n: number): string => acc + n, "="));
const empty: number[] = [];
console.log(empty.reduce((acc: number, n: number): number => acc + n, 42));
`
	got := runProgramGo(t, src)
	want := "10\n24\n=1234\n42\n"
	if got != want {
		t.Fatalf("array reduce program printed %q, want %q", got, want)
	}
}
