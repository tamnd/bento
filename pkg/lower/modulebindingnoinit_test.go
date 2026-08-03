package lower

import (
	"strings"
	"testing"
)

// A module-level binding with no initializer is a name that exists from the top of
// the module and holds undefined until something assigns it. Node's test/common
// declares `let catchWarning;` and fills it from a function further down, which is
// an ordinary thing for a module to do.
//
// The hoist that moves a module binding a function reads to package scope used to
// require an initializer, because both of its forms needed an expression: the
// package-init form puts it in the var spec, and the in-place form assigns it in
// main at the source position. A binding with no initializer has neither, and the
// answer is that it needs neither. Go zero-initializes a package var, and that is
// precisely what undefined-until-assigned means here.

// TestABareLetAFunctionReadsHoistsToAPackageVar pins the shape. The declaration
// reaches package scope with a type and no value.
func TestABareLetAFunctionReadsHoistsToAPackageVar(t *testing.T) {
	src := `let pending: number;
function record(n: number): void { pending = n; }
function readBack(): number { return pending; }
record(1);
console.log(readBack());`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var pending float64") {
		t.Fatalf("the uninitialized binding did not reach package scope:\n%s", out)
	}
}

// TestABareLetContributesNoStatementToMain pins the other half. There is no value
// to write at the source position, so the statement lowers to nothing rather than to
// a second declaration that would shadow the package var the functions read.
func TestABareLetContributesNoStatementToMain(t *testing.T) {
	src := `let pending: number;
function record(n: number): void { pending = n; }
function readBack(): number { return pending; }
record(1);
console.log(readBack());`
	out := renderProgram(t, src)
	if strings.Contains(out, "pending :=") {
		t.Errorf("the statement declared a main local that shadows the package var:\n%s", out)
	}
	if strings.Count(out, "var pending") != 1 {
		t.Errorf("the binding was declared more than once:\n%s", out)
	}
}

// TestABareLetKeepsItsAssignmentsInOrder builds and runs it. The package var starts
// at its zero value, each assignment lands, and the reader sees the settled value,
// which is what a render test cannot show.
func TestABareLetKeepsItsAssignmentsInOrder(t *testing.T) {
	skipIfShort(t)
	src := `
let pending: number;
function record(n: number): void { pending = n; }
function readBack(): number { return pending; }
record(7);
console.log(readBack());
record(12);
console.log(readBack());
`
	got := runProgramGo(t, src)
	want := "7\n12\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
