package lower

import (
	"strings"
	"testing"
)

// TestCovariantFuncReturnUpcasts pins that a function returning a derived class into a
// slot wanting a base class is wrapped in an adapter that calls the source and returns
// the address of the promoted base field. Go's func types are invariant, so func() *Dog
// is not assignable to func() *Animal even though a Dog is an Animal, so the adapter
// bridges the class-covariant return.
func TestCovariantFuncReturnUpcasts(t *testing.T) {
	const src = `class Animal { kind(): string { return "animal"; } }
class Dog extends Animal {}
function run(f: () => Animal): string { return f().kind(); }
const d = new Dog();
run(() => d);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func() *Animal {") {
		t.Fatalf("covariant func return did not adapt to the base signature:\n%s", out)
	}
	if !strings.Contains(out, ".Animal") || !strings.Contains(out, "return &") {
		t.Fatalf("covariant func return did not upcast through the promoted base field:\n%s", out)
	}
}

// TestCovariantThisReturnInSuper pins the this-returning shape a super call takes: an
// arrow `() => this` returns the polymorphic this the frontend does not type as a class,
// so the adapter reads the class from the emitted *Super result and upcasts it to the
// base the constructor parameter wants.
func TestCovariantThisReturnInSuper(t *testing.T) {
	const src = `class Base { constructor(func: () => Base) {} }
class Super extends Base { constructor() { super((() => this)); } }
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "func() *Base {") {
		t.Fatalf("this-returning super arrow did not adapt to the base signature:\n%s", out)
	}
	if !strings.Contains(out, ".Base") || !strings.Contains(out, "return &") {
		t.Fatalf("this-returning super arrow did not upcast through the promoted base field:\n%s", out)
	}
}

// TestCovariantFuncReturnRuns builds and runs a derived-returning callback passed where a
// base-returning one is wanted: the adapter upcasts the result, and the base method the
// caller invokes runs, so the program prints what the base method returns.
func TestCovariantFuncReturnRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Animal { kind(): string { return "animal"; } }
class Dog extends Animal {}
function run(f: () => Animal): string { return f().kind(); }
const d = new Dog();
console.log(run(() => d));
`
	if got, want := runProgramGo(t, src), "animal\n"; got != want {
		t.Fatalf("covariant func return run mismatch:\n got %q\nwant %q", got, want)
	}
}
