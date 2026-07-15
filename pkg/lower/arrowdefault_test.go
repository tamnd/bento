package lower

import (
	"strings"
	"testing"
)

// An arrow function is a Go func value, which carries no optional parameter, so a
// defaulted arrow parameter hands back in the general case. The one safe case is a
// const-bound arrow whose binding never escapes as a value: every call to it is a
// direct call the call site fills the omitted default at, the same way a top-level
// function's default is filled. These tests pin both halves, the escape-safe arrow
// that lowers and the escaping one that keeps its handback.

// TestArrowDefaultLowers pins that an escape-safe const-bound arrow lowers its
// defaulted parameter to a plain Go field and fills the default at the omitting call
// site, so f() emits f(5) and the explicit call keeps its argument.
func TestArrowDefaultLowers(t *testing.T) {
	const src = "const f = (x = 5) => x;\nconsole.log(f());\nconsole.log(f(3));\n"
	source := renderProgram(t, src)
	if !strings.Contains(source, "f := func(x float64) float64") {
		t.Errorf("arrow default did not lower to a plain Go field:\n%s", source)
	}
	if !strings.Contains(source, "f(5)") {
		t.Errorf("omitted call did not fill the default at the call site:\n%s", source)
	}
}

// TestArrowDefaultRuns builds and runs an escape-safe arrow default, checking the
// omitting call reads the default and the explicit call reads its argument.
func TestArrowDefaultRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const f = (x = 5) => x;\nconsole.log(f());\nconsole.log(f(3));\n"
	want := "5\n3\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("arrow default printed %q, want %q", got, want)
	}
}

// TestArrowDefaultTrailingRuns proves a default on the trailing parameter fills only
// when that argument is omitted, while an earlier required parameter still binds the
// argument the call supplies.
func TestArrowDefaultTrailingRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const add = (a: number, b = 10) => a + b;\nconsole.log(add(1));\nconsole.log(add(1, 2));\n"
	want := "11\n3\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("arrow trailing default printed %q, want %q", got, want)
	}
}

// TestArrowDefaultExplicitUndefinedRuns proves an explicit undefined in a defaulted
// slot counts as a missing argument, so the default fills it the same way an omission
// does.
func TestArrowDefaultExplicitUndefinedRuns(t *testing.T) {
	skipIfShort(t)
	const src = "const f = (x = 7) => x;\nconsole.log(f(undefined));\n"
	want := "7\n"
	if got := runProgramGo(t, src); got != want {
		t.Fatalf("arrow default with explicit undefined printed %q, want %q", got, want)
	}
}

// TestArrowDefaultEscapeHandsBack pins the escape guard: an arrow whose binding is
// read as a value, not just called directly, keeps its default handback rather than
// pass a Go zero value where a default belonged.
func TestArrowDefaultEscapeHandsBack(t *testing.T) {
	const src = "const f = (x = 5) => x;\nconst g = f;\nconsole.log(g());\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "parameter with a default value") {
		t.Fatalf("escaping arrow default handed back with %q, want the default-parameter reason", reason)
	}
}

// TestArrowDefaultReadsEarlierParamHandsBack pins that a default reading an earlier
// parameter stays a handback, since it is evaluated in the callee's scope where the
// call site cannot reconstruct it, so the candidate is rejected before the escape
// analysis runs.
func TestArrowDefaultReadsEarlierParamHandsBack(t *testing.T) {
	const src = "const f = (a: number, b = a + 1) => a + b;\nconsole.log(f(2));\n"
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "parameter with a default value") {
		t.Fatalf("self-reading arrow default handed back with %q, want the default-parameter reason", reason)
	}
}
