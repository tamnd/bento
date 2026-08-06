package build

import (
	"strings"
	"testing"
)

// TestNewPlainFunctionReachesRenderer pins that a `new` over a value the checker
// types as a plain function does not gate the build at the front door. The checker
// reports 7009 ("'new' expression, whose target lacks a construct signature,
// implicitly has an 'any' type"), a strictness artifact over JavaScript that builds
// a fresh object with the callable as its constructor at run time, so the front door
// tolerates the report and the program reaches the renderer.
//
// The renderer used to hand a plain-function target back; it now lowers it, because a
// function a program applies `new` to is an ES5 constructor and lowers to the runtime
// constructor value that has a real .prototype to link the instance to (ctorfunc.go).
// The tell is the runtime construct in the emitted Go.
func TestNewPlainFunctionReachesRenderer(t *testing.T) {
	src := "function f() {}\nconst x = new f();\nconsole.log(typeof x);\n"
	out, err := compileSource(t, src)
	if err != nil {
		if strings.Contains(err.Error(), "lacks a construct signature") {
			t.Fatalf("the new over a plain function should not gate at the front door, got the checker report: %v", err)
		}
		t.Fatalf("new over a plain function should lower, got: %v", err)
	}
	if !strings.Contains(out, "value.Construct(") {
		t.Fatalf("expected the runtime construct, got:\n%s", out)
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
