package lower

import (
	"strings"
	"testing"

	"github.com/tamnd/bento/pkg/frontend"
)

// renderAndFuncs pairs a renderer over a compiled program with the function
// declarations in it, the two things a predicate test needs.
func renderAndFuncs(prog *frontend.Program) (*Renderer, []frontend.Node) {
	var fns []frontend.Node
	collectKind(prog, prog.SourceFiles(), frontend.NodeFunctionDeclaration, &fns)
	return NewRenderer(prog), fns
}

// TestFactoryReturningAGrowingObjectRuns is the shape a Node module is built out of:
// fill an object inside a function, hand it back, read it at the call site. The bag
// crosses the return as itself, so the caller reads the property the factory set.
func TestFactoryReturningAGrowingObjectRuns(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `function make() {
  const o = {};
  o.x = 1;
  o.y = "two";
  return o;
}
const m = make();
console.log(m.x, m.y);
`))
	if got != "1 two\n" {
		t.Errorf("factory\n got: %q\nwant: %q", got, "1 two\n")
	}
}

// TestFactoryResultKeepsUndefinedAcrossTheReturn is why the return carries the box
// rather than a Go struct. The checker types make() by the shape the object finishes
// with, so a struct would hand the caller a zero-valued field for a property the
// factory never set. JavaScript reads undefined for it, on both sides of the call.
func TestFactoryResultKeepsUndefinedAcrossTheReturn(t *testing.T) {
	got := goRunSource(t, renderExpandoJS(t, `function make(set) {
  const o = {};
  if (set) {
    o.x = 1;
  }
  return o;
}
console.log(make(true).x);
console.log(make(false).x);
`))
	if got != "1\nundefined\n" {
		t.Errorf("unset property across the return\n got: %q\nwant: %q", got, "1\nundefined\n")
	}
}

// TestFactoryReturnsTheValueBag pins the signature: the Go function's result is the
// runtime value, not a struct of the shape the checker inferred.
func TestFactoryReturnsTheValueBag(t *testing.T) {
	source := renderExpandoJS(t, `function make() {
  const o = {};
  o.x = 1;
  return o;
}
console.log(make().x);
`)
	if !strings.Contains(source, "func Make() value.Value") {
		t.Errorf("a factory returning a growing object did not return the bag:\n%s", source)
	}
}

// TestFactoryWithAMixedReturnIsNotAGrowingFactory pins the boundary. A Go function has
// one result type, so a body that hands back the bag on one path and a number on
// another has no single lowering here and must not claim to return the bag.
func TestFactoryWithAMixedReturnIsNotAGrowingFactory(t *testing.T) {
	prog := compileJS(t, `function make(flag) {
  const o = {};
  o.x = 1;
  if (flag) {
    return 1;
  }
  return o;
}
console.log(make(false));
`)
	r, fns := renderAndFuncs(prog)
	for _, fn := range fns {
		if r.funcReturnsGrowingObject(fn) {
			t.Error("a function that returns a number on one path claimed to return the bag")
		}
	}
}

// TestFunctionWithNoReturnIsNotAGrowingFactory pins the other boundary: a body that
// falls off the end returns undefined, which is not this shape.
func TestFunctionWithNoReturnIsNotAGrowingFactory(t *testing.T) {
	prog := compileJS(t, `function fill() {
  const o = {};
  o.x = 1;
}
fill();
`)
	r, fns := renderAndFuncs(prog)
	for _, fn := range fns {
		if r.funcReturnsGrowingObject(fn) {
			t.Error("a function with no return claimed to return the bag")
		}
	}
}

// TestMutuallyRecursiveFactoriesTerminate pins that the visiting set breaks the cycle.
// Asking whether a returns an object that grows asks the same of b, which asks it of a
// again, and without the guard the predicate would recurse until the stack ran out.
// Either answer is fine here; what matters is that it answers. The JSDoc return types
// are what let the checker admit the cycle at all, since an unannotated pair like this
// is an implicit-any circularity it rejects outright.
func TestMutuallyRecursiveFactoriesTerminate(t *testing.T) {
	prog := compileJS(t, `/** @returns {any} */
function a() {
  const o = b();
  return o;
}
/** @returns {any} */
function b() {
  const o = a();
  return o;
}
console.log(typeof a);
`)
	r, fns := renderAndFuncs(prog)
	for _, fn := range fns {
		r.funcReturnsGrowingObject(fn)
	}
}
