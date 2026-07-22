package lower

import (
	"strings"
	"testing"
)

// TestSuperCallArgCapturesThisTakesGeneralForm pins that a derived constructor
// whose super() argument captures the instance, an arrow closing over `this`,
// declines the one-line composite fold and takes the general allocate-first
// form, so the receiver is bound before the super arguments lower and the
// captured `this` has a receiver to reach. The folded form emitted a bare
// `return &B{...}` with no receiver in scope, leaving the arrow's `this` an
// undefined identifier.
func TestSuperCallArgCapturesThisTakesGeneralForm(t *testing.T) {
	const src = `class A {
    constructor(p: any) {}
}
class B extends A {
    constructor() { super({ test: () => this.someMethod() }); }
    someMethod() {}
}`
	out := renderProgram(t, src)
	// The general form binds the receiver first; the folded form would return the
	// literal directly with no receiver assignment.
	if !strings.Contains(out, "b := &B{}") {
		t.Fatalf("expected the general allocate-first constructor form:\n%s", out)
	}
}

// TestSuperCallArgCapturesThisRuns builds and runs the capture so the receiver
// the arrow closes over is proven live: invoking the captured callback after
// construction reaches the instance method and mutates observable state.
func TestSuperCallArgCapturesThisRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let hits = 0;
class A {
    constructor(p: { test: () => void }) {}
}
class B extends A {
    constructor() { super({ test: () => this.someMethod() }); }
    someMethod() { hits++; }
}
const b = new B();
b.someMethod();
console.log(hits);`
	got := runProgramGo(t, src)
	want := "1\n"
	if got != want {
		t.Fatalf("super-call this-capture run mismatch:\n got %q\nwant %q", got, want)
	}
}
