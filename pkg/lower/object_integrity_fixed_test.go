package lower

import (
	"strings"
	"testing"
)

// A read-only integrity predicate on a fixed-shape object reads its default state.
// A fixed-shape object with no mutator use is always extensible, never sealed, never
// frozen, so Object.isExtensible, isSealed, and isFrozen box a throwaway copy and read
// the predicate off it rather than hand back, and the answer matches the JavaScript
// default. A binding that a mutator (Object.preventExtensions, seal, freeze) does touch
// is a different case: the objectdynshape routing boxes that binding from its literal, so
// the mutator lowers to the runtime integrity call on the real object and a later read
// sees the changed state. The copy path here covers only bindings no mutator names.

// TestFixedShapeIsFrozenLowers proves Object.isFrozen on a fixed-shape receiver
// lowers to the boxed-copy predicate rather than handing back.
func TestFixedShapeIsFrozenLowers(t *testing.T) {
	const src = `const o = { a: 1 };
console.log(Object.isFrozen(o));
`
	source := renderProgramTolerant(t, src)
	if !strings.Contains(source, "ObjectFromStruct") || !strings.Contains(source, "IsFrozen()") {
		t.Errorf("Object.isFrozen on a fixed shape did not lower to the boxed-copy predicate:\n%s", source)
	}
}

// TestFixedShapeIntegrityPredicatesRun builds and runs the three predicates against
// the JavaScript result: a fresh fixed-shape object is extensible, not sealed, not
// frozen.
func TestFixedShapeIntegrityPredicatesRun(t *testing.T) {
	skipIfShort(t)
	const src = `
const o = { a: 1, b: 2 };
console.log(Object.isExtensible(o));
console.log(Object.isSealed(o));
console.log(Object.isFrozen(o));
console.log(Object.isExtensible({}));
`
	if got, want := runProgramGoTolerant(t, src), "true\nfalse\nfalse\ntrue\n"; got != want {
		t.Fatalf("fixed-shape integrity predicates printed %q, want %q", got, want)
	}
}

// TestFixedShapeIntegrityMutatorBoxesAndRuns proves the mutating side no longer hands
// back: a binding handed to Object.freeze boxes from its literal, so the freeze lowers
// to the runtime Freeze on the real object. A later write is dropped and isFrozen reads
// true, matching the JavaScript the runtime object honors.
func TestFixedShapeIntegrityMutatorBoxesAndRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const o = { a: 1 };
Object.freeze(o);
o.a = 99;
console.log(o.a);
console.log(Object.isFrozen(o));
`
	if got, want := runProgramGoTolerant(t, src), "1\ntrue\n"; got != want {
		t.Fatalf("frozen boxed binding printed %q, want %q", got, want)
	}
}
