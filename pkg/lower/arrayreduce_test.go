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

// TestArrayReduceNoInitEmitsMethod pins that reduce without an initial value, the
// first-element-seed form that throws on an empty array, lowers to the
// value.Array ReduceNoInit method rather than the two-type-argument free function.
func TestArrayReduceNoInitEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduce((acc: number, n: number): number => acc + n));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ReduceNoInit(") {
		t.Errorf("no-init reduce did not lower to ReduceNoInit:\n%s", source)
	}
}

// TestArrayReduceNoInitRuns builds and runs the no-init form against the Node
// oracle: a sum that seeds from the first element and a single-element array that
// returns its element without running the callback.
func TestArrayReduceNoInitRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3, 4];
console.log(a.reduce((acc: number, n: number): number => acc + n));
console.log([7].reduce((acc: number, n: number): number => acc + n));
`
	got := runProgramGo(t, src)
	want := "10\n7\n"
	if got != want {
		t.Fatalf("no-init reduce program printed %q, want %q", got, want)
	}
}

// TestArrayReduceRightSameTypeEmitsFreeFunc pins that reduceRight over a number
// array into a number accumulator lowers to value.ReduceRight with both type
// arguments spelled out.
func TestArrayReduceRightSameTypeEmitsFreeFunc(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduceRight((acc: number, n: number): number => acc - n, 0));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "value.ReduceRight[float64, float64](") {
		t.Errorf("same-type reduceRight did not lower to value.ReduceRight[float64, float64]:\n%s", source)
	}
}

// TestArrayReduceRightNoInitEmitsMethod pins that reduceRight with no initial
// value lowers to the value.Array ReduceRightNoInit method.
func TestArrayReduceRightNoInitEmitsMethod(t *testing.T) {
	src := "const a = [1, 2, 3];\nconsole.log(a.reduceRight((acc: number, n: number): number => acc - n));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".ReduceRightNoInit(") {
		t.Errorf("no-init reduceRight did not lower to ReduceRightNoInit:\n%s", source)
	}
}

// TestArrayReduceRightRuns builds and runs reduceRight against the Node oracle,
// covering the initial-value fold, a type-changing string fold, and the no-init
// form that seeds from the last element.
func TestArrayReduceRightRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const a = [1, 2, 3];
console.log(a.reduceRight((acc: number, n: number): number => acc - n, 0));
console.log(a.reduceRight((acc: string, n: number): string => acc + n, "="));
console.log([1, 2, 3, 10].reduceRight((acc: number, n: number): number => acc - n));
`
	got := runProgramGo(t, src)
	want := "-6\n=321\n4\n"
	if got != want {
		t.Fatalf("reduceRight program printed %q, want %q", got, want)
	}
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
