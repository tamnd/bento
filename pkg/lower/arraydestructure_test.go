package lower

import (
	"strings"
	"testing"
)

// An array destructuring binding, const [a, b] = src, binds each name to the
// element at its position. Go has no destructuring, so it lowers to one short
// declaration per element reading through AtI, the same indexed read a written-out
// element access lowers to. The source must be a plain variable so the read repeats
// without re-evaluating it, and the pattern must be flat names whose types match the
// array element type.

// TestArrayDestructureBindsElements proves each name lowers to a positional AtI read
// off the source variable.
func TestArrayDestructureBindsElements(t *testing.T) {
	const src = "const pair: number[] = [10, 20];\nconst [a, b] = pair;\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a := pair.AtI(0)") {
		t.Errorf("first element did not lower to pair.AtI(0):\n%s", source)
	}
	if !strings.Contains(source, "b := pair.AtI(1)") {
		t.Errorf("second element did not lower to pair.AtI(1):\n%s", source)
	}
}

// TestArrayDestructureRuns builds and runs a numeric destructure so the positional
// reads are proven to pick the right elements.
func TestArrayDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const pair: number[] = [10, 20, 30];
const [a, b, c] = pair;
console.log(a);
console.log(b);
console.log(c);
`
	if got, want := runProgramGo(t, src), "10\n20\n30\n"; got != want {
		t.Fatalf("array destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureStrings proves the binding works for a string array, where the
// element read carries the string element type through to each name.
func TestArrayDestructureStrings(t *testing.T) {
	skipIfShort(t)
	const src = `
const names: string[] = ["alice", "bob"];
const [first, second] = names;
console.log(first);
console.log(second);
`
	if got, want := runProgramGo(t, src), "alice\nbob\n"; got != want {
		t.Fatalf("string destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureSingle proves a one-element pattern binds the leading element.
func TestArrayDestructureSingle(t *testing.T) {
	skipIfShort(t)
	const src = `
const xs: number[] = [7, 8, 9];
const [only] = xs;
console.log(only);
`
	if got, want := runProgramGo(t, src), "7\n"; got != want {
		t.Fatalf("single destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureHoleHandsBack proves a pattern with a hole hands back, since a
// skipped position is a later slice.
func TestArrayDestructureHoleHandsBack(t *testing.T) {
	const src = "const pair: number[] = [1, 2];\nconst [, b] = pair;\nconsole.log(b);\n"
	renderProgramHandBack(t, src)
}

// TestArrayDestructureDefaultHandsBack proves an element default hands back, since
// filling an undefined element with the default is a later slice.
func TestArrayDestructureDefaultHandsBack(t *testing.T) {
	const src = "const pair: number[] = [1, 2];\nconst [a = 5, b] = pair;\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}

// TestArrayDestructureRestHandsBack proves a rest element hands back, since gathering
// the tail into an array is a later slice.
func TestArrayDestructureRestHandsBack(t *testing.T) {
	const src = "const pair: number[] = [1, 2, 3];\nconst [a, ...rest] = pair;\nconsole.log(a);\nconsole.log(rest.length);\n"
	renderProgramHandBack(t, src)
}

// TestArrayDestructureNestedHandsBack proves a nested pattern hands back, since a
// pattern inside a pattern is a later slice.
func TestArrayDestructureNestedHandsBack(t *testing.T) {
	const src = "const grid: number[][] = [[1], [2]];\nconst [[a], b] = grid;\nconsole.log(a);\n"
	renderProgramHandBack(t, src)
}

// TestArrayDestructureCallSourceLowersToTemp proves a non-variable array source, a
// call returning an array, lowers by holding the source in a generated temporary read
// once, then reading each element off that temporary, so the source is evaluated once.
func TestArrayDestructureCallSourceLowersToTemp(t *testing.T) {
	const src = "function pair(): number[] { return [10, 20]; }\nconst [a, b] = pair();\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ":= Pair()") {
		t.Errorf("call source was not held in a temporary:\n%s", source)
	}
	if !strings.Contains(source, ".AtI(0)") || !strings.Contains(source, ".AtI(1)") {
		t.Errorf("elements did not read off the temporary through AtI:\n%s", source)
	}
}

// TestArrayDestructureCallSourceRuns builds and runs a call-source destructure so the
// evaluate-once temporary is proven to feed the positional reads.
func TestArrayDestructureCallSourceRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function pair(): number[] { return [10, 20]; }
const [a, b] = pair();
console.log(a);
console.log(b);
`
	if got, want := runProgramGo(t, src), "10\n20\n"; got != want {
		t.Fatalf("call-source array destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureTupleSourceHandsBack proves a tuple-literal source hands back,
// since a tuple has no single array element type to read every name through AtI.
func TestArrayDestructureTupleSourceHandsBack(t *testing.T) {
	const src = "const [a, b] = [10, 20];\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}
