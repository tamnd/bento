package lower

import (
	"strings"
	"testing"
)

// The front door admits code 2345 (an argument not assignable to its parameter) so a
// call the checker rejects on type grounds still reaches the renderer. The renderer
// then either lands the argument, when it and the parameter lower to the same Go type,
// or hands the unit back, when they do not or when an unguarded builtin path would emit
// Go that drops a value into a slot of another type. These pin both outcomes so the
// tolerance never ships broken Go.

// A numeric-literal-union argument passed where a number is wanted is a 2345, since the
// checker widens the union to number only under assignment, not at a call. Both lower to
// float64, so the representation guard lands the value and the call runs.
func TestAssignabilityNumericUnionArgRuns(t *testing.T) {
	skipIfShort(t)
	src := "function g(n: number): number { return n + 1; }\nconst k: 1 | 2 = 2;\nconsole.log(g(k));\n"
	if got := runProgramGoTolerant(t, src); got != "3\n" {
		t.Fatalf("g(k) with k:1|2 = %q, want 3", got)
	}
}

// A number passed where a string parameter is declared is a 2345 whose two sides lower
// to different Go types, so the argument bridge hands the call back rather than land a
// float64 in a value.BStr slot.
func TestAssignabilityNumberForStringArgHandsBack(t *testing.T) {
	src := "function f(s: string): string { return s; }\nconsole.log(f(5));\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "not assignable") {
		t.Fatalf("number-for-string arg reason = %q, want a not-assignable handback", reason)
	}
}

// The constructor bridge carries the same guard: a number passed to a string-typed
// constructor parameter hands back rather than emit NewC over a float64.
func TestAssignabilityNumberForStringCtorArgHandsBack(t *testing.T) {
	src := "class C { s: string; constructor(s: string) { this.s = s; } }\nconsole.log(new C(5).s);\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "not assignable") {
		t.Fatalf("number-for-string ctor arg reason = %q, want a not-assignable handback", reason)
	}
}

// A value dropped into a builtin element slot, a string pushed onto a number array, is a
// 2345 no guarded bridge reaches, since push lowers its argument straight into the Go
// slice. The end-of-render reconciliation catches the unseen site and hands the unit
// back rather than ship the broken Go the push path would emit.
func TestAssignabilityBuiltinElementSlotHandsBack(t *testing.T) {
	src := "const a = [1, 2, 3];\na.push(\"x\");\nconsole.log(a.length);\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no representation guard") {
		t.Fatalf("builtin element-slot reason = %q, want the reconciliation handback", reason)
	}
}

// A callback whose parameter type mismatches a builtin higher-order method's element is
// a 2345 on the arrow itself, again a site no guarded bridge reaches, so the
// reconciliation hands the unit back rather than pass a func(value.BStr) where a
// func(float64) is wanted.
func TestAssignabilityBuiltinCallbackHandsBack(t *testing.T) {
	src := "[1, 2, 3].forEach((x: string) => { console.log(x); });\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no representation guard") {
		t.Fatalf("builtin callback reason = %q, want the reconciliation handback", reason)
	}
}

// A user call whose argument wraps a builtin call that itself carries the 2345 must not
// be mistaken for a guarded site: the 2345 sits on the inner builtin argument, not on
// the outer argument the user call bridges. The reconciliation still sees the inner site
// unhandled and hands the unit back.
func TestAssignabilityNestedBuiltin2345HandsBack(t *testing.T) {
	src := "function g(n: number): number { return n; }\nconsole.log(g([1, 2, 3].indexOf(\"x\")));\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no representation guard") {
		t.Fatalf("nested builtin 2345 reason = %q, want the reconciliation handback", reason)
	}
}

// A correctly typed builtin callback carries no 2345, so nothing about the tolerance
// touches it: the reconciliation set is empty and the program lowers and runs as before.
func TestAssignabilityWellTypedCallbackStillRuns(t *testing.T) {
	skipIfShort(t)
	src := "const a = [1, 2, 3];\nlet s = 0;\na.forEach((x: number) => { s += x; });\nconsole.log(s);\n"
	if got := runProgramGoTolerant(t, src); got != "6\n" {
		t.Fatalf("well-typed forEach sum = %q, want 6", got)
	}
}
