package lower

import (
	"errors"
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// bodyStatements returns the statement list of the first top-level function declaration
// named fn, the body slice definiteLocalsOf analyzes for one function.
func bodyStatements(t *testing.T, prog *frontend.Program, entry frontend.Node, fn string) []frontend.Node {
	t.Helper()
	var decls []frontend.Node
	collectKind(prog, []frontend.Node{entry}, frontend.NodeFunctionDeclaration, &decls)
	for _, d := range decls {
		kids := prog.Children(d)
		if len(kids) == 0 || kids[0].Kind() != frontend.NodeIdentifier || prog.Text(kids[0]) != fn {
			continue
		}
		for _, c := range kids {
			if c.Kind() == frontend.NodeBlock {
				return prog.Children(c)
			}
		}
	}
	t.Fatalf("no function %q with a block body found", fn)
	return nil
}

// A binding declared with a non-optional static type and no initializer lowers to a
// plain Go var of that type when the checker has proven it assigned before every read
// and no closure captures it. These tests cover that lowering and its two guards: the
// closure-capture exclusion that keeps the unsound shape handing back, and the blank
// that keeps an assigned-but-unread binding compiling.

// TestTypedNoInitBindingRuns proves a no-initializer typed local assigned on both arms
// of a branch and then read carries the assigned value, not the Go zero.
func TestTypedNoInitBindingRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function pick(b: boolean): number {
  let x: number;
  if (b) { x = 5; } else { x = 9; }
  return x;
}
console.log(pick(true));
console.log(pick(false));
`
	if got, want := runProgramGo(t, src), "5\n9\n"; got != want {
		t.Fatalf("typed no-init binding printed %q, want %q", got, want)
	}
}

// TestTypedNoInitBindingDeclaresVar proves the binding renders as a plain typed var
// with no initializer, the value.Opt and value.Value no-init branches aside.
func TestTypedNoInitBindingDeclaresVar(t *testing.T) {
	const src = "function f(b: boolean): number {\n  let x: number;\n  if (b) { x = 1; } else { x = 2; }\n  return x;\n}\nconsole.log(f(true));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var x float64\n") {
		t.Errorf("no-init typed binding did not render as a plain typed var:\n%s", source)
	}
	if strings.Contains(source, "var x float64 =") {
		t.Errorf("no-init typed binding gained an initializer it never had:\n%s", source)
	}
}

// TestTypedNoInitUnusedCompiles proves a no-init binding assigned but never read still
// declares and compiles: the blank the var statement appends marks it used in Go.
func TestTypedNoInitUnusedCompiles(t *testing.T) {
	skipIfShort(t)
	const src = `
function f(): void {
  let x: number;
  x = 42;
}
f();
console.log("ok");
`
	if got, want := runProgramGo(t, src), "ok\n"; got != want {
		t.Fatalf("assigned-but-unused no-init binding printed %q, want %q", got, want)
	}
}

// TestTypedNoInitClosureCaptureHandsBack pins the soundness guard. A closure that reads
// a no-init binding may run while the binding is still unassigned, a read the checker's
// definite-assignment analysis does not police, so a Go zero would masquerade as the
// undefined the closure should see. The binding must keep handing back, not lower to a
// zero-valued var.
func TestTypedNoInitClosureCaptureHandsBack(t *testing.T) {
	const src = "function f(): number {\n  let x: number;\n  const g = () => x;\n  const r = g();\n  x = 5;\n  return r;\n}\nconsole.log(f());\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "no initializer") {
		t.Errorf("hand-back reason = %q, want it to mention a no-initializer binding", nyl.Reason)
	}
}

// TestDefiniteLocalsExcludesCaptured locks the analysis directly: a no-init typed local
// is a candidate on its own but drops out once a nested closure names it.
func TestDefiniteLocalsExcludesCaptured(t *testing.T) {
	free := "function f(b: boolean): number { let x: number; if (b) { x = 1; } else { x = 2; } return x; }\n"
	prog := compile(t, free)
	entry := entryFile(t, prog)
	r := NewRenderer(prog)
	body := bodyStatements(t, prog, entry, "f")
	set := r.definiteLocalsOf(body)
	if !set["x"] {
		t.Errorf("a no-init typed local read only in flow was not in the definite set: %v", set)
	}

	captured := "function f(): number { let x: number; const g = () => x; x = 3; return g(); }\n"
	prog2 := compile(t, captured)
	entry2 := entryFile(t, prog2)
	r2 := NewRenderer(prog2)
	body2 := bodyStatements(t, prog2, entry2, "f")
	set2 := r2.definiteLocalsOf(body2)
	if set2["x"] {
		t.Errorf("a closure-captured no-init local was left in the definite set: %v", set2)
	}
}

// A closure-captured no-init typed local rejoins the plain-var set when it is assigned
// by unconditional top-level code before any capturing closure is defined: the closure
// then cannot run before the slot holds a real value. These tests cover that admission,
// prove the by-reference capture reads the live value, and pin the two guards that keep
// the unsound shapes handing back.

// TestCaptureSafeUninitLocalInSet locks the analysis: a captured no-init local assigned
// before its capturing closure is defined is admitted, while the same local captured
// before its assignment is not.
func TestCaptureSafeUninitLocalInSet(t *testing.T) {
	safe := "function f(): number { let x: number; x = 3; const g = () => x; return g(); }\n"
	prog := compile(t, safe)
	r := NewRenderer(prog)
	body := bodyStatements(t, prog, entryFile(t, prog), "f")
	set := r.definiteLocalsOf(body)
	if !set["x"] {
		t.Errorf("a captured local assigned before its closure was not admitted: %v", set)
	}
}

// TestCaptureSafeUninitLocalDeclaresVar proves the admitted local renders as a plain
// typed var and does not hand back.
func TestCaptureSafeUninitLocalDeclaresVar(t *testing.T) {
	const src = "function f(): number {\n  let x: number;\n  x = 3;\n  const g = () => x;\n  return g();\n}\nconsole.log(f());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var x float64\n") {
		t.Errorf("a capture-safe no-init local did not render as a plain typed var:\n%s", source)
	}
}

// TestCaptureSafeUninitLocalRuns proves the by-reference capture reads the live value,
// including a reassignment made after the closure is defined.
func TestCaptureSafeUninitLocalRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function first(): number {
  let x: number;
  x = 3;
  const g = () => x;
  return g();
}
function latest(): number {
  let x: number;
  x = 1;
  const g = () => x;
  x = 7;
  return g();
}
console.log(first());
console.log(latest());
`
	if got, want := runProgramGo(t, src), "3\n7\n"; got != want {
		t.Fatalf("capture-safe no-init local printed %q, want %q", got, want)
	}
}

// TestCaptureBeforeAssignHandsBack pins the soundness guard from the other side: a
// closure defined before the first assignment can run while the slot is still
// unassigned, so the local must keep handing back rather than admit a zero-valued var.
func TestCaptureBeforeAssignHandsBack(t *testing.T) {
	const src = "function f(): number {\n  let x: number;\n  const g = () => x;\n  x = 5;\n  return g();\n}\nconsole.log(f());\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "no initializer") {
		t.Errorf("hand-back reason = %q, want it to mention a no-initializer binding", nyl.Reason)
	}
}

// TestCaptureSafeAssignWithClosureRhsHandsBack pins the conservative right-hand-side
// guard: an assignment whose right side itself holds a function could capture the
// local, so the proof declines it and the local keeps handing back.
func TestCaptureSafeAssignWithClosureRhsHandsBack(t *testing.T) {
	const src = "function f(): number {\n  let x: number;\n  x = (() => 4)();\n  const g = () => x;\n  return g();\n}\nconsole.log(f());\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	body := bodyStatements(t, prog, entryFile(t, prog), "f")
	set := r.definiteLocalsOf(body)
	if set["x"] {
		t.Errorf("a captured local assigned from a closure-bearing right side was admitted: %v", set)
	}
}
