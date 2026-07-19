package lower

import (
	"strings"
	"testing"
)

// TestCompoundObjectFieldAssignRuns proves a compound write o.k += v on a fixed-shape
// object lowers to the field selector's Go compound assignment and mutates in place.
func TestCompoundObjectFieldAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const o = { n: 10, s: "a" };
o.n += 5;
o.n *= 2;
o.s += "b";
console.log(o.n);
console.log(o.s);
`
	if got, want := runProgramGo(t, src), "30\na" + "b\n"; got != want {
		t.Fatalf("compound object field assignment printed %q, want %q", got, want)
	}
}

// TestCompoundObjectFieldStepCollapsesToIncDec proves a compound step of one on a
// fixed-shape object field prints Go's o.N++ rather than the spelled-out addition.
func TestCompoundObjectFieldStepCollapsesToIncDec(t *testing.T) {
	const src = `const o = { n: 1 };
o.n += 1;
`
	got := renderProgram(t, src)
	if !strings.Contains(got, "o.N++") {
		t.Fatalf("compound step did not collapse to ++; got:\n%s", got)
	}
}

// TestCompoundDynamicMemberAssignRuns proves a compound write o.k <op>= v on a
// dynamic receiver loads the property through Get, runs the boxed arithmetic, and
// stores the result back through Set.
func TestCompoundDynamicMemberAssignRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const o: any = { n: 10, s: "a" };
o.n += 5;
o.n *= 2;
o.s += "b";
console.log(o.n);
console.log(o.s);
`
	if got, want := runProgramGo(t, src), "30\na" + "b\n"; got != want {
		t.Fatalf("dynamic compound member assignment printed %q, want %q", got, want)
	}
}
