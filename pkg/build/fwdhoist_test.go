package build

import (
	"strings"
	"testing"
)

// TestCallableForwardHoistDeclaresBeforeCapture pins the fix for a callable
// object whose name an earlier statement captures inside a closure, the shape the
// test262 assert prelude writes: assert.compareArray = function () { ...
// compareArray ... } sits above const compareArray = function () { ... }. Without
// the hoist the closure would close over a Go variable declared below it, which
// does not compile. The binding's pointer must be declared at the scope top and
// its own site must lower to a plain assignment, so the emitted Go declares
// `var compare *Comparer` before the capturing closure and assigns
// `compare = &Comparer{}` at the const's original position.
func TestCallableForwardHoistDeclaresBeforeCapture(t *testing.T) {
	src := "interface Asserter {\n  (ok: boolean): void;\n  cmp: (a: number[], b: number[]) => void;\n}\n" +
		"interface Comparer {\n  (a: number[], b: number[]): boolean;\n  format: (a: number[]) => string;\n}\n" +
		"const check = function (ok: boolean): void { if (!ok) console.log(\"fail\"); } as Asserter;\n" +
		"check.cmp = function (a: number[], b: number[]): void { const ok = compare(a, b); check(ok); console.log(compare.format(a)); };\n" +
		"const compare = function (a: number[], b: number[]): boolean { return a.length === b.length; } as Comparer;\n" +
		"compare.format = function (a: number[]): string { return \"[\" + a.length + \"]\"; };\n" +
		"check.cmp([1, 2], [3, 4]);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a forward-captured callable object should lower, got: %v", err)
	}
	hoist := strings.Index(out, "var compare *Comparer")
	capture := strings.Index(out, "check.Cmp = func")
	assign := strings.Index(out, "compare = &Comparer{}")
	if hoist < 0 {
		t.Fatalf("expected the pointer to hoist as `var compare *Comparer`, got:\n%s", out)
	}
	if assign < 0 {
		t.Fatalf("expected the const site to lower to `compare = &Comparer{}`, got:\n%s", out)
	}
	if capture < 0 || hoist > capture {
		t.Fatalf("expected the hoisted declaration above the capturing closure, got:\n%s", out)
	}
}

// TestBinaryEvalOrderHazardHandsBack pins that `x + (x = 1)` hands back rather
// than lowering to a Go `x + f()` that may read the left x after the right
// operand's assignment closure runs. JavaScript evaluates the left operand first,
// so the sum is 1; Go leaves the plain left read unordered against the right
// call, so the emitted form can give 2. The worst case must stay handback until a
// later slice sequences the operands through a temp, never a miscompiled sum.
func TestBinaryEvalOrderHazardHandsBack(t *testing.T) {
	src := "var x = 0;\nif (x + (x = 1) !== 1) { throw new Error(\"bad\"); }\n"
	if _, err := compileSource(t, src); err == nil {
		t.Fatal("x + (x = 1) should hand back for operand sequencing, but it lowered")
	}
}

// TestBinaryEvalOrderAssignLeftStillLowers pins that the reverse arrangement is
// not swept up by the hazard guard: `(x = 1) + x` keeps its order under Go, which
// evaluates the left operand's assignment closure before the right operand's plain
// read, so it still lowers.
func TestBinaryEvalOrderAssignLeftStillLowers(t *testing.T) {
	src := "var x = 0;\nif ((x = 1) + x !== 2) { throw new Error(\"bad\"); }\n"
	if _, err := compileSource(t, src); err != nil {
		t.Fatalf("(x = 1) + x should still lower, got: %v", err)
	}
}
