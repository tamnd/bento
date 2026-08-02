package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestPatternBesideANameBindsEveryName pins what this slice makes true. A statement
// declaring a pattern beside a plain name binds all three names, each through the path
// that lowers it. Before this the destructuring lowering declined the statement for
// holding more than one declaration, and the plain path spelled the pattern as a Go
// identifier, so the emitted Go declared U5B_pU2C_U20_qU5D_ and nothing named p.
func TestPatternBesideANameBindsEveryName(t *testing.T) {
	const src = "const arr: number[] = [1, 2];\n" +
		"const [p, q] = arr, extra = 9;\n" +
		"console.log(p + q + extra);\n"
	got := renderProgram(t, src)
	for _, want := range []string{"p := arr.AtI(0)", "q := arr.AtI(1)", "extra := 9.0"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "U5B_") {
		t.Errorf("the pattern was spelled as a Go identifier:\n%s", got)
	}
}

// TestPatternBesideANameHoistsEveryNameItBinds pins the module case, which handed back
// rather than miscompiled. The hoist gives every name of the statement a package var,
// the plain declaration then lowers as an assignment into its own and the pattern's
// leaves did not, so the statement looked like a mix of a redeclared name and a new one.
// Lowering declaration by declaration makes each one a group of its own, so both store
// into their package vars.
func TestPatternBesideANameHoistsEveryNameItBinds(t *testing.T) {
	const src = "const arr: number[] = [1, 2];\n" +
		"const [p, q] = arr, extra = 9;\n" +
		"function sum(): number { return p + q + extra; }\n" +
		"console.log(sum());\n"
	got := renderProgram(t, src)
	for _, want := range []string{"p = arr.AtI(0)", "q = arr.AtI(1)", "extra = 9"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"p := arr.AtI(0)", "extra := "} {
		if strings.Contains(got, bad) {
			t.Errorf("a hoisted name still declared a main local %q:\n%s", bad, got)
		}
	}
}

// TestDeclaratorsLowerInSourceOrder pins that splitting the statement keeps the order it
// had. The declarations of a variable statement evaluate left to right, so one reading a
// name an earlier one bound reads it after the bind, and a hoisted statement's assignments
// have to reach main's source order the same way.
func TestDeclaratorsLowerInSourceOrder(t *testing.T) {
	const src = "const arr: number[] = [1, 2];\n" +
		"const o = { x: 3 };\n" +
		"const a = 4, [p, q] = arr, { x } = o, b = 5;\n" +
		"function f(): number { return a + p + q + x + b; }\n" +
		"console.log(f());\n"
	got := renderProgram(t, src)
	last := -1
	for _, want := range []string{"a = 4", "p = arr.AtI(0)", "q = arr.AtI(1)", "x = o.X", "b = 5"} {
		at := strings.Index(got, want)
		if at < 0 {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
		if at < last {
			t.Errorf("%q lowered out of source order:\n%s", want, got)
		}
		last = at
	}
}

// TestPatternBesideANameHandsBackOnAShapeItCannotLower pins the boundary. The pattern
// goes through the same destructure cores a statement holding one alone goes through, so
// a shape they do not lower yet hands back with their own reason. It must not fall
// through to the plain path, which would spell the pattern as a mangled name and emit a
// program that does not build, which is what a hole did before this.
func TestPatternBesideANameHandsBackOnAShapeItCannotLower(t *testing.T) {
	const src = "const arr: number[] = [1, 2];\n" +
		"const [, s] = arr, z = 1;\n" +
		"console.log(s, z);\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("want a hand-back, got %v", err)
	}
	if !strings.Contains(nyl.Reason, "hole or rest") {
		t.Errorf("want the destructure path's own reason, got %q", nyl.Reason)
	}
}

// TestPlainMultiDeclaratorStatementIsUntouched pins the other boundary. A statement with
// no pattern in it is the plain path's, and it still lowers to one Go declaration holding
// every name rather than one per declaration.
func TestPlainMultiDeclaratorStatementIsUntouched(t *testing.T) {
	const src = "const a = 1, b = 2;\n" +
		"console.log(a + b);\n"
	got := renderProgram(t, src)
	if n := strings.Count(got, "var ("); n != 1 {
		t.Fatalf("want one grouped declaration, got %d:\n%s", n, got)
	}
	for _, want := range []string{"a float64 = 1", "b float64 = 2"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
