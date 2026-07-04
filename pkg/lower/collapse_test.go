package lower

import (
	"strings"
	"testing"
)

// TestCollapseTernaryObjectCondition pins that a ternary whose condition the checker
// proved always truthy, a non-null object, collapses to the taken branch rather than
// the immediately-invoked func the runtime-tested form keeps: o ? a : b becomes a,
// with no test and no wrapper, since only the taken branch ever runs.
func TestCollapseTernaryObjectCondition(t *testing.T) {
	src := "function f(o: { x: number }, a: number, b: number): number { return o ? a : b; }\nconsole.log(f({ x: 1 }, 7, 9));\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "func()") {
		t.Errorf("always-truthy ternary condition did not collapse, still wraps a func:\n%s", source)
	}
	if !strings.Contains(source, "return a") {
		t.Errorf("collapsed ternary did not return the taken branch:\n%s", source)
	}
}

// TestCollapseLogicalAndObjectLeft pins that && over an always-truthy object left
// collapses to the right operand, o && o.x becomes o.x, since a truthy left makes
// && the right and the left has no side effect to keep.
func TestCollapseLogicalAndObjectLeft(t *testing.T) {
	src := "function f(o: { x: number }): number { return o && o.x; }\nconsole.log(f({ x: 5 }));\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "func()") {
		t.Errorf("always-truthy && did not collapse, still wraps a func:\n%s", source)
	}
	if !strings.Contains(source, "return o.") {
		t.Errorf("collapsed && did not return the right operand's property read:\n%s", source)
	}
}

// TestCollapseLogicalOrObjectLeft pins the other operator: || over an always-truthy
// object left collapses to the left operand itself, since a truthy left makes || the
// left and the right is dropped, which is the short-circuit.
func TestCollapseLogicalOrObjectLeft(t *testing.T) {
	src := "function f(o: { x: number }, d: { x: number }): { x: number } { return o || d; }\nconsole.log(f({ x: 1 }, { x: 2 }).x);\n"
	source := renderProgram(t, src)
	if strings.Contains(source, "func()") {
		t.Errorf("always-truthy || did not collapse, still wraps a func:\n%s", source)
	}
	if !strings.Contains(source, "return o") {
		t.Errorf("collapsed || did not return the left operand:\n%s", source)
	}
}

// TestCollapseImpureConditionKeepsTest pins the boundary: an always-truthy object
// condition with a side effect keeps its runtime form rather than collapse, since
// dropping the condition would drop its side effect. Here the object comes from a
// call, so the ternary condition is not repeatable and does not collapse. The call
// is over an object return whose truthiness position still hands back, so the whole
// unit falls back, which is the honest not-yet-lowerable rather than a wrong drop.
func TestCollapseImpureConditionKeepsTest(t *testing.T) {
	src := "function make(): { x: number } { return { x: 1 }; }\nfunction f(a: number, b: number): number { return make() ? a : b; }\nconsole.log(f(7, 9));\n"
	renderProgramHandBack(t, src)
}

// TestCollapseRuns builds and runs the collapsed forms and matches the Node oracle,
// so the drop is proven to preserve the value the runtime test would have produced:
// an always-truthy object makes ? take its true branch, && take its right, and ||
// take its left.
func TestCollapseRuns(t *testing.T) {
	skipIfShort(t)
	const src = `function ternObj(o: { x: number }, a: number, b: number): number {
  return o ? a : b;
}
function andProp(o: { x: number }): number {
  return o && o.x;
}
function orLeft(o: { x: number }, d: { x: number }): number {
  return (o || d).x;
}
console.log(ternObj({ x: 1 }, 7, 9));
console.log(andProp({ x: 42 }));
console.log(orLeft({ x: 3 }, { x: 4 }));
`
	got := runProgramGo(t, src)
	want := "7\n42\n3\n"
	if got != want {
		t.Fatalf("collapse program printed %q, want %q", got, want)
	}
}
