package build

import (
	"strings"
	"testing"
)

// TestNonIterableForOfReachesRenderer pins that a for...of over a value the checker
// types as the empty object `{}` no longer gates the build at the front door. The
// checker reports 2488 ("Type '{}' must have a '[Symbol.iterator]()' method"), a
// strictness artifact over JavaScript that walks the value at run time, so the front
// door tolerates the report and the program reaches the renderer. The value carries
// no static shape to iterate, so the renderer hands it back to the engine rather than
// emitting a wrong iteration: the tell is that the error is now a lowering hand-back,
// not the checker's iterator message.
func TestNonIterableForOfReachesRenderer(t *testing.T) {
	src := "function f(x: {}): void {\n  for (const e of x as {}) { console.log(e); }\n}\nf([1, 2, 3] as {});\n"
	_, err := compileSource(t, src)
	if err == nil {
		return // A full lowering is also acceptable; the point is the front door admits it.
	}
	if strings.Contains(err.Error(), "[Symbol.iterator]") {
		t.Fatalf("the non-iterable for...of should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestNonIterableSpreadReachesRenderer pins the same tolerance for an array-literal
// spread of a `{}`-typed value, `[...(x as {})]`, which draws the same 2488 and now
// reaches the renderer instead of gating.
func TestNonIterableSpreadReachesRenderer(t *testing.T) {
	src := "function f(x: {}): number {\n  const a = [...(x as {})];\n  return a.length;\n}\nconsole.log(f({}));\n"
	_, err := compileSource(t, src)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "[Symbol.iterator]") {
		t.Fatalf("the non-iterable spread should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestVoidUnionForOfReachesRenderer pins the tolerance for the other named shape, a
// for...of over a value the checker types as a union carrying void, `void | number[]`,
// the inferred return of a function that returns an array on one path and falls off
// the end on another. It draws 2488 the same way and now reaches the renderer.
func TestVoidUnionForOfReachesRenderer(t *testing.T) {
	src := "function g(): void | number[] { return [1, 2]; }\nfor (const e of g()) { console.log(e); }\n"
	_, err := compileSource(t, src)
	if err == nil {
		return
	}
	if strings.Contains(err.Error(), "[Symbol.iterator]") {
		t.Fatalf("the void-union for...of should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestGenuineErrorStillGatesAfterIterableTolerance pins that admitting the iterability
// report did not open the gate to unrelated errors: an undeclared name is still a hard
// front-door failure, so only the iterator-method family is tolerated.
func TestGenuineErrorStillGatesAfterIterableTolerance(t *testing.T) {
	src := "console.log(nope);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("an undeclared name should still gate the build")
	}
	if !strings.Contains(err.Error(), "Cannot find name") {
		t.Fatalf("expected the undeclared-name error, got: %v", err)
	}
}
