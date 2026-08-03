package lower

import (
	"strings"
	"testing"
)

// A file of helpers that name each other is the ordinary shape of a JavaScript
// module, and until this slice bento refused all of it. JavaScript hoists a
// function declaration and binds it at the top of its scope, so the order two
// helpers are written in does not matter; Go binds a closure to a local from its
// declaration onward, so the lowering used to hand back the moment a statement
// above a declaration named it.
//
// The distinction that reopens the shape is when the name is read. A read from
// inside another function's body happens when that function is called, which is
// after the whole block has finished binding, so declaring the Go local at the top
// of the block and assigning the closure at the declaration's own position is
// enough. A read that runs while the block runs is a genuine forward call and still
// hands back.
//
// Node's test/common/index.js is the file this was measured against: mustCall names
// _mustCallInner, which is declared sixteen lines below it, and almost every test in
// Node's suite requires that file.

// TestANestedHelperMayNameOneDeclaredBelowIt pins the shape the group exists for.
// The read is inside a function body, so the binding is declared at the top of the
// Go block and assigned where the source declares it.
func TestANestedHelperMayNameOneDeclaredBelowIt(t *testing.T) {
	src := `function outer(n: number): number {
  function first(x: number): number { return second(x) + 1; }
  function second(x: number): number { return x * 2; }
  return first(n);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var second func(x float64) float64") {
		t.Fatalf("the forward-named helper did not get a hoisted declaration:\n%s", out)
	}
	if !strings.Contains(out, "second = func(x float64) float64") {
		t.Fatalf("the closure did not bind by assignment at its own position:\n%s", out)
	}
}

// TestTheHoistedDeclarationSitsAboveEveryStatement pins where the declaration goes.
// The point of hoisting is that the binding exists before anything in the block
// runs, so a var emitted after the closure that captures it would compile to
// nothing useful, and Go would reject it outright.
func TestTheHoistedDeclarationSitsAboveEveryStatement(t *testing.T) {
	src := `function outer(n: number): number {
  function first(x: number): number { return second(x) + 1; }
  function second(x: number): number { return x * 2; }
  return first(n);
}`
	out := renderProgram(t, src)
	decl := strings.Index(out, "var second func")
	first := strings.Index(out, "first := func")
	if decl < 0 || first < 0 {
		t.Fatalf("expected both a hoisted declaration and the capturing closure:\n%s", out)
	}
	if decl > first {
		t.Errorf("the hoisted declaration came after the closure that captures it:\n%s", out)
	}
}

// TestAHelperNamedOnlyBelowKeepsItsShortForm pins that nothing changes for the
// common case. A declaration no earlier statement names needs no hoist, and it
// keeps the plain `name := closure` the lowering already emitted.
func TestAHelperNamedOnlyBelowKeepsItsShortForm(t *testing.T) {
	src := `function outer(n: number): number {
  function step(x: number): number { return x * 2; }
  return step(n);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "step := func(x float64) float64") {
		t.Fatalf("a helper named only below it did not keep the short form:\n%s", out)
	}
	if strings.Contains(out, "var step func") {
		t.Errorf("a helper that needs no hoist got one anyway:\n%s", out)
	}
}

// TestMutualRecursionBetweenTwoNestedHelpers pins the two-way case. Each names the
// other, so one is forward-named and hoisted and the other is not, and the pair has
// to agree on one Go local apiece.
func TestMutualRecursionBetweenTwoNestedHelpers(t *testing.T) {
	src := `function outer(n: number): boolean {
  function isEven(x: number): boolean { return x === 0 ? true : isOdd(x - 1); }
  function isOdd(x: number): boolean { return x === 0 ? false : isEven(x - 1); }
  return isEven(n);
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var isOdd func(x float64) bool") {
		t.Fatalf("the forward-named half of the pair was not hoisted:\n%s", out)
	}
	if strings.Count(out, "isOdd = func") != 1 || strings.Count(out, "isEven") < 3 {
		t.Errorf("the mutual pair did not bind one local apiece:\n%s", out)
	}
}

// TestACallThatRunsBeforeTheDeclarationBindsAheadOfIt pins the moving half. The
// call sits in statement position above the declaration, so it runs while the block
// runs and a binding at the declaration's own position would still be nil. The
// closure binds ahead of the call instead, which is where the source's hoisting
// already had it.
func TestACallThatRunsBeforeTheDeclarationBindsAheadOfIt(t *testing.T) {
	src := `function outer(n: number): number {
  const first = pick(n);
  function pick(x: number): number { return x * 2; }
  return first;
}`
	out := renderProgram(t, src)
	bind := strings.Index(out, "pick = func")
	call := strings.Index(out, "first := pick(")
	if bind < 0 || call < 0 {
		t.Fatalf("expected the closure to bind and the call to stay:\n%s", out)
	}
	if bind > call {
		t.Errorf("the closure bound after the call that reads it:\n%s", out)
	}
}

// TestAValueReadThatRunsBeforeTheDeclarationBindsAheadOfIt pins that the move is
// about when the name is read, not about it being a call. Passing the helper as a
// value above its declaration reads the binding then and there, and the closure has
// to be bound by then.
func TestAValueReadThatRunsBeforeTheDeclarationBindsAheadOfIt(t *testing.T) {
	src := `function outer(n: number): number {
  const chosen = pick;
  function pick(x: number): number { return x * 2; }
  return chosen(n);
}`
	out := renderProgram(t, src)
	bind := strings.Index(out, "pick = func")
	read := strings.Index(out, "chosen := pick")
	if bind < 0 || read < 0 {
		t.Fatalf("expected the closure to bind and the value read to stay:\n%s", out)
	}
	if bind > read {
		t.Errorf("the closure bound after the value read:\n%s", out)
	}
}

// TestAnImmediatelyInvokedFunctionCountsAsRunningNow pins that the callee of a call
// is not treated as a body that runs later. An IIFE above the declaration runs
// during the block, so the helper it names has to be bound ahead of the IIFE and not
// at its own position below it.
func TestAnImmediatelyInvokedFunctionCountsAsRunningNow(t *testing.T) {
	src := `function outer(n: number): number {
  const first = (function (): number { return pick(n); })();
  function pick(x: number): number { return x * 2; }
  return first;
}`
	out := renderProgram(t, src)
	bind := strings.Index(out, "pick = func")
	iife := strings.Index(out, "first := (func()")
	if bind < 0 || iife < 0 {
		t.Fatalf("expected the closure to bind and the IIFE to stay:\n%s", out)
	}
	if bind > iife {
		t.Errorf("the closure bound after the call that runs it:\n%s", out)
	}
}

// TestAForwardCallWhoseHelperReadsALaterLocalHandsBack pins the boundary the move
// cannot cross. The helper has to bind above the call, and its body reads a local
// the source declares below the call, which at that point is a Go variable that does
// not exist. JavaScript has no such limit, so this is a hand-back and not a guess.
func TestAForwardCallWhoseHelperReadsALaterLocalHandsBack(t *testing.T) {
	src := `function outer(n: number): number {
  const first = pick(n);
  const factor = 3;
  function pick(x: number): number { return x * factor; }
  return first;
}`
	if reason := renderProgramHandBack(t, src); !strings.Contains(reason, "hoisting") {
		t.Errorf("hand-back reason %q does not name the hoisting", reason)
	}
}

// TestAForwardCallBindsAheadOfEverythingItReads pins the other side of that
// boundary: a helper whose reads are all declared above the call moves cleanly, so
// the guard is about what the body captures and not about there being a forward call
// at all.
func TestAForwardCallBindsAheadOfEverythingItReads(t *testing.T) {
	src := `function outer(n: number): number {
  const factor = 3;
  const first = pick(n);
  function pick(x: number): number { return x * factor; }
  return first;
}`
	out := renderProgram(t, src)
	factor := strings.Index(out, "factor := 3")
	bind := strings.Index(out, "pick = func")
	call := strings.Index(out, "first := pick(")
	if factor < 0 || bind < 0 || call < 0 {
		t.Fatalf("expected all three of the binding, the closure and the call:\n%s", out)
	}
	if factor >= bind || bind >= call {
		t.Errorf("the closure did not land between what it reads and the call:\n%s", out)
	}
}

// TestHoistedHelpersRun builds and runs the shape end to end. The generated Go has
// to compile, which is the half a render test cannot check: a declaration in the
// wrong place or a func type that disagrees with the closure is a Go build failure,
// not a wrong string.
func TestHoistedHelpersRun(t *testing.T) {
	skipIfShort(t)
	src := `
function describe(n: number): string {
  function summarize(x: number): string { return classify(x) + " " + parity(x); }
  function classify(x: number): string { return x < 0 ? "negative" : "nonnegative"; }
  function parity(x: number): string { return x % 2 === 0 ? "even" : "odd"; }
  return summarize(n);
}
console.log(describe(4));
console.log(describe(-3));
`
	got := runProgramGo(t, src)
	want := "nonnegative even\nnegative odd\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestAForwardCallRuns builds and runs the moved shape end to end. This is the one
// Node's test/common needs: it calls platformTimeout at the top of the file and
// declares it a hundred lines further down.
func TestAForwardCallRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const scale = 10;
const timeout = adjust(50);
function adjust(ms: number): number { return ms * scale; }
console.log(timeout);
function outer(n: number): number {
  const first = pick(n);
  function pick(x: number): number { return x + 1; }
  return first;
}
console.log(outer(4));
`
	got := runProgramGo(t, src)
	want := "500\n5\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}
