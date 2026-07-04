package lower

import (
	"strings"
	"testing"
)

// TestObjectShorthandEmitsFieldFromIdentifier pins that a shorthand member {x}
// lowers like the explicit {x: x}: the interned struct field is initialised from
// the identifier read of the same name.
func TestObjectShorthandEmitsFieldFromIdentifier(t *testing.T) {
	src := `
const x = 1;
const y = "hi";
const o = { x, y };
console.log(o.x, o.y);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "X: x") || !strings.Contains(out, "Y: y") {
		t.Fatalf("expected shorthand fields initialised from the identifiers, got:\n%s", out)
	}
}

// TestObjectShorthandMixedWithExplicit pins that shorthand and explicit members
// share one interned shape, so { a, b: 2 } fills the struct from the identifier
// read for a and the expression for b.
func TestObjectShorthandMixedWithExplicit(t *testing.T) {
	src := `
const a = 10;
const o = { a, b: 2 };
console.log(o.a, o.b);
`
	out := renderProgram(t, src)
	if !strings.Contains(out, "A: a") || !strings.Contains(out, "B: 2") {
		t.Fatalf("expected a from the identifier and b from the literal, got:\n%s", out)
	}
}

// TestObjectShorthandRuns builds and runs the emitted Go and checks the field
// reads against the Node oracle.
func TestObjectShorthandRuns(t *testing.T) {
	skipIfShort(t)
	src := `
const first = "ada";
const age = 36;
const active = true;
const person = { first, age, active };
console.log(person.first);
console.log(person.age);
console.log(person.active);
`
	got := runProgramGo(t, src)
	want := "ada\n36\ntrue\n"
	if got != want {
		t.Fatalf("shorthand run mismatch:\n got %q\nwant %q", got, want)
	}
}
