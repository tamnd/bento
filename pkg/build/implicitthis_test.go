package build

import (
	"strings"
	"testing"
)

// TestFreeThisHelperReachesRenderer pins that a `this` read inside a plain function
// that carries no `this` annotation no longer gates the build at the front door. The
// checker reports 2683 ("'this' implicitly has type 'any' because it does not have a
// type annotation"), a strictness artifact over JavaScript that binds `this` at the
// call site, so the front door tolerates the report and the program reaches the
// renderer. The renderer lowers `this` only inside a class body it is currently
// lowering, so a free `this` finds no receiver and hands back to the engine rather
// than emitting a wrong reference: the tell is that the error is now a lowering
// hand-back, not the checker's implicit-this message.
func TestFreeThisHelperReachesRenderer(t *testing.T) {
	src := "function f(): string {\n  return typeof this;\n}\nconsole.log(f());\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a free this with no receiver should hand back, not lower")
	}
	if strings.Contains(err.Error(), "implicitly has type 'any'") {
		t.Fatalf("the free this should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestNestedFunctionThisReachesRenderer pins the same tolerance for a `this` read
// inside a plain function nested in a method. The nested function has no `this` of
// its own, so the checker draws 2683 there too, and the renderer must not read the
// enclosing method's receiver for it: the nested function declaration statement is
// itself a hand-back before its `this` is ever reached, so the program routes to the
// engine rather than binding the wrong receiver.
func TestNestedFunctionThisReachesRenderer(t *testing.T) {
	src := "class C {\n  m(): void {\n    function g(): string { return typeof this; }\n    console.log(g());\n  }\n}\nnew C().m();\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a nested free this should hand back, not lower")
	}
	if strings.Contains(err.Error(), "implicitly has type 'any'") {
		t.Fatalf("the nested free this should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestGenuineErrorStillGatesAfterImplicitThisTolerance pins that admitting the
// implicit-this report did not open the gate to unrelated errors: an undeclared name
// is still a hard front-door failure, so only the implicit-this family is tolerated.
func TestGenuineErrorStillGatesAfterImplicitThisTolerance(t *testing.T) {
	src := "console.log(nope);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("an undeclared name should still gate the build")
	}
	if !strings.Contains(err.Error(), "Cannot find name") {
		t.Fatalf("expected the undeclared-name error, got: %v", err)
	}
}
