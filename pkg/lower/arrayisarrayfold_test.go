package lower

import (
	"strings"
	"testing"
)

// TestArrayIsArrayKeepsOperandReferenced pins that Array.isArray on a statically
// typed operand still evaluates and references that operand. The brand is known at
// compile time, but folding to a bare true dropped the operand, so a binding whose
// only use was the check became declared-and-not-used and the emitted Go did not
// compile (the test/built-ins/Array/fromAsync gobuild fail). The fold now threads
// the operand through value.StaticBool.
func TestArrayIsArrayKeepsOperandReferenced(t *testing.T) {
	const src = `const arr: number[] = [1, 2, 3];
const b: boolean = Array.isArray(arr);
console.log(String(b));`
	out := renderProgram(t, src)
	if !strings.Contains(out, "value.StaticBool(arr, true)") {
		t.Fatalf("Array.isArray on a typed array did not keep the operand live via StaticBool:\n%s", out)
	}
}

// TestArrayIsArrayPreservesSideEffects runs Array.isArray over a side-effecting call
// and a plain value: the call must still run (the brand fold must not drop it), the
// static array answers true, and a non-array answers false, matching Node.
func TestArrayIsArrayPreservesSideEffects(t *testing.T) {
	skipIfShort(t)
	const src = `let called = 0;
function f(): number[] { called = called + 1; return [1]; }
const r1: boolean = Array.isArray(f());
const r2: boolean = Array.isArray(42);
console.log(String(r1));
console.log(String(r2));
console.log(String(called));`
	if got, want := runProgramGo(t, src), "true\nfalse\n1\n"; got != want {
		t.Fatalf("Array.isArray fold printed %q, want %q", got, want)
	}
}
