package lower

import (
	"errors"
	"strings"
	"testing"
)

// TestModulePatternLeafHoistsToAPackageVar pins the choice this slice makes. A module-level
// destructuring binding a function reads declares its leaves at package scope, one var per
// leaf, and the statement stays where it was in main so the reads happen in source order.
// Before this the leaves were main locals and the function naming one of them referred to
// nothing.
func TestModulePatternLeafHoistsToAPackageVar(t *testing.T) {
	const src = "const arr: number[] = [7, 8];\n" +
		"const [p, q] = arr;\n" +
		"function pq(): number { return p + q; }\n" +
		"console.log(pq());\n"
	got := renderProgram(t, src)
	pkg := got[:strings.Index(got, "func Pq()")]
	for _, want := range []string{"\tp float64\n", "\tq float64\n"} {
		if !strings.Contains(pkg, want) {
			t.Errorf("missing %q at package scope in:\n%s", want, got)
		}
	}
}

// TestModulePatternBindStoresIntoTheHoistedVar pins the other half. The statement lowers to
// one bind per leaf the way it always did, but a bind that declared a fresh Go local would
// shadow the package var, so the emitted Go would build and the function reading the name
// would still see an unset value. Each bind stores into the package var instead.
func TestModulePatternBindStoresIntoTheHoistedVar(t *testing.T) {
	const src = "const arr: number[] = [7, 8];\n" +
		"const [p, q] = arr;\n" +
		"function pq(): number { return p + q; }\n" +
		"console.log(pq());\n"
	got := renderProgram(t, src)
	for _, want := range []string{"p = arr.AtI(0)", "q = arr.AtI(1)"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	for _, bad := range []string{"p := arr.AtI(0)", "q := arr.AtI(1)"} {
		if strings.Contains(got, bad) {
			t.Errorf("a hoisted leaf still declared a main local %q:\n%s", bad, got)
		}
	}
}

// TestModulePatternDefaultedLeafDropsItsLocalDeclaration pins the shape that takes a second
// form. A leaf with a default lowers to a `var name T` and then an if-fill rather than one
// bind, so the hoist has to drop that declaration too, not just retarget an assignment.
func TestModulePatternDefaultedLeafDropsItsLocalDeclaration(t *testing.T) {
	const src = "const arr: number[] = [7];\n" +
		"const [a, b = 5] = arr;\n" +
		"function ab(): number { return a + b; }\n" +
		"console.log(ab());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got[:strings.Index(got, "func Ab()")], "\tb float64\n") {
		t.Errorf("the defaulted leaf did not reach package scope:\n%s", got)
	}
	body := got[strings.Index(got, "func main()"):]
	if strings.Contains(body, "var b float64") {
		t.Errorf("the defaulted leaf kept a main-local declaration shadowing its package var:\n%s", got)
	}
}

// TestModulePatternNoReaderStaysInMain pins the boundary. Hoisting is what a cross-boundary
// read asks for and nothing else, so a module-level pattern no function reads keeps binding
// main locals with `:=` exactly as it did.
func TestModulePatternNoReaderStaysInMain(t *testing.T) {
	const src = "const arr: number[] = [7, 8];\n" +
		"const [p, q] = arr;\n" +
		"console.log(p + q);\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "var p float64") {
		t.Errorf("a pattern no function reads was hoisted:\n%s", got)
	}
	if !strings.Contains(got, "p := arr.AtI(0)") {
		t.Errorf("the unhoisted leaf did not bind a main local:\n%s", got)
	}
}

// TestFunctionLocalPatternIsUntouched pins that the rewrite is keyed by symbol rather than
// by text. A function body that destructures into names a hoisted module pattern also binds
// declares its own locals, and those binds keep `:=`.
func TestFunctionLocalPatternIsUntouched(t *testing.T) {
	const src = "const arr: number[] = [1, 2];\n" +
		"const [p, q] = arr;\n" +
		"function f(): number { const [p, q] = [10, 20]; return p + q; }\n" +
		"console.log(f(), p, q);\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "func F()") {
		t.Fatalf("the reader did not render:\n%s", got)
	}
	body := got[strings.Index(got, "func F()"):]
	body = body[:strings.Index(body, "\n}")]
	for _, want := range []string{"p := ", "q := "} {
		if !strings.Contains(body, want) {
			t.Errorf("a function-local pattern lost its own declaration, want %q:\n%s", want, body)
		}
	}
}

// TestModulePatternBoxedLeafHandsBack pins the edge this slice does not take. A leaf whose
// slot holds a box is marked in the per-body dynamic set of the body that destructured it,
// and a top-level function reading the name is not inside that body, so the read would
// lower against the wrong model. It hands back rather than miscompile.
func TestModulePatternBoxedLeafHandsBack(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"}}') as Record<string, Row>;\n" +
		"const { a } = raw;\n" +
		"function tagOf(): string { return a.tag; }\n" +
		"console.log(tagOf());\n"
	prog := compile(t, src)
	r := NewRenderer(prog)
	_, err := r.RenderProgram(entryFile(t, prog))
	var nyl *NotYetLowerable
	if !errors.As(err, &nyl) {
		t.Fatalf("RenderProgram err = %v, want a *NotYetLowerable", err)
	}
	if !strings.Contains(nyl.Reason, "whose slot holds a box") {
		t.Errorf("hand-back reason %q does not name the box", nyl.Reason)
	}
}
