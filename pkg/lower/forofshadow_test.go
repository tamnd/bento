package lower

import (
	"strings"
	"testing"
)

// TestForOfIteratorBodyShadowsLoopVar pins that an inner const re-declaring a
// for...of loop variable of the same name opens its own Go block rather than
// reuse the loop variable's slot. The user-iterator body `{ const v = 0 }`
// shadows the loop's `v`, so the two must not collide in one Go scope, which
// Go rejects as a redeclaration.
func TestForOfIteratorBodyShadowsLoopVar(t *testing.T) {
	const src = `class Foo { }
class FooIterator {
    next() {
        return { value: new Foo, done: false };
    }
    [Symbol.iterator]() {
        return this;
    }
}
for (const v of new FooIterator()) {
    const v = 0; // new scope
}`
	out := renderProgram(t, src)
	// The shadowed body sits in its own nested block, so the loop emits no
	// loop-variable binding of its own: only the inner declaration writes v.
	if strings.Count(out, "v :=") != 1 {
		t.Fatalf("expected exactly one v binding, the inner shadow:\n%s", out)
	}
}

// TestForOfIteratorBodyShadowRuns builds and runs the shadow so its scope is
// proven: the inner const v is the only v the body reads, and the loop variable
// stays unobservable, so a body counter advances once per pulled element.
func TestForOfIteratorBodyShadowRuns(t *testing.T) {
	skipIfShort(t)
	const src = `class Foo { }
class FooIterator {
    next() {
        return { value: new Foo, done: false };
    }
    [Symbol.iterator]() {
        return this;
    }
}
let n = 0;
for (const v of new FooIterator()) {
    const v = 0; // new scope
    n++;
    if (n > 2) break;
}
console.log(n);`
	got := runProgramGo(t, src)
	want := "3\n"
	if got != want {
		t.Fatalf("for...of shadow run mismatch:\n got %q\nwant %q", got, want)
	}
}

// TestForOfArrayBodyShadowsLoopVar pins the same for the array range path: an
// inner const shadowing the loop variable leaves the range binding-less so the
// body's own v stands alone, rather than collide with a `for _, v := range`
// value of the same name in the one Go block a range shares with its body.
func TestForOfArrayBodyShadowsLoopVar(t *testing.T) {
	skipIfShort(t)
	const src = `let n = 0;
for (const v of [1, 2, 3]) {
    const v = 10; // new scope
    n += v;
}
console.log(n);`
	got := runProgramGo(t, src)
	want := "30\n"
	if got != want {
		t.Fatalf("array for...of shadow run mismatch:\n got %q\nwant %q", got, want)
	}
}
