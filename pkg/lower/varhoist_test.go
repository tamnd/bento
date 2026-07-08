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
