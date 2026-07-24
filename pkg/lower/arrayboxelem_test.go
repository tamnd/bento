package lower

import (
	"strings"
	"testing"
)

// TestDynamicArrayElementBoxes pins that a concrete element spliced into an any[]
// is boxed to value.Value, the store an any[] holds, rather than left as its bare
// Go type the boxed slice cannot take. The self-referential spread widens the
// element to any, so the number the splice adds must box.
func TestDynamicArrayElementBoxes(t *testing.T) {
	skipIfShort(t)
	const src = `let additional: any[] = [];
for (const subcomponent of [1, 2, 3]) {
    additional = [...additional, subcomponent];
}
console.log(additional.length);
console.log(additional[2]);`
	got := runProgramGo(t, src)
	want := "3\n3\n"
	if got != want {
		t.Fatalf("dynamic array element boxing run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestDynamicArrayPushBoxes pins that a push argument crossing into an any[] boxes to
// the element store the array holds: a number through value.Number, a string through
// value.StringValue, and a function expression through the callable value.NewFunc box,
// so the pushed function reads back as a value that answers Call. A bare Go value would
// not satisfy the array's value.Value element and the emitted Go would not compile.
func TestDynamicArrayPushBoxes(t *testing.T) {
	const src = `let a: any[] = [];
a.push(1);
a.push("x");
a.push(function () { return 5; });
`
	source := renderProgram(t, src)
	for _, want := range []string{"value.Number(1)", "value.StringValue(", "value.NewFunc("} {
		if !strings.Contains(source, want) {
			t.Errorf("push into an any[] did not box through %s:\n%s", want, source)
		}
	}
}

// TestDynamicArrayPushClosureRuns builds and runs the per-iteration closure a let
// binding pushes into a dynamic array, so the boxed function is proven callable and
// each closure captures its own loop value rather than a shared final one.
func TestDynamicArrayPushClosureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let a: any[] = [];
for (let i = 0; a.push(function () { return i; }), i < 5; ++i) { }
for (let k = 0; k < 5; ++k) {
  console.log(a[k]());
}
`
	got := runProgramGo(t, src)
	want := "0\n1\n2\n3\n4\n"
	if got != want {
		t.Fatalf("dynamic array push closure run mismatch:\n got %q\nwant %q", got, want)
	}
}
