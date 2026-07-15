package lower

import (
	"strings"
	"testing"
)

// The front door admits code 2322 (a value not assignable to the slot it initializes,
// is assigned to, or is returned into) so a binding the checker rejects on type grounds
// still reaches the renderer. The binding bridge then either lands the value, when it and
// the slot lower to the same Go type, or hands the unit back, when they do not or when an
// unguarded construct (an array element, an object property, a return) reaches the site
// with no representation guard. These pin both outcomes so the tolerance never ships
// broken Go. They are the initializer/assignment analog of the 2345 argument pins.

// A literal-typed binding initialized with another literal of the same primitive is a
// 2322: the checker rejects 1 as a 0. Both lower to float64, so the representation guard
// lands the value and the program runs, printing the value the initializer actually holds.
func TestAssignability2322SameReprInitRuns(t *testing.T) {
	skipIfShort(t)
	src := "const n: 0 = 1;\nconsole.log(n);\n"
	if got := runProgramGoTolerant(t, src); got != "1\n" {
		t.Fatalf("const n: 0 = 1 = %q, want 1", got)
	}
}

// A numeric-literal-union slot initialized outside the union is the same shape: 3 is not a
// 1 | 2, but every side lowers to float64, so the guard lands it and the program prints 3.
func TestAssignability2322LiteralUnionInitRuns(t *testing.T) {
	skipIfShort(t)
	src := "const k: 1 | 2 = 3;\nconsole.log(k);\n"
	if got := runProgramGoTolerant(t, src); got != "3\n" {
		t.Fatalf("const k: 1 | 2 = 3 = %q, want 3", got)
	}
}

// A number initializing a string binding is a 2322 whose two sides lower to different Go
// types, so the binding bridge hands the unit back rather than land a float64 in a
// value.BStr slot. The guard, not the reconciliation, catches it: the reason names the
// mismatch, not an unguarded construct.
func TestAssignability2322NumberForStringInitHandsBack(t *testing.T) {
	src := "const s: string = 5;\nconsole.log(s);\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "not assignable") {
		t.Fatalf("number-for-string init reason = %q, want a not-assignable handback", reason)
	}
	if strings.Contains(reason, "no representation guard") {
		t.Fatalf("number-for-string init reason = %q, want the guarded handback, not the reconciliation", reason)
	}
}

// A number assigned to a string binding carries the same 2322 on the assignment target,
// so the binding bridge hands the unit back the same way an initializer does.
func TestAssignability2322NumberForStringAssignHandsBack(t *testing.T) {
	src := "let s: string = \"a\";\ns = 5;\nconsole.log(s);\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "not assignable") {
		t.Fatalf("number-for-string assign reason = %q, want a not-assignable handback", reason)
	}
}

// A number dropped into a string-array element is a 2322 no guarded bridge reaches, since
// the array literal lowers its element straight into the Go slice. The end-of-render
// reconciliation catches the unseen site and hands the unit back rather than ship the
// broken Go the element path would emit.
func TestAssignability2322ArrayElementHandsBack(t *testing.T) {
	src := "const a: string[] = [1];\nconsole.log(a.length);\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no representation guard") {
		t.Fatalf("array-element 2322 reason = %q, want the reconciliation handback", reason)
	}
}

// A number returned where a string is declared is a 2322 on the return, again a site the
// binding bridge does not reach, so the reconciliation hands the unit back.
func TestAssignability2322ReturnHandsBack(t *testing.T) {
	src := "function f(): string { return 5; }\nconsole.log(f());\n"
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no representation guard") {
		t.Fatalf("return 2322 reason = %q, want the reconciliation handback", reason)
	}
}

// A correctly typed binding carries no 2322, so nothing about the tolerance touches it:
// the reconciliation set is empty and the program lowers and runs as before.
func TestAssignability2322WellTypedStillRuns(t *testing.T) {
	skipIfShort(t)
	src := "const n: number = 3;\nconsole.log(n + 1);\n"
	if got := runProgramGoTolerant(t, src); got != "4\n" {
		t.Fatalf("well-typed init = %q, want 4", got)
	}
}
