package lower

import (
	"strings"
	"testing"
)

// A closure whose binding moves up the block can only go somewhere every name it
// reads already exists, and the first slice read that limit too widely: any name a
// sibling declared at or below the destination stopped the move, and the unit handed
// back. Two of those names did not have to.
//
// A sibling function declaration is one. It is asked the same question in turn and
// comes along to the same statement when the answer is yes, which is what makes the
// order safe whatever the moved body does with it.
//
// A local the source declares below the destination is the other, and the answer
// there is indirection. If the early read only hands the function along as a value,
// the name it hands along can hold a forwarder assigned at the top of the block,
// with the real body assigned to a second local at the declaration's own position
// where the locals it reads exist. The value handed out early is the one that runs
// the body later, which is what the source promised. An early call is what is left,
// because the forwarder would have no body to call yet.
//
// The three tests in Node's suite that reached this: test-crypto-domains.js calls a
// helper at the top of a domain body that names two more below it,
// test-dgram-connect-send-empty-packet.js passes a listener to common.mustCall
// before declaring it, and test-http-server-keep-alive-timeout-slow-client-headers.js
// registers a data handler that reads a `let` two lines down.

// TestAForwardCallMayReadAHelperDeclaredBelowIt is the crypto-domains shape. The
// moved closure reads a sibling function declared under the destination, which is
// fine because that sibling reaches the top of the block too.
func TestAForwardCallMayReadAHelperDeclaredBelowIt(t *testing.T) {
	src := `function register(f: () => void): void {}
function outer(n: number): number {
  const seed = one(n);
  function one(x: number): number { register(() => { two(x); }); return x + 1; }
  function two(x: number): number { return x * 2; }
  return seed;
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var two func(x float64) float64") {
		t.Fatalf("the sibling the moved closure reads did not reach the block frame:\n%s", out)
	}
	bind := strings.Index(out, "one = func")
	call := strings.Index(out, "seed := one(")
	if bind < 0 || call < 0 {
		t.Fatalf("expected the moved binding and the call it moved for:\n%s", out)
	}
	if bind >= call {
		t.Errorf("the closure did not bind ahead of the call:\n%s", out)
	}
}

// TestASiblingThatCannotComeAlongStopsTheMove is the limit on that. The sibling is
// asked the same question and answers no, because it reads a local the source
// declares below the destination, so neither of them can go there.
func TestASiblingThatCannotComeAlongStopsTheMove(t *testing.T) {
	src := `function outer(n: number): number {
  const seed = one(n);
  function one(x: number): number { return two(x) + 1; }
  const factor = 2;
  function two(x: number): number { return x * factor; }
  return seed;
}`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "hoisting") {
		t.Errorf("reason = %q, want the hoisting hand-back", reason)
	}
}

// TestASiblingComesAlongToTheSameStatement pins that the dependency moves rather
// than only reaching the block's var frame. A declaration left at its own position
// is nil for anything the moved closure hands it to that runs on the spot, and this
// is the assertion that says it is not.
func TestASiblingComesAlongToTheSameStatement(t *testing.T) {
	src := `function register(f: () => void): void {}
function outer(n: number): number {
  const seed = one(n);
  function one(x: number): number { register(() => { two(x); }); return x + 1; }
  function two(x: number): number { return x * 2; }
  return seed;
}`
	out := renderProgram(t, src)
	dep := strings.Index(out, "two = func")
	call := strings.Index(out, "seed := one(")
	if dep < 0 || call < 0 {
		t.Fatalf("expected the dependency binding and the call:\n%s", out)
	}
	if dep >= call {
		t.Errorf("the dependency did not come along ahead of the call:\n%s", out)
	}
}

// TestAValuePassedBeforeItsDeclarationForwards is the dgram shape. The helper is
// handed to a call as a value before it is declared and reads a local declared
// after that call, so its name binds a forwarder at the top of the block and the
// body lands at the declaration's own position.
func TestAValuePassedBeforeItsDeclarationForwards(t *testing.T) {
	src := `function register(f: (x: number) => number): void {}
function outer(n: number): number {
  register(step);
  const bump = 3;
  function step(x: number): number { return x + bump; }
  return n;
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var stepImpl func(x float64) float64") {
		t.Fatalf("the forwarded body did not get its own binding:\n%s", out)
	}
	if !strings.Contains(out, "stepImpl(a0)") {
		t.Fatalf("the forwarder did not pass its argument along:\n%s", out)
	}
	bind := strings.Index(out, "step = func")
	pass := strings.Index(out, "Register(step)")
	if bind < 0 || pass < 0 {
		t.Fatalf("expected the forwarder and the call it was passed to:\n%s", out)
	}
	if bind >= pass {
		t.Errorf("the forwarder did not bind ahead of the call it is passed to:\n%s", out)
	}
	body := strings.Index(out, "stepImpl = func")
	decl := strings.Index(out, "bump := 3")
	if body < 0 || decl < 0 || body < decl {
		t.Errorf("the real body did not stay below the local it reads:\n%s", out)
	}
}

// TestAValueForwardedBeforeItsDeclarationRuns is the same shape end to end. A
// forwarder that dropped an argument or returned nothing is a Go build failure or a
// wrong number, and only running it says which.
func TestAValueForwardedBeforeItsDeclarationRuns(t *testing.T) {
	skipIfShort(t)
	src := `
let held: (x: number) => number = (x: number) => x;
function register(f: (x: number) => number): void { held = f; }
function outer(n: number): number {
  register(step);
  const bump = 3;
  function step(x: number): number { return x + bump; }
  return n;
}
outer(1);
console.log(held(4));
`
	got := runProgramGo(t, src)
	want := "7\n"
	if got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// TestACallBeforeTheDeclarationOfAHelperReadingALaterLocalHandsBack is what the
// forwarder cannot do. The call runs where the forwarder has no body yet, and
// JavaScript would raise on the local's own account, so the unit runs on the engine.
func TestACallBeforeTheDeclarationOfAHelperReadingALaterLocalHandsBack(t *testing.T) {
	src := `function outer(n: number): number {
  const first = step(n);
  const bump = 3;
  function step(x: number): number { return x + bump; }
  return first;
}`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "hoisting") {
		t.Errorf("reason = %q, want the hoisting hand-back", reason)
	}
}

// TestAForwardedHelperWithNoResultForwards pins the void case, since a forwarder
// that returns a value it does not have and one that drops the call are different
// mistakes and the type decides which body shape is written.
func TestAForwardedHelperWithNoResultForwards(t *testing.T) {
	src := `function register(f: (x: number) => void): void {}
function outer(n: number): number {
  register(step);
  const bump = 3;
  function step(x: number): void { console.log(x + bump); }
  return n;
}`
	out := renderProgram(t, src)
	if !strings.Contains(out, "stepImpl(a0)") {
		t.Fatalf("the forwarder did not call the body:\n%s", out)
	}
	if strings.Contains(out, "return stepImpl(a0)") {
		t.Errorf("a forwarder with no result returned one:\n%s", out)
	}
}
