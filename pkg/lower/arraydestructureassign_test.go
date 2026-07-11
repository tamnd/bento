package lower

import (
	"strings"
	"testing"
)

// An array destructuring assignment, [a, b] = rhs, assigns each already-declared
// target from the right side by position. It lowers to a single Go parallel
// assignment, which evaluates every right-hand side before assigning any target, the
// same order the destructuring assignment has. That order is what makes the swap
// idiom [a, b] = [b, a] fall out as the idiomatic a, b = b, a.

// TestArrayDestructureAssignFromVariable proves a plain array source reads each target
// through AtI in one parallel assignment.
func TestArrayDestructureAssignFromVariable(t *testing.T) {
	const src = "let a = 0;\nlet b = 0;\nconst pair: number[] = [10, 20];\n[a, b] = pair;\nconsole.log(a + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a, b = pair.AtI(0), pair.AtI(1)") {
		t.Errorf("array assignment did not lower to a parallel AtI read:\n%s", source)
	}
}

// TestArrayDestructureAssignSwap proves the swap idiom lowers to Go's parallel swap,
// where the source is an array literal of the targets in the other order.
func TestArrayDestructureAssignSwap(t *testing.T) {
	const src = "let a = 1;\nlet b = 2;\n[a, b] = [b, a];\nconsole.log(a + \" \" + b);\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "a, b = b, a") {
		t.Errorf("swap did not lower to a parallel assignment:\n%s", source)
	}
}

// TestArrayDestructureAssignRuns builds and runs a variable-source assignment followed
// by a swap so both the positional reads and the parallel swap are proven.
func TestArrayDestructureAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
let a = 1;
let b = 2;
const pair: number[] = [10, 20];
[a, b] = pair;
console.log(a);
console.log(b);
[a, b] = [b, a];
console.log(a);
console.log(b);
`
	if got, want := runProgramGo(t, src), "10\n20\n20\n10\n"; got != want {
		t.Fatalf("array destructure assignment printed %q, want %q", got, want)
	}
}

// TestArrayDestructureAssignThreeSwap proves a rotation across three targets lowers
// and runs, since the parallel assignment evaluates all sources first.
func TestArrayDestructureAssignThreeSwap(t *testing.T) {
	skipIfShort(t)
	const src = `
let a = 1;
let b = 2;
let c = 3;
[a, b, c] = [c, a, b];
console.log(a);
console.log(b);
console.log(c);
`
	if got, want := runProgramGo(t, src), "3\n1\n2\n"; got != want {
		t.Fatalf("three-way rotation printed %q, want %q", got, want)
	}
}

// TestArrayDestructureAssignMemberTargetRuns proves a member target lowers and runs,
// storing each element into the field it names.
func TestArrayDestructureAssignMemberTargetRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const o = { x: 0, y: 0 };\nconst pair: number[] = [1, 2];\n[o.x, o.y] = pair;\nconsole.log(o.x);\nconsole.log(o.y);\n"
	if got, want := runProgramGo(t, src), "1\n2\n"; got != want {
		t.Fatalf("array member target assignment printed %q, want %q", got, want)
	}
}

// TestArrayDestructureAssignArityMismatchHandsBack proves a literal source with a
// different element count than the targets hands back.
func TestArrayDestructureAssignArityMismatchHandsBack(t *testing.T) {
	const src = "let a = 0;\nlet b = 0;\n[a, b] = [1, 2, 3];\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}

// TestArrayDestructureAssignCallSourceHandsBack proves a call source hands back, since
// reading each element off the result needs a temporary to hold it.
func TestArrayDestructureAssignCallSourceHandsBack(t *testing.T) {
	const src = "function pair(): number[] { return [1, 2]; }\nlet a = 0;\nlet b = 0;\n[a, b] = pair();\nconsole.log(a + b);\n"
	renderProgramHandBack(t, src)
}
