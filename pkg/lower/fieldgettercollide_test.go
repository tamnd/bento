package lower

import (
	"strings"
	"testing"
)

// TestFieldGetterGoNameCollision pins that a class with a field and a getter whose
// exported Go names collide, a field x and a getter X, renames the backing field so
// the struct no longer carries a field and a method of the same Go name. The getter
// method keeps its name, the public property reads call it, and the field takes the
// disambiguated spelling every this.x read resolves through.
func TestFieldGetterGoNameCollision(t *testing.T) {
	src := `class B {
    x = 10;
    constructor() { this.x = 10; }
    static log(a: number) { }
    foo() { B.log(this.x); }
    get X() { return this.x; }
    set bX(y: number) { this.x = y; }
}
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "X_ float64") {
		t.Fatalf("backing field was not renamed off the getter's Go name:\n%s", out)
	}
	if !strings.Contains(out, "func (b *B) X() float64") {
		t.Fatalf("getter method did not keep its Go name:\n%s", out)
	}
	if !strings.Contains(out, "return b.X_") {
		t.Fatalf("field read did not resolve the renamed backing field:\n%s", out)
	}
}

// TestFieldGetterGoNameCollisionRuns builds and runs the shape to prove the field and
// the getter route to the right values: foo reads the field through the static call,
// the getter returns it, and the setter writes it, so a read after a set sees the
// written value.
func TestFieldGetterGoNameCollisionRuns(t *testing.T) {
	skipIfShort(t)
	src := `class B {
    x = 10;
    constructor() { this.x = 10; }
    static log(a: number) { }
    foo() { B.log(this.x); }
    get X() { return this.x; }
    set bX(y: number) { this.x = y; }
}
let b = new B();
b.foo();
console.log(b.X);
b.bX = 20;
console.log(b.X);
`
	out := renderProgram(t, src)
	if got, want := goRunSource(t, out), "10\n20\n"; got != want {
		t.Fatalf("field/getter collision run mismatch:\n got %q\nwant %q", got, want)
	}
}
