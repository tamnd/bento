package lower

import (
	"strings"
	"testing"
)

// TestOptArrayLiteralContextualBoxesElements pins that a non-empty array literal
// flowing into a slot whose element type is optional, T | undefined, re-emits at that
// optional element type: each present element wraps in value.Some[T] and the literal
// spells value.NewArray[value.Opt[T]], the header the slot's *value.Array[value.Opt[T]]
// accepts. A slot under an outer optional, (number | undefined)[] | undefined, also
// re-wraps the rebuilt array in value.Some so both the element and the outer optional
// agree with the slot.
func TestOptArrayLiteralContextualBoxesElements(t *testing.T) {
	src := `type Foo = (number | undefined)[] | undefined;
const foo: Foo = [1, 2, 3];
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, "value.NewArray[value.Opt[float64]]") {
		t.Fatalf("array literal did not re-emit at the optional element type:\n%s", out)
	}
	if !strings.Contains(out, "value.Some[float64](1)") {
		t.Fatalf("present element was not boxed in Some:\n%s", out)
	}
	if !strings.Contains(out, "value.Some[*value.Array[value.Opt[float64]]]") {
		t.Fatalf("outer optional array was not re-wrapped in Some:\n%s", out)
	}
}

// TestOptArrayElemNarrowedUnwraps pins that a read of an optional-element array the
// checker narrowed to the non-optional element, the shape a control-flow guard
// foo[i] !== undefined leaves, unwraps the stored value.Opt with .Get() so the narrowed
// use sees the T the checker gives it, while the undefined test itself keeps the Opt to
// call IsUndefined on.
func TestOptArrayElemNarrowedUnwraps(t *testing.T) {
	src := `type Foo = (number | undefined)[] | undefined;
const foo: Foo = [1, 2, 3];
const index = 1;
if (foo !== undefined && foo[index] !== undefined && foo[index] >= 0) {
    foo[index];
}
`
	out := renderProgramTolerant(t, src)
	if !strings.Contains(out, ".AtI(int(index)).IsUndefined()") {
		t.Fatalf("undefined test did not read the element as an Opt:\n%s", out)
	}
	if !strings.Contains(out, ".AtI(int(index)).Get() >= 0") {
		t.Fatalf("narrowed comparison did not unwrap the element with Get:\n%s", out)
	}
}

// TestOptArrayContextualRuns builds and runs the shape to prove the boxed array and the
// narrowed unwraps compile and the program completes with no output.
func TestOptArrayContextualRuns(t *testing.T) {
	skipIfShort(t)
	src := `type Foo = (number | undefined)[] | undefined;
const foo: Foo = [1, 2, 3];
const index = 1;
if (foo !== undefined && foo[index] !== undefined && foo[index] >= 0) {
    foo[index];
}
`
	out := renderProgramTolerant(t, src)
	if got := goRunSource(t, out); got != "" {
		t.Fatalf("optional-array run mismatch:\n got %q\nwant %q", got, "")
	}
}
