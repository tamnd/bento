package lower

import (
	"strings"
	"testing"
)

// TestValueCovariantThisUpcastInSuper pins the value-level twin of the func-return upcast:
// a super call that immediately invokes a this-returning arrow, super((() => this)()),
// passes a Super value into a Base-typed constructor parameter. The frontend types the
// invoked this as a type parameter, not a class, so the upcast reads the class from the
// emitted *Super result and addresses the promoted base field.
func TestValueCovariantThisUpcastInSuper(t *testing.T) {
	const src = `class Base { constructor(public b: Base) {} }
class Super extends Base { constructor() { super((() => this)()); } }
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func() *Super {") {
		t.Fatalf("invoked this-returning super arrow did not emit its concrete result:\n%s", out)
	}
	if !strings.Contains(out, "NewBase(&") || !strings.Contains(out, ".Base)") {
		t.Fatalf("this value into a base slot did not upcast through the promoted base field:\n%s", out)
	}
}

// TestValueCovariantThisUpcastRuns builds and runs the same shape with an observable base
// method: the super call stores this as its own base field, so reading that field back
// and calling a base method proves the upcast passed the same object under the base type.
func TestValueCovariantThisUpcastRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Base { constructor(public b: Base) {} tag(): string { return "base"; } }
class Super extends Base { constructor() { super((() => this)()); } }
const s = new Super();
console.log(s.b.tag());
`
	if got, want := runProgramGo(t, src), "base\n"; got != want {
		t.Fatalf("value covariant upcast run mismatch:\n got %q\nwant %q", got, want)
	}
}
