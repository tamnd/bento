package lower

import (
	"strings"
	"testing"
)

// TestPatternBoxLeafDeclaresAPackageBox pins the declaration half of this slice. A
// destructuring leaf off a boxed source holds a value.Value, so the package var it hoists
// to is declared as one rather than as the shape the checker gave the leaf.
func TestPatternBoxLeafDeclaresAPackageBox(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"}}') as Record<string, Row>;\n" +
		"const { a } = raw;\n" +
		"function tagOf(): string { return a.tag; }\n" +
		"console.log(tagOf());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "a value.Value") {
		t.Errorf("the boxed leaf was not declared as a box:\n%s", got)
	}
}

// TestPatternBoxLeafReadsThroughTheValueModel pins the read half, which is what made this
// a build break rather than a hand-back. The binder marks its boxed names in a set keyed by
// name and scoped to the body that ran the pattern, and a top-level function is not inside
// it, so the read there lowered as a struct field selector on a value.Value.
func TestPatternBoxLeafReadsThroughTheValueModel(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"}}') as Record<string, Row>;\n" +
		"const { a } = raw;\n" +
		"function tagOf(): string { return a.tag; }\n" +
		"console.log(tagOf());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "a.Get(value.FromGoString(\"tag\"))") {
		t.Errorf("the read off the boxed leaf did not dispatch through the value model:\n%s", got)
	}
	if strings.Contains(got, "a.Tag") {
		t.Errorf("the read off the boxed leaf kept a static field selector:\n%s", got)
	}
}

// TestPatternBoxLeafBoxesTheParameterItFlowsInto pins why the mark belongs inside the
// boxed-signature fixpoint rather than in a pass of its own after it. That pass decides a
// parameter's Go slot from what flows into it, so a leaf whose box was still unknown when
// it ran left take's parameter a static struct, and the call then had a box to coerce into
// a shape that has no coercion. It handed back where Node prints an answer.
func TestPatternBoxLeafBoxesTheParameterItFlowsInto(t *testing.T) {
	const src = "type Row = { id: number; tag: string };\n" +
		"const raw = JSON.parse('{\"a\":{\"id\":1,\"tag\":\"x\"}}') as Record<string, Row>;\n" +
		"const { a } = raw;\n" +
		"function take(r: Row): string { return r.tag; }\n" +
		"function f(): string { return take(a); }\n" +
		"console.log(f());\n"
	got := renderProgram(t, src)
	if !strings.Contains(got, "func Take(r value.Value)") {
		t.Errorf("the parameter the boxed leaf flows into was not boxed:\n%s", got)
	}
}

// TestPatternPrimitiveLeafKeepsItsGoValue pins the other side of the rule. A leaf the
// checker types number, string, or boolean comes down to its Go primitive at the bind, so
// it holds no box and hoists at that type. Marking it would put every read of the name back
// on the runtime route, where a boxed string answers undefined for toUpperCase.
func TestPatternPrimitiveLeafKeepsItsGoValue(t *testing.T) {
	const src = "const prim = JSON.parse('{\"n\":3,\"s\":\"hi\"}') as { n: number; s: string };\n" +
		"const { n, s } = prim;\n" +
		"function f(): string { return `${n} ${s.toUpperCase()}`; }\n" +
		"console.log(f());\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "n value.Value") || strings.Contains(got, "s value.Value") {
		t.Errorf("a primitive leaf was declared as a box:\n%s", got)
	}
	if !strings.Contains(got, "value.ToNumber(") || !strings.Contains(got, "value.ToString(") {
		t.Errorf("a primitive leaf did not come down at the bind:\n%s", got)
	}
}

// TestStaticPatternLeafIsUntouched pins the boundary. Only a boxed source can hand a leaf a
// box, so a pattern over a plain typed array binds ordinary Go values and hoists at their
// own types exactly as it did.
func TestStaticPatternLeafIsUntouched(t *testing.T) {
	const src = "const arr: number[] = [7, 8];\n" +
		"const [p, q] = arr;\n" +
		"function pq(): number { return p + q; }\n" +
		"console.log(pq());\n"
	got := renderProgram(t, src)
	if strings.Contains(got, "value.Value") {
		t.Errorf("a static pattern leaf was boxed:\n%s", got)
	}
	if !strings.Contains(got, "p = arr.AtI(0)") {
		t.Errorf("the static leaf did not store into its package var:\n%s", got)
	}
}
