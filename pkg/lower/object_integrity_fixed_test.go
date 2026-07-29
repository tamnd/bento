package lower

import (
	"strings"
	"testing"
)

// A read-only integrity predicate on a fixed-shape object reads its default state.
// A fixed-shape object is always extensible, never sealed, never frozen, because the
// mutators that could change that (Object.preventExtensions, seal, freeze, and the
// Reflect forms) all hand the whole unit back on a fixed-shape receiver. So
// Object.isExtensible, isSealed, and isFrozen box a throwaway copy and read the
// predicate off it rather than hand back, and the answer matches the JavaScript
// default. The mutators keep their handback, since a copy cannot return the receiver
// with its own integrity changed.

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

// TestFixedShapeIntegrityMutatorHandsBack proves the mutating side keeps its
// handback: Object.freeze on a fixed-shape receiver cannot return the receiver with
// its integrity changed off a copy, so the whole unit hands back rather than emit a
// wrong result.
func TestFixedShapeIntegrityMutatorHandsBack(t *testing.T) {
	const src = `const o = { a: 1 };
Object.freeze(o);
`
	reason := renderProgramTolerantHandBack(t, src)
	if !strings.Contains(reason, "no runtime integrity state") {
		t.Errorf("hand-back reason %q does not name the fixed-shape integrity guard", reason)
	}
}
