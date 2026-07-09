package lower

import (
	"strings"
	"testing"
)

// TestUnusedBindingBlanked pins that a local declared and never read gets a
// trailing blank assignment. Go rejects a declared-and-not-used local, so without
// the blank the emitted Go would not compile. A lone `var x = <expr>;` is the most
// common test262 shape, so this wall stands between the prelude and most bodies.
func TestUnusedBindingBlanked(t *testing.T) {
	src := `var x = 5;`
	out := renderProgram(t, src)
	if !strings.Contains(out, "_ = x") {
		t.Fatalf("unused local did not get a trailing blank assignment:\n%s", out)
	}
}

// TestTruthyFoldedConditionBlanked pins that an object whose only read is a
// control-flow condition still builds. lowerTruthy folds an always-truthy object to
// the Go constant true and drops the read, so a var read only there would be
// declared and not used. countElidedReads records the folded condition so the
// binding gets a trailing blank instead. test262 reaches this with for (; obj; )
// where obj is a plain object, always truthy.
func TestTruthyFoldedConditionBlanked(t *testing.T) {
	src := `var obj = { value: false }; for (var i = 0; obj; ) { break; }`
	out := renderProgram(t, src)
	if !strings.Contains(out, "for true") {
		t.Fatalf("always-truthy for condition did not fold to a constant:\n%s", out)
	}
	if !strings.Contains(out, "_ = obj") {
		t.Fatalf("object read only by a folded condition did not get a blank:\n%s", out)
	}
}

// TestTruthyFoldedConditionKeptWhenReadElsewhere pins that an object read again
// outside the folded condition keeps no spurious behavior: the emit reads it, so the
// blank the fold would add sits harmlessly beside the real read and the value prints.
func TestTruthyFoldedConditionKeptWhenReadElsewhere(t *testing.T) {
	skipIfShort(t)
	src := `var obj = { value: 7 }; var out = 0; if (obj) { out = obj.value; } console.log(String(out));`
	got := runProgramGo(t, src)
	want := "7\n"
	if got != want {
		t.Fatalf("object used past a folded condition run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestUsedBindingNotBlanked pins that a local read somewhere keeps no blank, so a
// binding the program actually uses reads as the developer wrote it.
func TestUsedBindingNotBlanked(t *testing.T) {
	src := `var x = 5; console.log(String(x));`
	out := renderProgram(t, src)
	if strings.Contains(out, "_ = x") {
		t.Fatalf("used local gained a spurious blank assignment:\n%s", out)
	}
}

// TestShorthandRefBuildsAndRuns pins that a binding referenced only through an
// object-literal shorthand still builds and runs. The shorthand identifier resolves
// to the property rather than the local, so the symbol walk counts the local as
// unused and emits a trailing `_ = first`. That blank is harmless: it sits beside a
// binding the struct literal reads on the next line, so the Go compiles and the
// program prints the value. Keying the blank on the symbol count alone, rather than
// on how many times the name appears, is what lets a shadowed unused binding get its
// own blank without an outer namesake keeping it alive.
func TestShorthandRefBuildsAndRuns(t *testing.T) {
	skipIfShort(t)
	src := `const first = "ada"; const person = { first }; console.log(person.first);`
	got := runProgramGo(t, src)
	want := "ada\n"
	if got != want {
		t.Fatalf("shorthand-referenced binding run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestRedeclaredVarBlankedOnce pins that a `var` redeclared in the same block, one
// binding with two declaration name nodes, gets a single trailing blank rather than
// none. JavaScript folds `{ var f; var f; }` to one binding, so the second `var`
// lowers to nothing and the first must carry the blank; counting the two name nodes
// as two uses would leave the binding unblanked and the emitted Go would not compile.
func TestRedeclaredVarBlankedOnce(t *testing.T) {
	src := `{ var f; var f; }`
	out := renderProgram(t, src)
	if n := strings.Count(out, "_ = f"); n != 1 {
		t.Fatalf("redeclared unused var got %d blanks, want exactly 1:\n%s", n, out)
	}
}

// TestRedeclaredVarBuildsAndRuns builds and runs the block-scope redeclaration shape
// test262 exercises, `{ var f; var f; }` as a positive test that var redeclaration is
// legal, and checks it compiles and runs to completion.
func TestRedeclaredVarBuildsAndRuns(t *testing.T) {
	skipIfShort(t)
	src := `{ var f; var f; }
console.log("ok");`
	got := runProgramGo(t, src)
	want := "ok\n"
	if got != want {
		t.Fatalf("redeclared var run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestUnusedBindingRunsInitializer builds and runs an unused binding whose
// initializer has a side effect and checks the effect still happens, the way an
// unused `var x = f();` still evaluates f() in JavaScript.
func TestUnusedBindingRunsInitializer(t *testing.T) {
	skipIfShort(t)
	src := `
let count: number = 0;
function tick(): number {
  count += 1;
  return count;
}
var first = tick();
var second = tick();
console.log(String(count));
`
	got := runProgramGo(t, src)
	want := "2\n"
	if got != want {
		t.Fatalf("unused-binding initializer run mismatch:\n got %q\nwant %q", got, want)
	}
}
