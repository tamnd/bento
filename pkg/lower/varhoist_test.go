package lower

import (
	"strings"
	"testing"
)

// TestVarHoistEmitsScopeTopDeclaration pins the shape: a var written in a nested
// block and read after it is declared once at the top of its scope with the
// binding's Go type, and the in-block var lowers to a plain assignment, the
// function-scoping JavaScript gives a var.
func TestVarHoistEmitsScopeTopDeclaration(t *testing.T) {
	const src = `function label(n: number): string {
  if (n > 0) {
    var sign = "positive";
  } else {
    var sign = "negative";
  }
  return sign;
}
console.log(label(1));
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var sign value.BStr") {
		t.Errorf("hoisted var was not declared at the scope top:\n%s", source)
	}
	if !strings.Contains(source, "sign = value.FromGoString(\"positive\")") {
		t.Errorf("in-block var did not lower to an assignment:\n%s", source)
	}
	if strings.Contains(source, "sign :=") {
		t.Errorf("hoisted var kept a block-local short declaration:\n%s", source)
	}
}

// TestBlockLocalVarStaysLocal pins the boundary: a var used only inside its own
// block is not hoisted, so it keeps its block-local declaration and no top-of-scope
// var appears.
func TestBlockLocalVarStaysLocal(t *testing.T) {
	const src = `function g(n: number): number {
  let total = 0;
  if (n > 0) {
    var y = n * 2;
    total = y;
  }
  return total;
}
console.log(g(3));
`
	source := renderProgram(t, src)
	if strings.Contains(source, "var y float64") {
		t.Errorf("a block-only var was hoisted when it did not escape:\n%s", source)
	}
	if !strings.Contains(source, "y := ") {
		t.Errorf("a block-only var lost its block-local declaration:\n%s", source)
	}
}

// TestTopLevelBlockVarHoists pins that a var declared inside a bare block at the
// scope root, then read after the block, hoists to a single scope-top declaration.
// The block sits directly in the top statement list, so the hoist walk has to step
// into the top statement itself; starting one level below would step over the block
// and leave the var block-local, undeclared at the read after it.
func TestTopLevelBlockVarHoists(t *testing.T) {
	const src = `var x = "outside";
{
  var x = "inside";
}
console.log(x);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "var x value.BStr") {
		t.Errorf("top-level block var was not declared once at the scope top:\n%s", source)
	}
	if !strings.Contains(source, "x = value.FromGoString(\"inside\")") {
		t.Errorf("in-block var did not lower to an assignment:\n%s", source)
	}
	if strings.Contains(source, "x :=") {
		t.Errorf("top-level block var kept a short declaration, splitting the binding:\n%s", source)
	}
}

// TestTopLevelBlockVarRuns builds and runs the top-level block var end to end: the
// outer and in-block var are one binding, so the value read after the block is the
// one the block assigned, matching JavaScript function scoping.
func TestTopLevelBlockVarRuns(t *testing.T) {
	skipIfShort(t)
	const src = `var x = "outside";
{
  var x = "inside";
}
console.log(x);
`
	got := runProgramGo(t, src)
	want := "inside\n"
	if got != want {
		t.Fatalf("top-level block var printed %q, want %q", got, want)
	}
}

// TestVarHoistRuns builds and runs the hoist end to end against the Node answers: a
// var assigned in each branch of an if and read after it returns the branch value,
// and a var written in a catch and read after the try carries across the block.
func TestVarHoistRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function label(n: number): string {
  if (n > 0) {
    var sign = "positive";
  } else {
    var sign = "non-positive";
  }
  return sign;
}
let outcome = "";
try {
  throw "boom";
} catch (e) {
  var handled = 1;
} finally {
  outcome = "swept";
}
console.log(label(3));
console.log(label(-2));
if (handled === 1) {
  console.log("caught");
}
console.log(outcome);
`
	got := runProgramGo(t, src)
	want := "positive\nnon-positive\ncaught\nswept\n"
	if got != want {
		t.Fatalf("var-hoist program printed %q, want %q", got, want)
	}
}

// TestForwardCapturedVarHoists pins that a var a closure declared earlier in the
// same scope reads is pre-declared at the scope top, above the closure, and its own
// site lowers to an assignment. Without the hoist the closure would close over a
// name declared below it, which Go rejects with an undefined reference.
func TestForwardCapturedVarHoists(t *testing.T) {
	const src = `function f1(): number {
  function f2(): number { return x; }
  var x = 1;
  return f2();
}
console.log(f1());
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var x float64") {
		t.Fatalf("forward-captured var was not pre-declared at the scope top:\n%s", out)
	}
	if strings.Contains(out, "x :=") {
		t.Fatalf("forward-captured var kept a short declaration below its closure:\n%s", out)
	}
	if !strings.Contains(out, "x = 1") {
		t.Fatalf("forward-captured var did not lower its site to an assignment:\n%s", out)
	}
}

// TestForwardCapturedVarRuns proves the scope chain: an inner function reads a var
// its enclosing function declares after the inner function, and a mutation through a
// second inner function is visible, because all three share the one hoisted binding.
func TestForwardCapturedVarRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function f(): number {
  function get(): number { return z; }
  function inc(): void { z = z + 1; }
  var z = 10;
  inc();
  return get();
}
console.log(f());
`
	got := runProgramGo(t, src)
	want := "11\n"
	if got != want {
		t.Fatalf("forward-captured var run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestSelfReferentialVarHoists pins that a var whose own initializer reads its
// name is declared at the scope top so the initializer reads the undefined the
// var holds before assignment, matching JavaScript, rather than emitting a Go
// short declaration that reads a name it is still declaring.
func TestSelfReferentialVarHoists(t *testing.T) {
	const src = `var a: any = { f: a };`
	out := renderProgram(t, src)
	if !strings.Contains(out, "var a value.Value") {
		t.Fatalf("self-referential var did not pre-declare its slot:\n%s", out)
	}
	if strings.Contains(out, "a :=") {
		t.Fatalf("self-referential var kept a short declaration Go rejects:\n%s", out)
	}
}

// TestSelfReferentialVarRuns builds and runs the self-reference so the undefined
// read is proven by the result: a.f is the undefined the var held while its
// object literal was built.
func TestSelfReferentialVarRuns(t *testing.T) {
	skipIfShort(t)
	const src = `var a: any = { f: a };
console.log(typeof a.f);
console.log(typeof a);`
	got := runProgramGo(t, src)
	want := "undefined\nobject\n"
	if got != want {
		t.Fatalf("self-referential var run mismatch:\n got %q\nwant %q", got, want)
	}
}
