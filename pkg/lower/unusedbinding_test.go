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
