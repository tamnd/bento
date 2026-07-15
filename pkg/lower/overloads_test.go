package lower

import (
	"strings"
	"testing"
)

// A function overload set is several declarations under one symbol: bodyless signatures
// and one implementation with a body. Only the implementation runs, so the set lowers to
// one Go func for the body and every call routes through it. This slice claims the
// representation-safe subset: an implementation whose parameters are all required and all
// dynamic and whose return is dynamic or void, so a call boxes each argument and reads
// the result back as a box. These pin the lowering, the boxed-result contract, and the
// handbacks so the overload path never ships Go the toolchain rejects.

const overloadDecl = "function f(x: number): number;\n" +
	"function f(x: string): string;\n" +
	"function f(x: any): any { return x; }\n"

// A call that matches an overload runs the implementation with the argument boxed, and
// the boxed result prints through the value model, so f(3) and f("hi") each echo their
// argument the way the identity implementation returns it.
func TestOverloadMatchedCallRuns(t *testing.T) {
	skipIfShort(t)
	src := overloadDecl + "console.log(f(3));\nconsole.log(f(\"hi\"));\n"
	if got := runProgramGoTolerant(t, src); got != "3\nhi\n" {
		t.Fatalf("overloaded call = %q, want 3 then hi", got)
	}
}

// The boxed result flows into a number slot through the dynamic boundary, so a call
// stored in a typed const and used in arithmetic coerces with ToNumber and the program
// prints the computed value rather than mistyping the box as a float64.
func TestOverloadResultCoercesToNumber(t *testing.T) {
	skipIfShort(t)
	src := overloadDecl + "const n: number = f(3);\nconsole.log(n + 1);\n"
	if got := runProgramGoTolerant(t, src); got != "4\n" {
		t.Fatalf("overloaded result in arithmetic = %q, want 4", got)
	}
}

// A call whose argument matches no overload signature is a 2769 the front door admits.
// It lowers to the same boxed dispatch and runs the implementation with the argument it
// was given, the run-time behavior JavaScript has for a call no overload accepts, so
// f(true) prints true rather than handing the unit back.
func TestOverloadUnmatchedCallRuns(t *testing.T) {
	skipIfShort(t)
	src := overloadDecl + "console.log(f(true));\n"
	if got := runProgramGoTolerant(t, src); got != "true\n" {
		t.Fatalf("unmatched overloaded call = %q, want true", got)
	}
}

// An overload set whose implementation is not all-dynamic (a concrete or union
// parameter) is a later slice: the call site could not box against the impl's Go
// parameter type, so the whole unit hands back rather than emit a partial function.
func TestOverloadConcreteImplHandsBack(t *testing.T) {
	src := "function f(x: number): number;\n" +
		"function f(x: string): string;\n" +
		"function f(x: number | string): number | string { return x; }\n" +
		"console.log(f(3));\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "all-dynamic") {
		t.Fatalf("concrete-impl overload reason = %q, want the all-dynamic handback", reason)
	}
}

// A plain non-overloaded function keeps its ordinary lowering: one declaration, no
// signature siblings, so nothing about the overload path touches it and a direct call
// stays a static Go call.
func TestOverloadPlainFunctionUnaffected(t *testing.T) {
	skipIfShort(t)
	src := "function f(x: any): any { return x; }\nconsole.log(f(3));\n"
	if got := runProgramGoTolerant(t, src); got != "3\n" {
		t.Fatalf("plain function = %q, want 3", got)
	}
}
