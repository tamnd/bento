package build

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/lower"
)

// TestBlockScopeFnVarRedeclarationRejected pins the ES2015 Block early error bento
// catches ahead of lowering: a block-scoped function declaration whose name also
// occurs as a var in the same block. TypeScript permits the function/var merge and
// reports nothing, so without the check the lowerer emits `f := func(){}; var f`
// into one Go block and the toolchain rejects it. node throws SyntaxError at parse
// time, so bento must return a real build error, not lower the program.
func TestBlockScopeFnVarRedeclarationRejected(t *testing.T) {
	src := "function x() {\n  { function f() {}; var f; }\n}\nx();\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a block-scoped function declaration colliding with a same-block var should be rejected")
	}
	if !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("expected the block early-error rejection, got: %v", err)
	}
	// The rejection must be a real early error, not a lowering hand-back: a
	// parse-phase negative test scores a *NotYetLowerable as a handback, not a pass.
	var nyl *lower.NotYetLowerable
	if strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("the rejection must be a real build error, not a handback: %v", err)
	}
	_ = nyl
}

// TestCatchParamLexicalRedeclarationRejected pins the catch early error from spec
// 13.15.1: a catch parameter name that also occurs in the LexicallyDeclaredNames of
// the catch block, a directly nested function declaration here, is a SyntaxError.
// TypeScript reports nothing, so without the check the lowerer emits a Go block that
// binds the catch parameter and then redeclares it as a function and the toolchain
// rejects it. node throws at parse time, so this must be a real build error, not a
// hand-back, or the parse-phase negative test scores as a handback rather than a pass.
func TestCatchParamLexicalRedeclarationRejected(t *testing.T) {
	src := "function f() {\n  try {\n  } catch (e) {\n    function e() {}\n  }\n}\nf();\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a catch parameter redeclared by a catch-block function should be rejected")
	}
	if !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("expected the catch early-error rejection, got: %v", err)
	}
	if strings.Contains(err.Error(), "not yet lowerable") {
		t.Fatalf("the rejection must be a real build error, not a handback: %v", err)
	}
}

// TestOptionalCatchBindingStillLowers pins that an optional catch binding, a catch
// clause with no parameter, does not trip the catch early-error check. There are no
// BoundNames to collide, so the check must read past the missing parameter node
// without dereferencing it and let the program lower.
func TestOptionalCatchBindingStillLowers(t *testing.T) {
	for _, src := range []string{
		"try {} catch {}\n",
		"try { throw 1; } catch { }\n",
		"function f() { try {} catch { function g() {} } } f();\n",
	} {
		if _, err := compileSource(t, src); err != nil {
			t.Fatalf("optional catch binding should lower, got: %v (src %q)", err, src)
		}
	}
}

// TestBlockScopeFnVarRedeclarationNestedBlockRejected pins the same collision when
// the var sits in a nested plain block inside the function's block: a var is
// function-scoped, so the inner block's `var f` is one of the outer block's
// var-declared names and still collides with the outer block's function `f`.
func TestBlockScopeFnVarRedeclarationNestedBlockRejected(t *testing.T) {
	src := "function g() {\n  { function f() {} { var f; } }\n}\ng();\n"
	_, err := compileSource(t, src)
	if err == nil {
		t.Fatal("a block-scoped function colliding with a var in a nested plain block should be rejected")
	}
	if !strings.Contains(err.Error(), "SyntaxError") {
		t.Fatalf("expected the block early-error rejection, got: %v", err)
	}
}

// TestFnVarNestedInInnerFunctionStillLowers pins the precision boundary on the var
// side: a var of the same name declared inside a NESTED function is a different var
// scope, so it does not collide with the outer function declaration and the program
// must still lower.
func TestFnVarNestedInInnerFunctionStillLowers(t *testing.T) {
	src := "function h(){ var f = 1; function g(){ var f = 2; return f; } return g(); } h();\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a var in a nested function is a separate scope and should still lower, got: %v", err)
	}
	if !strings.Contains(out, "func H(") {
		t.Fatalf("expected the program to lower, got:\n%s", out)
	}
}

// TestFnAndUnrelatedVarStillLowers pins that a block function and an unrelated var
// name do not trip the check.
func TestFnAndUnrelatedVarStillLowers(t *testing.T) {
	src := "function h(){ function f(){} return f; } var f2; h();\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a function and an unrelated var name should still lower, got: %v", err)
	}
	if !strings.Contains(out, "func H(") {
		t.Fatalf("expected the program to lower, got:\n%s", out)
	}
}

// TestBlockFunctionNoCollisionStillLowers pins that a block-scoped function with no
// colliding var lowers as before, so the check does not fire on the function
// declaration alone.
func TestBlockFunctionNoCollisionStillLowers(t *testing.T) {
	src := "function h(){ function f(){}; f(); } h();\n"
	out, err := compileSource(t, src)
	if err != nil {
		t.Fatalf("a block function with no colliding var should still lower, got: %v", err)
	}
	if !strings.Contains(out, "func H(") {
		t.Fatalf("expected the program to lower, got:\n%s", out)
	}
}
