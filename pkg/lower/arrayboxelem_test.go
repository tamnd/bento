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

// TestEvolvingArrayElementCallRoutesThroughCall pins that a call whose callee is an
// element read off an evolving array, a[k]() where a was declared any[] and narrowed to
// a function, dispatches through the boxed value's Call method rather than a direct Go
// call on a value.Value, which would not compile.
func TestEvolvingArrayElementCallRoutesThroughCall(t *testing.T) {
	const src = `let a = [];
a.push(function () { return 7; });
console.log(a[0]());
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Call()") {
		t.Errorf("evolving-array element call did not route through the boxed Call:\n%s", source)
	}
}

// TestForInitClosureCapturingCounterHandsBack pins that a for-let whose init clause
// declares a closure over another loop counter hands back rather than emit the block
// form, which hoists the counter to one shared var the post clause mutates and would
// make the closure read the counter's final value where ES6 freezes it at the init
// binding. A handback keeps the shape safe until the per-iteration lowering lands.
func TestForInitClosureCapturingCounterHandsBack(t *testing.T) {
	const src = `let a: any[] = [];
for (let i = 0, f = function () { return i; }; i < 5; ++i) {
  a.push(f);
}
`
	prog := compile(t, src)
	r := NewRenderer(prog)
	r.SetGoSignatures(testGoSignatures())
	_, err := r.RenderProgram(entryFile(t, prog))
	if err == nil {
		t.Fatalf("for-let init closure over a counter lowered, want a hand-back:\n%s", src)
	}
	if !strings.Contains(err.Error(), "per-iteration binding") {
		t.Fatalf("handback reason = %q, want a per-iteration-binding deferral", err.Error())
	}
}

// TestMultiCounterForFoldsToGoInitClause pins that a for-let declaring two float64
// counters folds into Go's own multi-variable init clause rather than the block
// form, so Go's per-iteration loop variables let a body closure capture each
// iteration's counter value. The block form would hoist the counters into shared
// vars the post clause mutates in place, capturing their final value instead.
func TestMultiCounterForFoldsToGoInitClause(t *testing.T) {
	const src = `let a: any[] = [], b: any[] = [];
for (let i = 0, j = 10; i < 5; ++i, ++j) {
  a.push(function () { return i; });
  b.push(function () { return j; });
}
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "for i, j := 0.0, 10.0;") {
		t.Errorf("multi-counter for did not fold into Go's init clause:\n%s", source)
	}
}

// TestMultiCounterClosureRuns builds and runs the two-counter loop, proving each
// body closure reads its own iteration's counter, so a[k] answers k and b[k]
// answers k + 10, the per-iteration binding ES6 gives a let loop.
func TestMultiCounterClosureRuns(t *testing.T) {
	skipIfShort(t)
	const src = `let a: any[] = [], b: any[] = [];
for (let i = 0, j = 10; i < 5; ++i, ++j) {
  a.push(function () { return i; });
  b.push(function () { return j; });
}
for (let k = 0; k < 5; ++k) {
  console.log(a[k]());
  console.log(b[k]());
}
`
	got := runProgramGo(t, src)
	want := "0\n10\n1\n11\n2\n12\n3\n13\n4\n14\n"
	if got != want {
		t.Fatalf("multi-counter closure run mismatch:\n got %q\nwant %q", got, want)
	}
}
