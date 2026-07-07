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

// TestUsedBindingNotBlanked pins that a local read somewhere keeps no blank, so a
// binding the program actually uses reads as the developer wrote it.
func TestUsedBindingNotBlanked(t *testing.T) {
	src := `var x = 5; console.log(String(x));`
	out := renderProgram(t, src)
	if strings.Contains(out, "_ = x") {
		t.Fatalf("used local gained a spurious blank assignment:\n%s", out)
	}
}

// TestShorthandRefKeepsBindingUsed pins that a binding referenced only through an
// object-literal shorthand is not mistaken for unused. The shorthand identifier
// resolves to the property rather than the local, so the symbol walk alone would
// miss the read; the name-occurrence guard keeps the binding used and adds no blank.
func TestShorthandRefKeepsBindingUsed(t *testing.T) {
	src := `const first = "ada"; const person = { first }; console.log(person.first);`
	out := renderProgram(t, src)
	if strings.Contains(out, "_ = first") {
		t.Fatalf("shorthand-referenced binding was wrongly blanked:\n%s", out)
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
