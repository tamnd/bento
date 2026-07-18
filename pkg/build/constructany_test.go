package build

import (
	"strings"
	"testing"
)

// TestNewPlainFunctionReachesRenderer pins that a `new` over a value the checker
// types as a plain function no longer gates the build at the front door. The checker
// reports 7009 ("'new' expression, whose target lacks a construct signature,
// implicitly has an 'any' type"), a strictness artifact over JavaScript that builds
// a fresh object with the callable as its constructor at run time, so the front door
// tolerates the report and the program reaches the renderer. The renderer lowers a
// `new` only for a class or a named built-in constructor, so a plain-function target
// hands back to the engine rather than emitting a wrong construction: the tell is
// that the error is now a lowering hand-back, not the checker's construct-signature
// message.
func TestNewPlainFunctionReachesRenderer(t *testing.T) {
	src := "function f() {}\nconst x = new f();\nconsole.log(typeof x);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("new over a plain function should hand back, not lower")
	}
	if strings.Contains(err.Error(), "lacks a construct signature") {
		t.Fatalf("the new over a plain function should no longer gate at the front door, got the checker report: %v", err)
	}
	if !strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("expected a lowering hand-back to the engine, got: %v", err)
	}
}

// TestNewUserClassStillLowers pins that admitting 7009 did not disturb the user-class
// path: a class carries a construct signature, so `new C()` never draws 7009 and
// still lowers to the class's generated constructor rather than handing back.
func TestNewUserClassStillLowers(t *testing.T) {
	src := "class C {\n  x = 1;\n}\nconsole.log(new C().x);\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("new of a user class should still lower, got: %v", err)
	}
	if !strings.Contains(out, "NewC(") {
		t.Fatalf("expected the class constructor call to lower, got:\n%s", out)
	}
}

// TestGenuineErrorStillGatesAfterConstructAnyTolerance pins that admitting the
// construct-signature report did not open the gate to unrelated errors: an undeclared
// name is still a hard front-door failure, so only the construct-any family is
// tolerated.
func TestGenuineErrorStillGatesAfterConstructAnyTolerance(t *testing.T) {
	src := "console.log(nope);\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("an undeclared name should still gate the build")
	}
	if !strings.Contains(err.Error(), "Cannot find name") {
		t.Fatalf("expected the undeclared-name error, got: %v", err)
	}
}
