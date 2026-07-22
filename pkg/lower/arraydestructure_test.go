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

// TestArrayDestructureDefaultRuns proves an element default lowers: a present slot
// keeps its value and a missing slot takes the default.
func TestArrayDestructureDefaultRuns(t *testing.T) {
	const src = "const pair: number[] = [1, 2];\nconst [a = 5, b] = pair;\nconsole.log(a + b);\n"
	if got, want := runProgramGo(t, src), "3\n"; got != want {
		t.Fatalf("array default destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureRestRuns proves a rest element gathers the tail past the fixed
// slots into a fresh array, binding the head by index.
func TestArrayDestructureRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const pair: number[] = [1, 2, 3];\nconst [a, ...rest] = pair;\nconsole.log(a);\nconsole.log(rest.length);\n"
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("array rest destructure printed %q, want %q", got, want)
	}
}

// TestArrayNestedInArrayLowers proves an array pattern nested inside an array pattern
// reads the inner element from the slot the outer pattern selected: the outer slot is
// held in a temporary, then the inner pattern reads off it by index.
func TestArrayNestedInArrayLowers(t *testing.T) {
	const src = "const grid: number[][] = [[1, 2], [3, 4]];\nconst [[a, b], [c, d]] = grid;\nconsole.log(a + b + c + d);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".AtI(0).AtI(0)") && !strings.Contains(source, "a := ") {
		t.Errorf("nested array pattern did not read the inner element off the outer slot:\n%s", source)
	}
}

// TestArrayNestedInArrayRuns builds and runs a two-level array destructure so each
// inner name is proven to carry the element the outer slot held.
func TestArrayNestedInArrayRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const grid: number[][] = [[1, 2], [3, 4]];
const [[a, b], [c, d]] = grid;
console.log(a);
console.log(b);
console.log(c);
console.log(d);
`
	if got, want := runProgramGo(t, src), "1\n2\n3\n4\n"; got != want {
		t.Fatalf("nested array destructure printed %q, want %q", got, want)
	}
}

// TestObjectNestedInArrayRuns proves an object pattern nested inside an array pattern
// reads the object off the slot the array pattern selected, then binds the inner
// properties, so the two shapes cross.
func TestObjectNestedInArrayRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const arr: { x: number; y: number }[] = [{ x: 1, y: 2 }, { x: 3, y: 4 }];
const [{ x, y }, { x: p, y: q }] = arr;
console.log(x);
console.log(y);
console.log(p);
console.log(q);
`
	if got, want := runProgramGo(t, src), "1\n2\n3\n4\n"; got != want {
		t.Fatalf("object-in-array destructure printed %q, want %q", got, want)
	}
}

// TestArrayNestedDefaultRuns proves a default inside a nested array pattern composes
// the fill through the nesting: the inner slot takes the default when the outer slot's
// array has no element there, and its own value otherwise.
func TestArrayNestedDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const grid: number[][] = [[1], [3, 4]];
const [[a, b = 9], [c, d]] = grid;
console.log(a);
console.log(b);
console.log(c);
console.log(d);
`
	if got, want := runProgramGo(t, src), "1\n9\n3\n4\n"; got != want {
		t.Fatalf("nested array default destructure printed %q, want %q", got, want)
	}
}

// TestArrayNestedRestRuns proves a rest inside a nested array pattern gathers the tail
// past the inner fixed slots, composing the gather rule through the nesting.
func TestArrayNestedRestRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const grid: number[][] = [[1, 2, 3], [4, 5]];
const [[a, ...rest], [b]] = grid;
console.log(a);
console.log(rest.length);
console.log(b);
`
	if got, want := runProgramGo(t, src), "1\n2\n4\n"; got != want {
		t.Fatalf("nested array rest destructure printed %q, want %q", got, want)
	}
}

// TestArrayNestedInAssignmentRuns proves a nested array pattern in an assignment
// target stores each leaf into its existing local through the nesting, reading each
// inner element off the slot the outer pattern selected on the source variable.
func TestArrayNestedInAssignmentRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const m: number[][] = [[1, 2], [3, 4]];
let a = 0, b = 0, c = 0, d = 0;
([[a, b], [c, d]] = m);
console.log(a);
console.log(b);
console.log(c);
console.log(d);
`
	if got, want := runProgramGo(t, src), "1\n2\n3\n4\n"; got != want {
		t.Fatalf("nested array assignment printed %q, want %q", got, want)
	}
}

// TestArrayNestedInAssignmentDefaultRuns proves a default inside a nested array
// assignment pattern fills the leaf from the outer slot when that slot has no element
// there, composing the fill rule through the nesting into the existing target.
func TestArrayNestedInAssignmentDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const m: number[][] = [[1], [3, 4]];
let a = 0, b = 0, c = 0, d = 0;
([[a, b = 9], [c, d]] = m);
console.log(a);
console.log(b);
console.log(c);
console.log(d);
`
	if got, want := runProgramGo(t, src), "1\n9\n3\n4\n"; got != want {
		t.Fatalf("nested array assignment default printed %q, want %q", got, want)
	}
}

// TestArrayMemberTargetAssignmentRuns proves an array destructuring assignment whose
// targets are property accesses stores each element into the member target, reading the
// source elements by index and landing them in the object fields.
func TestArrayMemberTargetAssignmentRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const o = { a: 0, b: 0 };
const xs: number[] = [1, 2];
[o.a, o.b] = xs;
console.log(o.a);
console.log(o.b);
`
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("array member-target assignment printed %q, want %q", got, want)
	}
}

// TestArrayMixedMemberTargetAssignmentRuns proves a target list that mixes a plain
// local with a member target stores each element into its own kind of target in one
// parallel assignment.
func TestArrayMixedMemberTargetAssignmentRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let a = 0;
const o = { b: 0 };
const xs: number[] = [7, 8];
[a, o.b] = xs;
console.log(a);
console.log(o.b);
`
	if got, want := runProgramGo(t, src), "7\n8\n"; got != want {
		t.Fatalf("mixed member-target assignment printed %q, want %q", got, want)
	}
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

// TestArrayDestructureTupleSourceRuns proves a tuple source destructures through its
// positional fields rather than the array AtI read: the checker types a bare
// [10, 20] as the tuple [number, number], so each name binds from its E<i> field
// (typed/05 delivery slice 5), and the compiled program prints the same sum the
// TypeScript does.
func TestArrayDestructureTupleSourceRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const [a, b] = [10, 20];\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".E0") || !strings.Contains(source, ".E1") {
		t.Errorf("tuple source did not read positional fields:\n%s", source)
	}
	if got, want := runProgramGo(t, src), "30\n"; got != want {
		t.Fatalf("tuple source destructure printed %q, want %q", got, want)
	}
}

// TestAnnotatedArrayDestructureRuns proves a type annotation on the binding does
// not derail the destructure: `let [x]: [number] = [1]` still binds x by a
// positional read the way the unannotated form does, rather than fall to the
// plain path that would take the pattern text as a Go variable name.
func TestAnnotatedArrayDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function f(): number {
  let [x]: [number] = [7];
  return x;
}
console.log(f());
`
	source := renderProgram(t, src)
	if strings.Contains(source, "U5B_") {
		t.Fatalf("annotated array destructure took the pattern text as a name:\n%s", source)
	}
	if got, want := runProgramGo(t, src), "7\n"; got != want {
		t.Fatalf("annotated array destructure printed %q, want %q", got, want)
	}
}

// TestAnnotatedObjectDestructureRuns proves the same for an object pattern: the
// type annotation between the pattern and the initializer does not stop the
// destructure from reading each shorthand property.
func TestAnnotatedObjectDestructureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function f(): number {
  let { y }: { y: number } = { y: 9 };
  return y;
}
console.log(f());
`
	source := renderProgram(t, src)
	if strings.Contains(source, "U7B_") {
		t.Fatalf("annotated object destructure took the pattern text as a name:\n%s", source)
	}
	if got, want := runProgramGo(t, src), "9\n"; got != want {
		t.Fatalf("annotated object destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureMemberSourceLowersToTemp proves a member-read array source is
// held in a generated temporary read once, extending the call-source case in #258 to
// a property access.
func TestArrayDestructureMemberSourceLowersToTemp(t *testing.T) {
	const src = "const o = { pair: [3, 4] };\nconst [a, b] = o.pair;\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, ".AtI(0)") || !strings.Contains(source, ".AtI(1)") {
		t.Errorf("member source did not read elements through AtI:\n%s", source)
	}
}

// TestArrayDestructureMemberSourceRuns builds and runs a member-source destructure so
// the held-once temporary is proven to feed the positional reads.
func TestArrayDestructureMemberSourceRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o = { pair: [3, 4] };\nconst [a, b] = o.pair;\nconsole.log(a);\nconsole.log(b);\n"
	if got, want := runProgramGo(t, src), "3\n4\n"; got != want {
		t.Fatalf("member-source array destructure printed %q, want %q", got, want)
	}
}

// TestArrayDestructureCallSourceEvaluatesOnce builds and runs a call-source
// destructure whose source increments a counter, so the held-once temporary is proven
// to run the call a single time rather than once per bound element.
func TestArrayDestructureCallSourceEvaluatesOnce(t *testing.T) {
	skipIfShort(t)
	const src = `let calls = 0;
function make(): number[] { calls += 1; return [1, 2]; }
const [a, b] = make();
console.log(a + b);
console.log(calls);
`
	if got, want := runProgramGo(t, src), "3\n1\n"; got != want {
		t.Fatalf("call-source array destructure printed %q, want %q (source should run once)", got, want)
	}
}
