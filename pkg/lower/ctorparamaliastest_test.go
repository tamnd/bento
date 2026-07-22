package lower

import (
	"strings"
	"testing"
)

// TestFieldInitOuterBindingShadowedByCtorParam pins that a constructor parameter whose Go
// name would shadow an outer binding a field initializer reads is renamed, so the field
// initializer reaches the outer binding rather than the parameter. A class field
// initializer runs outside the constructor's parameter scope, so `p = x` reads the outer
// const x, not the constructor parameter x of the same name.
func TestFieldInitOuterBindingShadowedByCtorParam(t *testing.T) {
	const src = `const x = 1
class C {
    p = x
    constructor(x: string) { }
}
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func NewC(x_ ") {
		t.Fatalf("colliding constructor parameter was not renamed:\n%s", out)
	}
	if !strings.Contains(out, "P: x}") && !strings.Contains(out, "P: x\n") {
		t.Fatalf("field initializer did not read the outer binding:\n%s", out)
	}
}

// TestFieldInitShadowRunsReadsOuter builds and runs the shadowing shape with an observable
// value: the field reads the outer const, so the instance carries the outer value, not the
// constructor argument, which proves the parameter no longer captures the field read.
func TestFieldInitShadowRunsReadsOuter(t *testing.T) {
	skipIfShort(t)
	const src = `const x = 7
class C {
    p = x
    constructor(x: string) { }
}
const c = new C("hi");
console.log(c.p);
`
	if got, want := runProgramGo(t, src), "7\n"; got != want {
		t.Fatalf("field init shadow run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestCtorParamPropertyShadowUpcast pins that when the renamed parameter is itself a
// parameter property, its store into the field uses the renamed name too: the property
// store `this.q = x` lowers the parameter node, which resolves through the alias, so the
// emitted constructor reads the renamed parameter, never the outer binding.
func TestCtorParamPropertyShadowUpcast(t *testing.T) {
	const src = `const x = 1
class C {
    p = x
    constructor(public x: string) { }
}
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func NewC(x_ ") {
		t.Fatalf("colliding parameter property was not renamed:\n%s", out)
	}
	if !strings.Contains(out, "X: x_") {
		t.Fatalf("parameter-property store did not use the renamed parameter:\n%s", out)
	}
	if !strings.Contains(out, "P: x}") && !strings.Contains(out, "P: x,") && !strings.Contains(out, "P: x\n") {
		t.Fatalf("field initializer did not read the outer binding:\n%s", out)
	}
}
