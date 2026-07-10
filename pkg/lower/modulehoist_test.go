package lower

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// A module-level binding a top-level function reads hoists to a package var. When
// the initializer is a literal or arithmetic over literals it moves whole to package
// scope. When it is a call or an expression over other module state it cannot run at
// package-init time, so the binding becomes a zero-valued package var and its
// statement stays in main to assign it at its source position, keeping the module
// top-level evaluation order. These tests cover that in-place-assignment hoist.

// TestModuleCallInitHoistsAsAssignment proves a binding whose initializer is a call
// declares a zero-valued package var and runs the call in main, not at package init.
func TestModuleCallInitHoistsAsAssignment(t *testing.T) {
	const src = "function seed(): number { return 7; }\nconst base = seed();\nfunction use(): number { return base + 1; }\nconsole.log(use());\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "var base float64\n") {
		t.Errorf("call-initialized binding did not become a zero-valued package var:\n%s", source)
	}
	if strings.Contains(source, "var base float64 = seed()") {
		t.Errorf("call initializer ran at package init instead of in main:\n%s", source)
	}
	mainIdx := strings.Index(source, "func main()")
	if mainIdx < 0 || !strings.Contains(source[mainIdx:], "base = Seed()") {
		t.Errorf("the call assignment did not stay in main at its source position:\n%s", source)
	}
}

// TestModuleCallInitRuns proves the call-initialized binding carries its settled
// value into the function that reads it.
func TestModuleCallInitRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function seed(): number { return 7; }
const base = seed();
function use(): number { return base + 1; }
console.log(use());
`
	if got, want := runProgramGo(t, src), "8\n"; got != want {
		t.Fatalf("call-initialized module binding printed %q, want %q", got, want)
	}
}

// TestModuleExpressionOverBindingsRuns proves a binding whose initializer is an
// expression over other module bindings settles in source order before the function
// that reads it runs.
func TestModuleExpressionOverBindingsRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
const w = 4;
const h = 3;
const area = w * h;
function f(): number { return area; }
console.log(f());
`
	if got, want := runProgramGo(t, src), "12\n"; got != want {
		t.Fatalf("expression-over-bindings module binding printed %q, want %q", got, want)
	}
}

// TestModuleBindingChainRuns proves a chain of bindings, each reading the one before
// it, settles left to right before any function body runs.
func TestModuleBindingChainRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function seed(): number { return 10; }
const a = seed();
const b = a + 1;
const c = b + 1;
function f(): number { return a + b + c; }
console.log(f());
`
	if got, want := runProgramGo(t, src), "33\n"; got != want {
		t.Fatalf("module binding chain printed %q, want %q", got, want)
	}
}

// TestModuleBindingReadInNestedClosureRuns proves a binding read from inside a
// nested arrow, not just the top-level function body, resolves to the hoisted
// package var.
func TestModuleBindingReadInNestedClosureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `
function compute(): number { return 42; }
const base = compute();
function outer(): number {
  const inner = () => base;
  return inner();
}
console.log(outer());
`
	if got, want := runProgramGo(t, src), "42\n"; got != want {
		t.Fatalf("nested-closure module read printed %q, want %q", got, want)
	}
}

// TestForwardModuleRefDetection locks the in-place-assignment safety guard directly.
// The TypeScript checker already rejects a direct module-binding forward reference as
// a use-before-assignment, so the guard is defensive, but it must still order the
// bindings correctly on its own: a read of an earlier binding is safe, and a read of
// the binding's own slot or a later one is a forward reference the hoist must decline
// rather than assign from a zero-valued package var.
func TestForwardModuleRefDetection(t *testing.T) {
	const src = "var a = 1;\nvar b = a + 1;\nvar c = b + 1;\n"
	prog := compile(t, src)
	entry := entryFile(t, prog)
	r := NewRenderer(prog)
	order := moduleBindingOrder(prog, entry)

	var decls []frontend.Node
	collectKind(prog, []frontend.Node{entry}, frontend.NodeVariableDeclaration, &decls)
	if len(decls) != 3 {
		t.Fatalf("found %d variable declarations, want 3", len(decls))
	}
	sym := func(i int) frontend.Symbol {
		s, ok := prog.SymbolAt(prog.Children(decls[i])[0])
		if !ok {
			t.Fatalf("declaration %d has no symbol", i)
		}
		return s
	}
	a, b, c := sym(0), sym(1), sym(2)
	if !(order[a] < order[b] && order[b] < order[c]) {
		t.Fatalf("ordinals not increasing: a=%d b=%d c=%d", order[a], order[b], order[c])
	}

	cInit := lastChild(prog, decls[2])
	// c reads b, which is declared before c, so at c's own ordinal it is a safe
	// backward reference.
	if r.forwardModuleRef(cInit, order, order[c]) {
		t.Errorf("c reading the earlier b was flagged as a forward reference")
	}
	// The same read checked at b's own ordinal is a read of the current slot, which
	// the guard must treat as a forward reference.
	if !r.forwardModuleRef(cInit, order, order[b]) {
		t.Errorf("reading b at b's own ordinal was not flagged as a forward reference")
	}
}

func lastChild(prog *frontend.Program, n frontend.Node) frontend.Node {
	kids := prog.Children(n)
	return kids[len(kids)-1]
}
