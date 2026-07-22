package lower

import (
	"strings"
	"testing"
)

// TestTupleDestructureWidensBindingToUnion pins that a tuple destructuring whose
// binding is declared wider than the field the initializer literal minted coerces the
// field read into the binding's declared type. let [x]: [string | number] = [1] types
// the field the number the literal holds while x is declared the union, so the read
// wraps through the union constructor rather than handing back on the divergence.
func TestTupleDestructureWidensBindingToUnion(t *testing.T) {
	src := `function g() {
    let [x]: [string | number] = [1];
    x;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "NumOrStrOfNum(") {
		t.Fatalf("widened tuple binding did not coerce the field read into the union:\n%s", out)
	}
}

// TestTupleDestructureUndefinedDefaultFolds pins that a tuple destructuring with a
// default over a statically-undefined element folds to the default alone: the read is
// dead, so the binding takes the default and the source is not drawn into a temp.
func TestTupleDestructureUndefinedDefaultFolds(t *testing.T) {
	src := `function g() {
    let [z = "d"]: [string | undefined] = [undefined];
    z;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, `z := value.FromGoString("d")`) {
		t.Fatalf("undefined-element default did not fold to the default binding:\n%s", out)
	}
	if strings.Contains(out, ".E0") {
		t.Fatalf("a dead read over an always-undefined element must not draw the field:\n%s", out)
	}
}

// TestArrayDestructureWidensBindingToUnion pins the array-source sibling of the tuple
// widening: let [x]: (string | number)[] = [1] reads a float64 element into the
// string | number x, coercing the AtI read through the union constructor.
func TestArrayDestructureWidensBindingToUnion(t *testing.T) {
	src := `function g() {
    let [x]: (string | number)[] = [1];
    x;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "NumOrStrOfNum(") {
		t.Fatalf("widened array binding did not coerce the element read into the union:\n%s", out)
	}
}

// TestArrayDestructureUndefinedDefaultFolds pins the array-source sibling of the
// undefined-default fold: the always-undefined element makes the read dead, so z takes
// the default and the source is evaluated once to the blank for its side effects.
func TestArrayDestructureUndefinedDefaultFolds(t *testing.T) {
	src := `function g() {
    let [z = "d"]: (string | undefined)[] = [undefined];
    z;
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, `z := value.FromGoString("d")`) {
		t.Fatalf("undefined-element default did not fold to the default binding:\n%s", out)
	}
	if !strings.Contains(out, "_ = value.NewArray") {
		t.Fatalf("a fully-defaulted array source must still evaluate once to the blank:\n%s", out)
	}
}
