package lower

import (
	"strings"
	"testing"
)

// TestObjectSpreadEmitsFieldCopy pins that a spread member { ...base } lowers to
// a keyed composite literal that reads each of the source's fields, the same
// struct-field access a plain base.k read lowers to, so the spread copies fields
// rather than handing the literal back.
func TestObjectSpreadEmitsFieldCopy(t *testing.T) {
	const src = `const base = { a: 1, b: 2 };
const m = { ...base, c: 3 };
console.log(m.a);
`
	source := renderProgram(t, src)
	if !strings.Contains(source, "base.A") || !strings.Contains(source, "base.B") {
		t.Errorf("spread did not copy the source fields base.A and base.B:\n%s", source)
	}
}

// TestObjectSpreadNonIdentifierHandsBack pins that a spread whose source is not a
// plain identifier hands the literal back: reading the fields of an expression one
// by one would re-evaluate it, so a spread of an object literal or a call is a
// later slice rather than a wrong emission that evaluates the source per field.
func TestObjectSpreadNonIdentifierHandsBack(t *testing.T) {
	const src = `const m = { ...{ a: 1 }, b: 2 };
console.log(m.a);
`
	reason := renderProgramHandBack(t, src)
	if !strings.Contains(reason, "not a plain identifier") {
		t.Errorf("hand-back reason = %q, want it to name the non-identifier spread source", reason)
	}
}

// TestObjectSpreadRuns builds and runs the emitted Go against the Node oracle,
// covering the two override paths the lowering must match: an explicit key after
// a spread overrides the field the spread set, and a second spread overrides the
// first where their fields collide, both the left-to-right last-writer-wins rule.
func TestObjectSpreadRuns(t *testing.T) {
	skipIfShort(t)
	const src = `const base = { a: 1, b: 2 };
const over = { ...base, b: 20, c: 3 };
console.log(` + "`${over.a},${over.b},${over.c}`" + `);
const p = { a: 1, b: 2 };
const q = { b: 30, c: 40 };
const m = { ...p, ...q };
console.log(` + "`${m.a},${m.b},${m.c}`" + `);
`
	got := runProgramGo(t, src)
	const want = "1,20,3\n1,30,40\n"
	if got != want {
		t.Errorf("object spread program printed %q, want %q", got, want)
	}
}

// TestObjectSpreadOptionalOverRequiredMerges pins the union-spread merge: when a
// later spread's member is optional but the earlier one supplies the same field as
// required, the merged property is required and takes src.Field.Or(prev), the
// present-else-fallback spread evaluates, rather than dropping a bare value.Opt
// into the concrete field, which would not build.
func TestObjectSpreadOptionalOverRequiredMerges(t *testing.T) {
	const src = `declare const a: { x: number };
declare const b: { x?: number };
const c = { ...a, ...b };
`
	source := renderProgram(t, src)
	if !strings.Contains(source, ".Or(a.X)") {
		t.Errorf("optional-over-required spread did not merge with .Or(a.X):\n%s", source)
	}
}
